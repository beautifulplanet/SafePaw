// =============================================================
// SafePaw Gateway - Zero-Dependency Redis Client (RESP)
// =============================================================
// Minimal Redis client implementing only what revocation needs:
//   SET key value EX ttl
//   GET key
//   DEL key
//   KEYS pattern (for count)
//   AUTH password
//
// Uses the Redis RESP (REdis Serialization Protocol) directly.
// No external dependencies. Connection pooling via single
// persistent connection with automatic reconnect.
//
// WHY zero-dep?
//   The gateway uses only google/uuid as its sole dependency.
//   Adding go-redis would pull in 15+ transitive packages.
//   RESP is simple enough to implement in <200 lines.
// =============================================================

package middleware

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RedisClient is a minimal RESP protocol client.
type RedisClient struct {
	mu       sync.Mutex
	addr     string
	password string
	conn     net.Conn
	reader   *bufio.Reader
}

// NewRedisClient connects to Redis. Returns nil if addr is empty.
func NewRedisClient(addr, password string) *RedisClient {
	if addr == "" {
		return nil
	}
	rc := &RedisClient{addr: addr, password: password}
	if err := rc.connect(); err != nil {
		log.Printf("[REDIS] Initial connection failed (will retry): %v", err)
	}
	return rc
}

func (rc *RedisClient) connect() error {
	conn, err := net.DialTimeout("tcp", rc.addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", rc.addr, err)
	}
	rc.conn = conn
	rc.reader = bufio.NewReader(conn)

	if rc.password != "" {
		if err := rc.auth(); err != nil {
			conn.Close() // #nosec G104 -- cleanup on auth failure
			rc.conn = nil
			return fmt.Errorf("auth: %w", err)
		}
	}
	return nil
}

func (rc *RedisClient) auth() error {
	return rc.sendCommand("AUTH", rc.password)
}

func (rc *RedisClient) sendCommand(args ...string) error {
	if rc.conn == nil {
		if err := rc.connect(); err != nil {
			return err
		}
	}

	_ = rc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}

	_, err := rc.conn.Write([]byte(b.String()))
	return err
}

func (rc *RedisClient) readLine() (string, error) {
	_ = rc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := rc.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (rc *RedisClient) readResponse() (string, error) {
	line, err := rc.readLine()
	if err != nil {
		return "", err
	}

	if len(line) == 0 {
		return "", fmt.Errorf("empty response")
	}

	switch line[0] {
	case '+': // Simple string
		return line[1:], nil
	case '-': // Error
		return "", fmt.Errorf("redis: %s", line[1:])
	case ':': // Integer
		return line[1:], nil
	case '$': // Bulk string
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return "", fmt.Errorf("invalid bulk length: %s", line)
		}
		if n == -1 {
			return "", nil // nil
		}
		buf := make([]byte, n+2) // +2 for \r\n
		_ = rc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err = bufio.NewReader(rc.reader).Read(buf)
		if err != nil {
			full := make([]byte, 0, n+2)
			for len(full) < n+2 {
				chunk := make([]byte, n+2-len(full))
				nn, err := rc.reader.Read(chunk)
				full = append(full, chunk[:nn]...)
				if err != nil {
					return string(full[:min(len(full), n)]), err
				}
			}
			return string(full[:n]), nil
		}
		return string(buf[:n]), nil
	case '*': // Array (used by KEYS)
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return "", fmt.Errorf("invalid array length: %s", line)
		}
		if n <= 0 {
			return "0", nil
		}
		for i := 0; i < n; i++ {
			if _, err := rc.readResponse(); err != nil {
				return "", err
			}
		}
		return strconv.Itoa(n), nil
	default:
		return line, nil
	}
}

func (rc *RedisClient) reconnectAndRetry(args ...string) (string, error) {
	if rc.conn != nil {
		rc.conn.Close() // #nosec G104 -- cleanup before reconnect
		rc.conn = nil
	}
	if err := rc.connect(); err != nil {
		return "", err
	}
	if err := rc.sendCommand(args...); err != nil {
		return "", err
	}
	return rc.readResponse()
}

// Do executes a Redis command and returns the response.
func (rc *RedisClient) Do(args ...string) (string, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if err := rc.sendCommand(args...); err != nil {
		return rc.reconnectAndRetry(args...)
	}

	resp, err := rc.readResponse()
	if err != nil {
		return rc.reconnectAndRetry(args...)
	}
	return resp, nil
}

// Set stores a key with optional TTL.
func (rc *RedisClient) Set(key, value string, ttl time.Duration) error {
	if ttl > 0 {
		_, err := rc.Do("SET", key, value, "EX", strconv.Itoa(int(ttl.Seconds())))
		return err
	}
	_, err := rc.Do("SET", key, value)
	return err
}

// Get retrieves a key's value. Returns empty string if not found.
func (rc *RedisClient) Get(key string) (string, error) {
	return rc.Do("GET", key)
}

// Del deletes a key.
func (rc *RedisClient) Del(key string) error {
	_, err := rc.Do("DEL", key)
	return err
}

// Close closes the connection.
func (rc *RedisClient) Close() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.conn != nil {
		rc.conn.Close() // #nosec G104 -- best-effort cleanup
		rc.conn = nil
	}
}
