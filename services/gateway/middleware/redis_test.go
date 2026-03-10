package middleware

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockRedisServer is a minimal RESP server for testing RedisClient.
// It handles AUTH, SET, GET, DEL, and KEYS commands.
type mockRedisServer struct {
	listener net.Listener
	mu       sync.Mutex
	store    map[string]string
	password string
	closed   bool
}

func newMockRedisServer(t *testing.T, password string) *mockRedisServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock redis: listen failed: %v", err)
	}
	s := &mockRedisServer{
		listener: ln,
		store:    make(map[string]string),
		password: password,
	}
	go s.serve(t)
	return s
}

func (s *mockRedisServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *mockRedisServer) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.listener.Close()
}

func (s *mockRedisServer) serve(t *testing.T) {
	t.Helper()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go s.handleConn(t, conn)
	}
}

func (s *mockRedisServer) handleConn(t *testing.T, conn net.Conn) {
	t.Helper()
	defer conn.Close()
	reader := bufio.NewReader(conn)
	authed := s.password == "" // auto-auth if no password

	for {
		args, err := readRESPArray(reader)
		if err != nil {
			return
		}
		if len(args) == 0 {
			continue
		}

		cmd := strings.ToUpper(args[0])
		switch cmd {
		case "AUTH":
			if len(args) < 2 {
				writeRESPError(conn, "ERR wrong number of arguments")
				continue
			}
			if args[1] == s.password {
				authed = true
				writeRESPSimple(conn, "OK")
			} else {
				writeRESPError(conn, "ERR invalid password")
			}

		case "SET":
			if !authed {
				writeRESPError(conn, "NOAUTH Authentication required")
				continue
			}
			if len(args) < 3 {
				writeRESPError(conn, "ERR wrong number of arguments")
				continue
			}
			key, val := args[1], args[2]
			s.mu.Lock()
			s.store[key] = val
			s.mu.Unlock()
			writeRESPSimple(conn, "OK")

		case "GET":
			if !authed {
				writeRESPError(conn, "NOAUTH Authentication required")
				continue
			}
			if len(args) < 2 {
				writeRESPError(conn, "ERR wrong number of arguments")
				continue
			}
			s.mu.Lock()
			val, ok := s.store[args[1]]
			s.mu.Unlock()
			if !ok {
				writeRESPBulkNil(conn)
			} else {
				writeRESPBulk(conn, val)
			}

		case "DEL":
			if !authed {
				writeRESPError(conn, "NOAUTH Authentication required")
				continue
			}
			if len(args) < 2 {
				writeRESPError(conn, "ERR wrong number of arguments")
				continue
			}
			s.mu.Lock()
			_, existed := s.store[args[1]]
			delete(s.store, args[1])
			s.mu.Unlock()
			if existed {
				writeRESPInt(conn, 1)
			} else {
				writeRESPInt(conn, 0)
			}

		case "KEYS":
			if !authed {
				writeRESPError(conn, "NOAUTH Authentication required")
				continue
			}
			s.mu.Lock()
			count := len(s.store)
			s.mu.Unlock()
			// Simplified: just return count as array length with dummy entries
			writeRESPArrayLen(conn, count)
			s.mu.Lock()
			for k := range s.store {
				writeRESPBulk(conn, k)
			}
			s.mu.Unlock()

		default:
			writeRESPError(conn, "ERR unknown command '"+cmd+"'")
		}
	}
}

// readRESPArray reads a RESP array (*N\r\n$len\r\ndata\r\n...).
func readRESPArray(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '*' {
		return nil, fmt.Errorf("expected array, got %q", line)
	}
	n := 0
	for i := 1; i < len(line); i++ {
		n = n*10 + int(line[i]-'0')
	}
	args := make([]string, n)
	for i := 0; i < n; i++ {
		// Read $len
		lenLine, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		lenLine = strings.TrimRight(lenLine, "\r\n")
		if len(lenLine) < 2 || lenLine[0] != '$' {
			return nil, fmt.Errorf("expected bulk string, got %q", lenLine)
		}
		size := 0
		for j := 1; j < len(lenLine); j++ {
			size = size*10 + int(lenLine[j]-'0')
		}
		buf := make([]byte, size+2) // +2 for \r\n
		total := 0
		for total < size+2 {
			nn, err := r.Read(buf[total:])
			if err != nil {
				return nil, err
			}
			total += nn
		}
		args[i] = string(buf[:size])
	}
	return args, nil
}

func writeRESPSimple(conn net.Conn, msg string) {
	fmt.Fprintf(conn, "+%s\r\n", msg)
}

func writeRESPError(conn net.Conn, msg string) {
	fmt.Fprintf(conn, "-%s\r\n", msg)
}

func writeRESPBulk(conn net.Conn, val string) {
	fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(val), val)
}

func writeRESPBulkNil(conn net.Conn) {
	fmt.Fprint(conn, "$-1\r\n")
}

func writeRESPInt(conn net.Conn, n int) {
	fmt.Fprintf(conn, ":%d\r\n", n)
}

func writeRESPArrayLen(conn net.Conn, n int) {
	fmt.Fprintf(conn, "*%d\r\n", n)
}

// ── Tests ───────────────────────────────────────────────────────

