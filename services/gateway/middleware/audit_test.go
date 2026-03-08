package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityContext_Lifecycle(t *testing.T) {
	// Verify context round-trip: create → attach → retrieve
	r := httptest.NewRequest("POST", "/echo", nil)
	r.Header.Set("X-Request-ID", "test-123")
	r.RemoteAddr = "10.0.0.1:12345"

	sc := NewSecurityContext(r)
	if sc.RequestID != "test-123" {
		t.Errorf("RequestID = %q, want %q", sc.RequestID, "test-123")
	}
	if sc.Method != "POST" {
		t.Errorf("Method = %q, want %q", sc.Method, "POST")
	}
	if sc.Path != "/echo" {
		t.Errorf("Path = %q, want %q", sc.Path, "/echo")
	}

	r = WithSecurityContext(r, sc)
	got := GetSecurityContext(r)
	if got != sc {
		t.Error("GetSecurityContext did not return the same context")
	}
}

func TestGetSecurityContext_Nil(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if sc := GetSecurityContext(r); sc != nil {
		t.Error("expected nil SecurityContext for request without context")
	}
}

func TestAuditEmitter_EmitsJSON(t *testing.T) {
	// Capture log output
	var logBuf strings.Builder
	restoreLog := captureLogOutput(&logBuf)
	defer restoreLog()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate middleware writing decisions
		sc := GetSecurityContext(r)
		if sc == nil {
			t.Fatal("SecurityContext should be available in inner handler")
		}
		sc.Auth = &AuthDecision{Outcome: "allow", Sub: "user1", Scope: "ws"}
		sc.InputScan = &ScanDecision{Risk: "high", Triggers: []string{"instruction_override"}}
		sc.RateLimit = &RateLimitDecision{Allowed: true}
		sc.BruteForce = &BruteForceDecision{Banned: false}
		w.WriteHeader(http.StatusOK)
	})

	handler := AuditEmitter(inner)

	req := httptest.NewRequest("POST", "/echo", nil)
	req.Header.Set("X-Request-ID", "audit-test-1")
	req.RemoteAddr = "192.168.1.1:9999"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := logBuf.String()
	if !strings.Contains(output, "[AUDIT]") {
		t.Fatalf("expected [AUDIT] prefix in log, got: %s", output)
	}

	// Extract the JSON part after "[AUDIT] "
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON found in audit log: %s", output)
	}
	jsonStr := output[idx:]
	// Trim trailing newline
	jsonStr = strings.TrimSpace(jsonStr)

	var rec2 auditRecord
	if err := json.Unmarshal([]byte(jsonStr), &rec2); err != nil {
		t.Fatalf("failed to parse audit JSON: %v\nraw: %s", err, jsonStr)
	}

	if rec2.Type != "gateway_audit" {
		t.Errorf("type = %q, want %q", rec2.Type, "gateway_audit")
	}
	if rec2.RequestID != "audit-test-1" {
		t.Errorf("request_id = %q, want %q", rec2.RequestID, "audit-test-1")
	}
	if rec2.Method != "POST" {
		t.Errorf("method = %q, want %q", rec2.Method, "POST")
	}
	if rec2.StatusCode != 200 {
		t.Errorf("status_code = %d, want 200", rec2.StatusCode)
	}
	if rec2.Auth == nil || rec2.Auth.Outcome != "allow" || rec2.Auth.Sub != "user1" {
		t.Errorf("auth = %+v, want allow/user1", rec2.Auth)
	}
	if rec2.InputScan == nil || rec2.InputScan.Risk != "high" {
		t.Errorf("input_scan = %+v, want risk=high", rec2.InputScan)
	}
	if rec2.RateLimit == nil || !rec2.RateLimit.Allowed {
		t.Errorf("rate_limit = %+v, want allowed=true", rec2.RateLimit)
	}
	if rec2.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", rec2.DurationMs)
	}
}

func TestAuditEmitter_SkipsHealthAndMetrics(t *testing.T) {
	var logBuf strings.Builder
	restoreLog := captureLogOutput(&logBuf)
	defer restoreLog()

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// SecurityContext should NOT be present for skipped paths
		if sc := GetSecurityContext(r); sc != nil {
			t.Error("SecurityContext should be nil for /health")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := AuditEmitter(inner)

	for _, path := range []string{"/health", "/metrics"} {
		called = false
		logBuf.Reset()
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Errorf("inner handler not called for %s", path)
		}
		if strings.Contains(logBuf.String(), "[AUDIT]") {
			t.Errorf("audit log emitted for %s, should be skipped", path)
		}
	}
}

func TestAuditEmitter_CapturesNon200Status(t *testing.T) {
	var logBuf strings.Builder
	restoreLog := captureLogOutput(&logBuf)
	defer restoreLog()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	handler := AuditEmitter(inner)
	req := httptest.NewRequest("GET", "/secret", nil)
	req.Header.Set("X-Request-ID", "status-test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := logBuf.String()
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON in audit log: %s", output)
	}

	var rec2 auditRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(output[idx:])), &rec2); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if rec2.StatusCode != 403 {
		t.Errorf("status_code = %d, want 403", rec2.StatusCode)
	}
}

func TestAuditEmitter_DefaultStatusOnWrite(t *testing.T) {
	// When handler calls Write() without WriteHeader(), status should be 200
	var logBuf strings.Builder
	restoreLog := captureLogOutput(&logBuf)
	defer restoreLog()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	handler := AuditEmitter(inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := logBuf.String()
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON in audit log: %s", output)
	}

	var rec2 auditRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(output[idx:])), &rec2); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if rec2.StatusCode != 200 {
		t.Errorf("status_code = %d, want 200", rec2.StatusCode)
	}
}

// captureLogOutput redirects log output to a buffer and returns a restore function.
func captureLogOutput(buf *strings.Builder) func() {
	origOut := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	return func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	}
}
