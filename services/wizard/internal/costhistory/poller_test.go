package costhistory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ── Mock Persister ──────────────────────────────────────────────────

type mockPersister struct {
	mu             sync.Mutex
	dailyUpserts   []DailySnapshot
	modelUpserts   []ModelSnapshot
	closed         bool
	dailyErr       error
	modelErr       error
}

func (m *mockPersister) UpsertDailySnapshot(_ context.Context, snap DailySnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dailyErr != nil {
		return m.dailyErr
	}
	m.dailyUpserts = append(m.dailyUpserts, snap)
	return nil
}

func (m *mockPersister) UpsertModelSnapshot(_ context.Context, snap ModelSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.modelErr != nil {
		return m.modelErr
	}
	m.modelUpserts = append(m.modelUpserts, snap)
	return nil
}

func (m *mockPersister) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockPersister) getDailyCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dailyUpserts)
}

func (m *mockPersister) getModelCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.modelUpserts)
}

// ── Mock env reader ─────────────────────────────────────────────────

func mockEnvReader(secret string) EnvReader {
	return func() (map[string]string, error) {
		return map[string]string{"AUTH_SECRET": secret}, nil
	}
}

// ── Test fetchUsage ────────────────────────────────────────────────

func TestPoller_fetchUsage_OK(t *testing.T) {
	// Mock gateway that returns a valid usage response
	resp := gatewayUsageResponse{
		Status: "ok",
		Daily: []struct {
			Date        string  `json:"date"`
			TotalCost   float64 `json:"totalCost"`
			TotalTokens int64   `json:"totalTokens"`
			Input       int64   `json:"input"`
			Output      int64   `json:"output"`
		}{
			{Date: "2025-01-15", TotalCost: 0.50, TotalTokens: 10000, Input: 6000, Output: 4000},
		},
		Models: []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Count    int    `json:"count"`
			Totals   struct {
				TotalCost   float64 `json:"totalCost"`
				TotalTokens int64   `json:"totalTokens"`
				Input       int64   `json:"input"`
				Output      int64   `json:"output"`
			} `json:"totals"`
		}{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Count: 5},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("missing Authorization header")
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// 32-byte secret for HMAC
	secret := "abcdefghijklmnopqrstuvwxyz123456"

	p := &Poller{
		gatewayURL: srv.URL,
		envReader:  mockEnvReader(secret),
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	ctx := context.Background()
	data, err := p.fetchUsage(ctx)
	if err != nil {
		t.Fatalf("fetchUsage: %v", err)
	}
	if data.Status != "ok" {
		t.Errorf("status = %q, want ok", data.Status)
	}
	if len(data.Daily) != 1 {
		t.Fatalf("daily entries = %d, want 1", len(data.Daily))
	}
	if data.Daily[0].Date != "2025-01-15" {
		t.Errorf("daily[0].date = %q, want 2025-01-15", data.Daily[0].Date)
	}
	if len(data.Models) != 1 {
		t.Fatalf("model entries = %d, want 1", len(data.Models))
	}
	if data.Models[0].Provider != "anthropic" {
		t.Errorf("models[0].provider = %q, want anthropic", data.Models[0].Provider)
	}
}

func TestPoller_fetchUsage_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := &Poller{
		gatewayURL: srv.URL,
		envReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
}

func TestPoller_fetchUsage_StatusNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "error"})
	}))
	defer srv.Close()

	p := &Poller{
		gatewayURL: srv.URL,
		envReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected error from non-ok status")
	}
}

func TestPoller_fetchUsage_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := &Poller{
		gatewayURL: srv.URL,
		envReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected error from invalid JSON")
	}
}

func TestPoller_fetchUsage_ShortSecret(t *testing.T) {
	p := &Poller{
		gatewayURL: "http://localhost:9999",
		envReader:  mockEnvReader("short"), // < 32 chars
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected error from short AUTH_SECRET")
	}
}

