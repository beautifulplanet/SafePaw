package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetrics_RecordAndServe(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("GET", 200, "/health", 5*time.Millisecond)
	m.RecordRequest("POST", 200, "/ws/chat", 100*time.Millisecond)
	m.RecordRequest("GET", 429, "/ws/chat", 1*time.Millisecond)
	m.RecordInjection("high")
	m.RecordInjection("medium")
	m.RecordInjection("high")
	m.RecordRevocation()
	m.RecordRateLimited()
	m.RecordAuthFailure("missing_token")

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	checks := []string{
		`safepaw_requests_total{method="GET",status="200",path="/health"} 1`,
		`safepaw_requests_total{method="POST",status="200",path="/ws"} 1`,
		`safepaw_prompt_injection_detected_total{risk="high"} 2`,
		`safepaw_prompt_injection_detected_total{risk="medium"} 1`,
		`safepaw_tokens_revoked_total 1`,
		`safepaw_rate_limited_total 1`,
		`safepaw_auth_failures_total{reason="missing_token"} 1`,
		`safepaw_active_connections 0`,
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("metrics output missing: %q\n\nFull output:\n%s", check, body)
		}
	}

	if !strings.Contains(body, "safepaw_request_duration_seconds_bucket") {
		t.Error("missing histogram buckets in output")
	}
}

func TestMetricsMiddleware_RecordsStatus(t *testing.T) {
	m := NewMetrics()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := MetricsMiddleware(m, inner)

	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, `status="404"`) {
		t.Errorf("expected 404 status in metrics, got:\n%s", body)
	}
}

func TestMetrics_ConnectionGauge(t *testing.T) {
	m := NewMetrics()
	m.AddConnection()
	m.AddConnection()
	m.RemoveConnection()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "safepaw_active_connections 1") {
		t.Error("expected active_connections = 1")
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/ws/chat", "/ws"},
		{"/ws", "/ws"},
		{"/admin/revoke", "/admin"},
		{"/health", "/health"},
		{"/some/path", "/some/path"},
	}
	for _, tc := range tests {
		got := normalizePath(tc.input)
		if got != tc.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMetrics_ConcurrentCounter(t *testing.T) {
	m := NewMetrics()
	done := make(chan struct{})

	// Hammer the same key from multiple goroutines
	for i := 0; i < 10; i++ {
		go func() {
			m.RecordRequest("GET", 200, "/ws", time.Millisecond)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify no panic and handler works
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "safepaw_requests_total") {
		t.Error("expected safepaw_requests_total in output")
	}
}

func TestMetrics_DurationBuckets(t *testing.T) {
	m := NewMetrics()

	// Record a request with a duration that hits multiple buckets
	m.RecordRequest("POST", 201, "/api/chat", 500*time.Millisecond)
	m.RecordRequest("POST", 201, "/api/chat", 2*time.Second)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "safepaw_request_duration_seconds_bucket") {
		t.Error("expected duration bucket metrics")
	}
}

func TestMetrics_Handler_MalformedDurationKey(t *testing.T) {
	m := NewMetrics()

	// Inject a malformed key (no colon) directly into durations map
	// to cover the len(parts)!=2 continue branch in Handler().
	m.durations["badkey"] = newHistogram(m.durationBuckets)

	// Also add a valid entry so we confirm it still renders.
	m.RecordRequest("GET", 200, "/health", 5*time.Millisecond)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "safepaw_request_duration_seconds_bucket") {
		t.Error("expected valid duration bucket metrics despite malformed key")
	}
	// The malformed key should be silently skipped.
	if strings.Contains(body, "badkey") {
		t.Error("malformed key should not appear in output")
	}
}
