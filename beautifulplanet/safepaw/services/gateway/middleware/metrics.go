// =============================================================
// SafePaw Gateway - Prometheus Metrics (Zero-Dependency)
// =============================================================
// Lightweight metrics collector that exposes a /metrics endpoint
// in Prometheus text exposition format. No external dependencies.
//
// WHAT WE TRACK:
//   - safepaw_requests_total (counter, by method+status+path)
//   - safepaw_request_duration_seconds (histogram, by method+path)
//   - safepaw_prompt_injection_detected_total (counter, by risk)
//   - safepaw_tokens_revoked_total (counter)
//   - safepaw_rate_limited_total (counter)
//   - safepaw_auth_failures_total (counter, by reason)
//   - safepaw_active_connections (gauge)
//
// WHY NOT prometheus/client_golang?
//   Zero external dependencies policy. This hand-rolled collector
//   is <200 lines and covers the 80% case. Swap in the official
//   client when the team outgrows this.
// =============================================================

package middleware

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects gateway telemetry in a thread-safe manner.
type Metrics struct {
	mu sync.RWMutex

	requestsTotal     map[string]*int64 // "method:status:path" → count
	injectionDetected map[string]*int64 // "risk_level" → count
	revokedTotal      int64
	rateLimitedTotal  int64
	authFailures      map[string]*int64 // "reason" → count
	activeConns       int64

	// Duration histogram buckets (seconds)
	durationBuckets []float64
	durations       map[string]*histogram // "method:path" → histogram
}

type histogram struct {
	buckets []int64 // count per bucket
	sum     float64 // total seconds
	count   int64
	mu      sync.Mutex
}

func newHistogram(buckets []float64) *histogram {
	return &histogram{
		buckets: make([]int64, len(buckets)),
	}
}

func (h *histogram) observe(seconds float64, buckets []float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += seconds
	h.count++
	for i, bound := range buckets {
		if seconds <= bound {
			h.buckets[i]++
		}
	}
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		requestsTotal:     make(map[string]*int64),
		injectionDetected: make(map[string]*int64),
		authFailures:      make(map[string]*int64),
		durationBuckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		durations:         make(map[string]*histogram),
	}
}

func (m *Metrics) counter(store map[string]*int64, key string) {
	m.mu.RLock()
	ptr, ok := store[key]
	m.mu.RUnlock()
	if ok {
		atomic.AddInt64(ptr, 1)
		return
	}
	m.mu.Lock()
	if ptr, ok = store[key]; ok {
		m.mu.Unlock()
		atomic.AddInt64(ptr, 1)
		return
	}
	var v int64 = 1
	store[key] = &v
	m.mu.Unlock()
}

// RecordRequest records a completed HTTP request.
func (m *Metrics) RecordRequest(method string, status int, path string, duration time.Duration) {
	normPath := normalizePath(path)
	key := fmt.Sprintf("%s:%d:%s", method, status, normPath)
	m.counter(m.requestsTotal, key)

	durKey := method + ":" + normPath
	m.mu.RLock()
	h, ok := m.durations[durKey]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		if h, ok = m.durations[durKey]; !ok {
			h = newHistogram(m.durationBuckets)
			m.durations[durKey] = h
		}
		m.mu.Unlock()
	}
	h.observe(duration.Seconds(), m.durationBuckets)
}

// RecordInjection records a prompt injection detection event.
func (m *Metrics) RecordInjection(risk string) {
	m.counter(m.injectionDetected, risk)
}

// RecordRevocation records a token revocation event.
func (m *Metrics) RecordRevocation() {
	atomic.AddInt64(&m.revokedTotal, 1)
}

// RecordRateLimited records a rate-limited request.
func (m *Metrics) RecordRateLimited() {
	atomic.AddInt64(&m.rateLimitedTotal, 1)
}

// RecordAuthFailure records an authentication failure by reason.
func (m *Metrics) RecordAuthFailure(reason string) {
	m.counter(m.authFailures, reason)
}

// AddConnection increments active connection gauge.
func (m *Metrics) AddConnection() { atomic.AddInt64(&m.activeConns, 1) }

// RemoveConnection decrements active connection gauge.
func (m *Metrics) RemoveConnection() { atomic.AddInt64(&m.activeConns, -1) }