func TestPoller_fetchUsage_NoSecret(t *testing.T) {
	p := &Poller{
		gatewayURL: "http://localhost:9999",
		envReader:  mockEnvReader(""), // empty
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected error from empty AUTH_SECRET")
	}
}

func TestPoller_fetchUsage_EnvReaderError(t *testing.T) {
	p := &Poller{
		gatewayURL: "http://localhost:9999",
		envReader: func() (map[string]string, error) {
			return nil, context.DeadlineExceeded
		},
		client: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected error from env reader failure")
	}
}

// ── Test persist ───────────────────────────────────────────────────

func TestPoller_persist_DailyOnly(t *testing.T) {
	store := &mockPersister{}
	p := &Poller{store: store}

	data := &gatewayUsageResponse{
		Daily: []struct {
			Date        string  `json:"date"`
			TotalCost   float64 `json:"totalCost"`
			TotalTokens int64   `json:"totalTokens"`
			Input       int64   `json:"input"`
			Output      int64   `json:"output"`
		}{
			{Date: "2025-01-15", TotalCost: 0.50, TotalTokens: 10000, Input: 6000, Output: 4000},
			{Date: "2025-01-14", TotalCost: 0.30, TotalTokens: 5000, Input: 3000, Output: 2000},
		},
	}

	if err := p.persist(context.Background(), data); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := store.getDailyCount(); got != 2 {
		t.Errorf("daily upserts = %d, want 2", got)
	}
	if store.dailyUpserts[0].Date != "2025-01-15" {
		t.Errorf("daily[0].date = %q, want 2025-01-15", store.dailyUpserts[0].Date)
	}
	if store.dailyUpserts[0].PromptTokens != 6000 {
		t.Errorf("daily[0].PromptTokens = %d, want 6000", store.dailyUpserts[0].PromptTokens)
	}
}

func TestPoller_persist_SessionsPreferred(t *testing.T) {
	store := &mockPersister{}
	p := &Poller{store: store}

	data := &gatewayUsageResponse{
		// Both daily and sessions data present — sessions should win
		Daily: []struct {
			Date        string  `json:"date"`
			TotalCost   float64 `json:"totalCost"`
			TotalTokens int64   `json:"totalTokens"`
			Input       int64   `json:"input"`
			Output      int64   `json:"output"`
		}{
			{Date: "2025-01-15", TotalCost: 0.50, TotalTokens: 10000},
		},
		Sessions: &struct {
			Daily []struct {
				Date      string  `json:"date"`
				Tokens    int64   `json:"tokens"`
				Cost      float64 `json:"cost"`
				Messages  int     `json:"messages"`
				ToolCalls int     `json:"toolCalls"`
			} `json:"daily"`
		}{
			Daily: []struct {
				Date      string  `json:"date"`
				Tokens    int64   `json:"tokens"`
				Cost      float64 `json:"cost"`
				Messages  int     `json:"messages"`
				ToolCalls int     `json:"toolCalls"`
			}{
				{Date: "2025-01-15", Tokens: 9500, Cost: 0.48, Messages: 25, ToolCalls: 7},
			},
		},
	}

	if err := p.persist(context.Background(), data); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := store.getDailyCount(); got != 1 {
		t.Errorf("daily upserts = %d, want 1", got)
	}
	// Sessions data should have been used (has Messages and ToolCalls)
	snap := store.dailyUpserts[0]
	if snap.Messages != 25 {
		t.Errorf("Messages = %d, want 25 (from sessions data)", snap.Messages)
	}
	if snap.ToolCalls != 7 {
		t.Errorf("ToolCalls = %d, want 7 (from sessions data)", snap.ToolCalls)
	}
	if snap.TotalCostUSD != 0.48 {
		t.Errorf("TotalCostUSD = %f, want 0.48", snap.TotalCostUSD)
	}
}