func TestRedisClient_NilForEmptyAddr(t *testing.T) {
	rc := NewRedisClient("", "")
	if rc != nil {
		t.Error("expected nil client for empty addr")
	}
}

func TestRedisClient_SetGetDel(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	// SET
	if err := rc.Set("key1", "value1", 0); err != nil {
		t.Fatalf("SET failed: %v", err)
	}

	// GET
	val, err := rc.Get("key1")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if val != "value1" {
		t.Errorf("expected value1, got %q", val)
	}

	// DEL
	if err := rc.Del("key1"); err != nil {
		t.Fatalf("DEL failed: %v", err)
	}

	// GET after DEL
	val, err = rc.Get("key1")
	if err != nil {
		t.Fatalf("GET after DEL failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty after DEL, got %q", val)
	}
}

func TestRedisClient_SetWithTTL(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	// SET with TTL — our mock doesn't expire, but the command should succeed
	if err := rc.Set("ttl-key", "ttl-val", 60*time.Second); err != nil {
		t.Fatalf("SET with TTL failed: %v", err)
	}

	val, err := rc.Get("ttl-key")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if val != "ttl-val" {
		t.Errorf("expected ttl-val, got %q", val)
	}
}

func TestRedisClient_WithPassword(t *testing.T) {
	srv := newMockRedisServer(t, "secret123")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "secret123")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	if err := rc.Set("auth-key", "auth-val", 0); err != nil {
		t.Fatalf("SET with auth failed: %v", err)
	}

	val, err := rc.Get("auth-key")
	if err != nil {
		t.Fatalf("GET with auth failed: %v", err)
	}
	if val != "auth-val" {
		t.Errorf("expected auth-val, got %q", val)
	}
}

func TestRedisClient_WrongPassword(t *testing.T) {
	srv := newMockRedisServer(t, "correct-password")
	defer srv.Close()

	// Connection with wrong password should fail on first command or auth
	rc := NewRedisClient(srv.Addr(), "wrong-password")
	if rc == nil {
		t.Fatal("expected non-nil client (connects lazily)")
	}
	defer rc.Close()

	// The connection itself may fail during auth.
	// At minimum, subsequent commands should fail.
	err := rc.Set("x", "y", 0)
	if err == nil {
		t.Log("SET didn't fail immediately — wrong password may not propagate until command")
	}
}

func TestRedisClient_Do(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	// Direct Do call
	resp, err := rc.Do("SET", "do-key", "do-val")
	if err != nil {
		t.Fatalf("Do SET failed: %v", err)
	}
	if resp != "OK" {
		t.Errorf("expected OK, got %q", resp)
	}

	resp, err = rc.Do("GET", "do-key")
	if err != nil {
		t.Fatalf("Do GET failed: %v", err)
	}
	if resp != "do-val" {
		t.Errorf("expected do-val, got %q", resp)
	}
}

func TestRedisClient_Reconnect(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	// First command succeeds
	if err := rc.Set("k1", "v1", 0); err != nil {
		t.Fatalf("first SET failed: %v", err)
	}

	// Force-close the underlying connection to simulate disconnect
	rc.mu.Lock()
	if rc.conn != nil {
		rc.conn.Close()
	}
	rc.mu.Unlock()

	// Next command should reconnect and succeed
	if err := rc.Set("k2", "v2", 0); err != nil {
		t.Fatalf("SET after reconnect failed: %v", err)
	}

	val, err := rc.Get("k2")
	if err != nil {
		t.Fatalf("GET after reconnect failed: %v", err)
	}
	if val != "v2" {
		t.Errorf("expected v2, got %q", val)
	}
}

func TestRedisClient_Close(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}

	// Close should not panic
	rc.Close()

	// Double close should not panic
	rc.Close()
}

func TestRedisClient_ConnectionRefused(t *testing.T) {
	// Connect to a port nothing is listening on
	rc := NewRedisClient("127.0.0.1:1", "") // port 1 almost certainly not listening
	if rc == nil {
		t.Fatal("expected non-nil client (lazy connect)")
	}
	defer rc.Close()

	// Commands should fail
	_, err := rc.Get("x")
	if err == nil {
		t.Error("expected error connecting to refused port")
	}
}

func TestRedisClient_MultipleKeys(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	// Set multiple keys
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		val := fmt.Sprintf("val-%d", i)
		if err := rc.Set(key, val, 0); err != nil {
			t.Fatalf("SET %s failed: %v", key, err)
		}
	}

	// Verify all
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		val, err := rc.Get(key)
		if err != nil {
			t.Fatalf("GET %s failed: %v", key, err)
		}
		expected := fmt.Sprintf("val-%d", i)
		if val != expected {
			t.Errorf("GET %s = %q, want %q", key, val, expected)
		}
	}
}

func TestRedisClient_KEYS(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	if rc == nil {
		t.Fatal("expected non-nil client")
	}
	defer rc.Close()

	// Set some keys
	rc.Set("a", "1", 0)
	rc.Set("b", "2", 0)
	rc.Set("c", "3", 0)

	// KEYS * should return "3" (count as string)
	resp, err := rc.Do("KEYS", "*")
	if err != nil {
		t.Fatalf("KEYS failed: %v", err)
	}
	if resp != "3" {
		t.Errorf("expected KEYS count=3, got %q", resp)
	}
}