// Handler returns an http.Handler that serves /metrics in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		var b strings.Builder

		// requests_total
		b.WriteString("# HELP safepaw_requests_total Total HTTP requests processed.\n")
		b.WriteString("# TYPE safepaw_requests_total counter\n")
		m.mu.RLock()
		keys := sortedKeys(m.requestsTotal)
		for _, k := range keys {
			parts := strings.SplitN(k, ":", 3)
			if len(parts) == 3 {
				v := atomic.LoadInt64(m.requestsTotal[k])
				fmt.Fprintf(&b, "safepaw_requests_total{method=%q,status=%q,path=%q} %d\n",
					parts[0], parts[1], parts[2], v)
			}
		}
		m.mu.RUnlock()

		// prompt_injection_detected_total
		b.WriteString("# HELP safepaw_prompt_injection_detected_total Prompt injection detections by risk level.\n")
		b.WriteString("# TYPE safepaw_prompt_injection_detected_total counter\n")
		m.mu.RLock()
		for _, k := range sortedKeys(m.injectionDetected) {
			v := atomic.LoadInt64(m.injectionDetected[k])
			fmt.Fprintf(&b, "safepaw_prompt_injection_detected_total{risk=%q} %d\n", k, v)
		}
		m.mu.RUnlock()

		// tokens_revoked_total
		b.WriteString("# HELP safepaw_tokens_revoked_total Total tokens revoked.\n")
		b.WriteString("# TYPE safepaw_tokens_revoked_total counter\n")
		fmt.Fprintf(&b, "safepaw_tokens_revoked_total %d\n", atomic.LoadInt64(&m.revokedTotal))

		// rate_limited_total
		b.WriteString("# HELP safepaw_rate_limited_total Total rate-limited requests.\n")
		b.WriteString("# TYPE safepaw_rate_limited_total counter\n")
		fmt.Fprintf(&b, "safepaw_rate_limited_total %d\n", atomic.LoadInt64(&m.rateLimitedTotal))

		// auth_failures_total
		b.WriteString("# HELP safepaw_auth_failures_total Authentication failures by reason.\n")
		b.WriteString("# TYPE safepaw_auth_failures_total counter\n")
		m.mu.RLock()
		for _, k := range sortedKeys(m.authFailures) {
			v := atomic.LoadInt64(m.authFailures[k])
			fmt.Fprintf(&b, "safepaw_auth_failures_total{reason=%q} %d\n", k, v)
		}
		m.mu.RUnlock()

		// active_connections
		b.WriteString("# HELP safepaw_active_connections Current active connections.\n")
		b.WriteString("# TYPE safepaw_active_connections gauge\n")
		fmt.Fprintf(&b, "safepaw_active_connections %d\n", atomic.LoadInt64(&m.activeConns))

		// request_duration_seconds (histogram)
		b.WriteString("# HELP safepaw_request_duration_seconds Request duration histogram.\n")
		b.WriteString("# TYPE safepaw_request_duration_seconds histogram\n")
		m.mu.RLock()
		durKeys := make([]string, 0, len(m.durations))
		for k := range m.durations {
			durKeys = append(durKeys, k)
		}
		sort.Strings(durKeys)
		for _, k := range durKeys {
			h := m.durations[k]
			parts := strings.SplitN(k, ":", 2)
			if len(parts) != 2 {
				continue
			}
			method, path := parts[0], parts[1]
			h.mu.Lock()
			for i, bound := range m.durationBuckets {
				fmt.Fprintf(&b, "safepaw_request_duration_seconds_bucket{method=%q,path=%q,le=\"%.3f\"} %d\n",
					method, path, bound, h.buckets[i])
			}
			fmt.Fprintf(&b, "safepaw_request_duration_seconds_bucket{method=%q,path=%q,le=\"+Inf\"} %d\n",
				method, path, h.count)
			fmt.Fprintf(&b, "safepaw_request_duration_seconds_sum{method=%q,path=%q} %.6f\n",
				method, path, h.sum)
			fmt.Fprintf(&b, "safepaw_request_duration_seconds_count{method=%q,path=%q} %d\n",
				method, path, h.count)
			h.mu.Unlock()
		}
		m.mu.RUnlock()

		w.Write([]byte(b.String()))
	})
}

// MetricsMiddleware records request metrics for all traffic.
func MetricsMiddleware(m *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		m.AddConnection()
		defer m.RemoveConnection()
		next.ServeHTTP(sw, r)
		m.RecordRequest(r.Method, sw.status, r.URL.Path, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wrote {
		sw.status = code
		sw.wrote = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

// normalizePath reduces cardinality by collapsing path parameters.
func normalizePath(p string) string {
	if strings.HasPrefix(p, "/ws") {
		return "/ws"
	}
	if strings.HasPrefix(p, "/admin/") {
		return "/admin"
	}
	return p
}

func sortedKeys(m map[string]*int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