func TestPoller_persist_ModelsSkipEmpty(t *testing.T) {
	store := &mockPersister{}
	p := &Poller{store: store}

	data := &gatewayUsageResponse{
		Models: []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Count    int    `json:"count"`
			Totals   struct {
				TotalCost   float64 `json:"totalCost"`
				TotalTokens int64   `json:"totalTokens"`
				Input       int64   `json:"input"`
				Output      int64   `json:"output"`
			} `json:"totals"`
		}{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Count: 5},
			{Provider: "", Model: "", Count: 0}, // should be skipped
			{Provider: "openai", Model: "gpt-4o", Count: 3},
		},
	}

	if err := p.persist(context.Background(), data); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := store.getModelCount(); got != 2 {
		t.Errorf("model upserts = %d, want 2 (empty entry skipped)", got)
	}
}

func TestPoller_persist_EmptyData(t *testing.T) {
	store := &mockPersister{}
	p := &Poller{store: store}

	data := &gatewayUsageResponse{Status: "ok"}

	if err := p.persist(context.Background(), data); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := store.getDailyCount(); got != 0 {
		t.Errorf("daily upserts = %d, want 0", got)
	}
	if got := store.getModelCount(); got != 0 {
		t.Errorf("model upserts = %d, want 0", got)
	}
}

func TestPoller_persist_DailyUpsertError(t *testing.T) {
	store := &mockPersister{dailyErr: context.DeadlineExceeded}
	p := &Poller{store: store}

	data := &gatewayUsageResponse{
		Daily: []struct {
			Date        string  `json:"date"`
			TotalCost   float64 `json:"totalCost"`
			TotalTokens int64   `json:"totalTokens"`
			Input       int64   `json:"input"`
			Output      int64   `json:"output"`
		}{
			{Date: "2025-01-15"},
		},
	}

	err := p.persist(context.Background(), data)
	if err == nil {
		t.Fatal("expected error from failed daily upsert")
	}
}

func TestPoller_persist_ModelUpsertError(t *testing.T) {
	store := &mockPersister{modelErr: context.DeadlineExceeded}
	p := &Poller{store: store}

	data := &gatewayUsageResponse{
		Models: []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Count    int    `json:"count"`
			Totals   struct {
				TotalCost   float64 `json:"totalCost"`
				TotalTokens int64   `json:"totalTokens"`
				Input       int64   `json:"input"`
				Output      int64   `json:"output"`
			} `json:"totals"`
		}{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Count: 5},
		},
	}

	err := p.persist(context.Background(), data)
	if err == nil {
		t.Fatal("expected error from failed model upsert")
	}
}

// ── Test poll (integration of fetch + persist) ─────────────────────

func TestPoller_poll_EndToEnd(t *testing.T) {
	store := &mockPersister{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(gatewayUsageResponse{
			Status: "ok",
			Daily: []struct {
				Date        string  `json:"date"`
				TotalCost   float64 `json:"totalCost"`
				TotalTokens int64   `json:"totalTokens"`
				Input       int64   `json:"input"`
				Output      int64   `json:"output"`
			}{
				{Date: "2025-01-15", TotalCost: 0.50, TotalTokens: 10000, Input: 6000, Output: 4000},
			},
			Models: []struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
				Count    int    `json:"count"`
				Totals   struct {
					TotalCost   float64 `json:"totalCost"`
					TotalTokens int64   `json:"totalTokens"`
					Input       int64   `json:"input"`
					Output      int64   `json:"output"`
				} `json:"totals"`
			}{
				{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Count: 10},
			},
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := &Poller{
		store:      store,
		gatewayURL: srv.URL,
		envReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		client:     &http.Client{Timeout: 5 * time.Second},
		ctx:        ctx,
		cancel:     cancel,
	}

	p.poll()

	// Verify data was persisted
	if got := store.getDailyCount(); got != 1 {
		t.Errorf("daily upserts = %d, want 1", got)
	}
	if got := store.getModelCount(); got != 1 {
		t.Errorf("model upserts = %d, want 1", got)
	}

	// Status should reflect success
	lastOK, lastErr := p.Status()
	if lastOK.IsZero() {
		t.Error("lastOK should be set after successful poll")
	}
	if lastErr != nil {
		t.Errorf("lastErr should be nil, got %v", lastErr)
	}
}

func TestPoller_poll_FetchError(t *testing.T) {
	store := &mockPersister{}

	// Server that always returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := &Poller{
		store:      store,
		gatewayURL: srv.URL,
		envReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		client:     &http.Client{Timeout: 5 * time.Second},
		ctx:        ctx,
		cancel:     cancel,
	}

	p.poll()

	// Status should reflect failure
	_, lastErr := p.Status()
	if lastErr == nil {
		t.Error("lastErr should be set after failed poll")
	}

	// No data should have been persisted
	if got := store.getDailyCount(); got != 0 {
		t.Errorf("daily upserts = %d, want 0", got)
	}
}

// ── Test NewPoller / Stop lifecycle ─────────────────────────────────

func TestNewPoller_DefaultInterval(t *testing.T) {
	store := &mockPersister{}

	// The poller will try to fetch after 30s delay, but we stop it immediately
	p := NewPoller(PollerConfig{
		Store:      store,
		GatewayURL: "http://192.0.2.1:9999", // unreachable, doesn't matter
		EnvReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		// Interval = 0 → should default to 5 minutes
	})

	// Stop immediately
	p.Stop()

	// Verify the poller was created (non-nil)
	if p == nil {
		t.Fatal("NewPoller returned nil")
	}
}

func TestPoller_Stop_Idempotent(t *testing.T) {
	store := &mockPersister{}
	p := NewPoller(PollerConfig{
		Store:      store,
		GatewayURL: "http://192.0.2.1:9999",
		EnvReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		Interval:   time.Hour, // won't trigger
	})

	// Stop multiple times — should not panic
	p.Stop()
	p.Stop()
}

// ── Test token minting (via fetchUsage auth header) ────────────────

func TestPoller_fetchUsage_AuthHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(gatewayUsageResponse{Status: "ok"})
	}))
	defer srv.Close()

	p := &Poller{
		gatewayURL: srv.URL,
		envReader:  mockEnvReader("abcdefghijklmnopqrstuvwxyz123456"),
		client:     &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.fetchUsage(context.Background())
	if err != nil {
		t.Fatalf("fetchUsage: %v", err)
	}

	if gotAuth == "" {
		t.Fatal("Authorization header was empty")
	}
	if len(gotAuth) < 10 {
		t.Errorf("Authorization header too short: %q", gotAuth)
	}
	// Should be "Bearer <payload>.<sig>"
	if gotAuth[:7] != "Bearer " {
		t.Errorf("Authorization should start with 'Bearer ', got %q", gotAuth[:7])
	}
}

// ── Test persist with sessions.daily having empty entries ───────────

func TestPoller_persist_SessionsEmptyDaily(t *testing.T) {
	store := &mockPersister{}
	p := &Poller{store: store}

	// Sessions present but with empty daily slice — should fall through to Daily
	data := &gatewayUsageResponse{
		Daily: []struct {
			Date        string  `json:"date"`
			TotalCost   float64 `json:"totalCost"`
			TotalTokens int64   `json:"totalTokens"`
			Input       int64   `json:"input"`
			Output      int64   `json:"output"`
		}{
			{Date: "2025-01-14", TotalCost: 0.30, TotalTokens: 5000, Input: 3000, Output: 2000},
		},
		Sessions: &struct {
			Daily []struct {
				Date      string  `json:"date"`
				Tokens    int64   `json:"tokens"`
				Cost      float64 `json:"cost"`
				Messages  int     `json:"messages"`
				ToolCalls int     `json:"toolCalls"`
			} `json:"daily"`
		}{
			Daily: nil, // empty
		},
	}

	if err := p.persist(context.Background(), data); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := store.getDailyCount(); got != 1 {
		t.Errorf("daily upserts = %d, want 1 (fallback to Daily)", got)
	}
	// Should be from the fallback Daily data, not sessions
	if store.dailyUpserts[0].PromptTokens != 3000 {
		t.Errorf("PromptTokens = %d, want 3000 (from fallback Daily)", store.dailyUpserts[0].PromptTokens)
	}
}
