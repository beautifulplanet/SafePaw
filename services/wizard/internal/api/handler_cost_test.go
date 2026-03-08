package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"safepaw/wizard/internal/costhistory"
	"safepaw/wizard/internal/middleware"
)

func asViewer(r *http.Request) *http.Request {
	return middleware.SetRole(r, "viewer")
}

// mockQuerier implements costhistory.Querier for tests.
type mockQuerier struct {
	dailyRows []costhistory.DailyRow
	modelRows []costhistory.ModelRow
	trend     *costhistory.TrendResult
	err       error
}

func (m *mockQuerier) ListDailySnapshots(_ context.Context, _ int) ([]costhistory.DailyRow, error) {
	return m.dailyRows, m.err
}

func (m *mockQuerier) ListModelSnapshots(_ context.Context, _ int) ([]costhistory.ModelRow, error) {
	return m.modelRows, m.err
}

func (m *mockQuerier) GetTrend(_ context.Context, _ int) (*costhistory.TrendResult, error) {
	return m.trend, m.err
}

// decodeMap is a test helper that decodes JSON body into a map.
func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return m
}

// --- handleCostHistory ---

func TestCostHistory_Unavailable(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/history", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	m := decodeMap(t, rec)
	if m["status"] != "unavailable" {
		t.Errorf("status = %v, want unavailable", m["status"])
	}
}

func TestCostHistory_Success(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{
		dailyRows: []costhistory.DailyRow{
			{Date: "2025-01-15", TotalTokens: 1000, TotalCostUSD: 0.05, PromptTokens: 600, CompletionTk: 400, Messages: 10, ToolCalls: 2},
			{Date: "2025-01-14", TotalTokens: 800, TotalCostUSD: 0.04, PromptTokens: 500, CompletionTk: 300, Messages: 8, ToolCalls: 1},
		},
	})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/history?days=7", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	m := decodeMap(t, rec)
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
	if m["days"] != float64(7) {
		t.Errorf("days = %v, want 7", m["days"])
	}
	daily, ok := m["daily"].([]interface{})
	if !ok || len(daily) != 2 {
		t.Fatalf("daily has %d rows, want 2", len(daily))
	}
}

func TestCostHistory_Empty(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{dailyRows: nil})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/history", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
	daily, ok := m["daily"].([]interface{})
	if !ok || len(daily) != 0 {
		t.Errorf("daily should be empty array, got %v", m["daily"])
	}
}

func TestCostHistory_DBError(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{err: errors.New("connection refused")})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/history", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "error" {
		t.Errorf("status = %v, want error", m["status"])
	}
	if m["error"] != "database query failed" {
		t.Errorf("error = %v, want 'database query failed'", m["error"])
	}
}

// --- handleCostModels ---

func TestCostModels_Unavailable(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/models", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "unavailable" {
		t.Errorf("status = %v, want unavailable", m["status"])
	}
}

func TestCostModels_Success(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{
		modelRows: []costhistory.ModelRow{
			{Date: "2025-01-15", Provider: "anthropic", Model: "claude-sonnet-4-20250514", RequestCount: 5, TotalTokens: 5000, TotalCostUSD: 0.15},
			{Date: "2025-01-15", Provider: "openai", Model: "gpt-4o", RequestCount: 3, TotalTokens: 2000, TotalCostUSD: 0.08},
		},
	})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/models?days=14", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
	if m["days"] != float64(14) {
		t.Errorf("days = %v, want 14", m["days"])
	}
	models, ok := m["models"].([]interface{})
	if !ok || len(models) != 2 {
		t.Fatalf("models has %d rows, want 2", len(models))
	}
}

func TestCostModels_DBError(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{err: errors.New("timeout")})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/models", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "error" {
		t.Errorf("status = %v, want error", m["status"])
	}
}

// --- handleCostTrends ---

func TestCostTrends_Unavailable(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/trends", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "unavailable" {
		t.Errorf("status = %v, want unavailable", m["status"])
	}
}

func TestCostTrends_Success(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{
		trend: &costhistory.TrendResult{
			RecentDays:     7,
			RecentCost:     1.25,
			RecentTokens:   50000,
			PriorCost:      0.80,
			PriorTokens:    30000,
			CostChangeP:    56.25,
			TokenChangeP:   66.67,
			DailyAvgRecent: 0.178,
			DailyAvgPrior:  0.114,
			AnomalyScore:   0.36,
		},
	})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/trends?days=7", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
	trend, ok := m["trend"].(map[string]interface{})
	if !ok {
		t.Fatal("trend should be an object")
	}
	if trend["recentDays"] != float64(7) {
		t.Errorf("recentDays = %v, want 7", trend["recentDays"])
	}
	if trend["anomalyScore"] != 0.36 {
		t.Errorf("anomalyScore = %v, want 0.36", trend["anomalyScore"])
	}
}

func TestCostTrends_DBError(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{err: errors.New("db down")})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/trends", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	m := decodeMap(t, rec)
	if m["status"] != "error" {
		t.Errorf("status = %v, want error", m["status"])
	}
}

// --- parseDays ---

func TestParseDays_Default(t *testing.T) {
	r := httptest.NewRequest("GET", "/test", nil)
	got := parseDays(r, 30, 365)
	if got != 30 {
		t.Errorf("parseDays empty = %d, want 30", got)
	}
}

func TestParseDays_Valid(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?days=14", nil)
	got := parseDays(r, 30, 365)
	if got != 14 {
		t.Errorf("parseDays(14) = %d, want 14", got)
	}
}

func TestParseDays_InvalidString(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?days=abc", nil)
	got := parseDays(r, 30, 365)
	if got != 30 {
		t.Errorf("parseDays(abc) = %d, want default 30", got)
	}
}

func TestParseDays_Negative(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?days=-5", nil)
	got := parseDays(r, 30, 365)
	if got != 30 {
		t.Errorf("parseDays(-5) = %d, want default 30", got)
	}
}

func TestParseDays_Zero(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?days=0", nil)
	got := parseDays(r, 7, 90)
	if got != 7 {
		t.Errorf("parseDays(0) = %d, want default 7", got)
	}
}

func TestParseDays_ClampToMax(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?days=999", nil)
	got := parseDays(r, 30, 365)
	if got != 365 {
		t.Errorf("parseDays(999) = %d, want max 365", got)
	}
}

func TestParseDays_ExactMax(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?days=90", nil)
	got := parseDays(r, 7, 90)
	if got != 90 {
		t.Errorf("parseDays(90) = %d, want 90", got)
	}
}

// --- RBAC: viewer/operator should also have access ---

func TestCostHistory_ViewerAccess(t *testing.T) {
	h := newTestHandler(t)
	h.SetCostQuerier(&mockQuerier{dailyRows: []costhistory.DailyRow{}})
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/cost/history", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asViewer(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("viewer status = %d, want 200", rec.Code)
	}
	m := decodeMap(t, rec)
	if m["status"] != "ok" {
		t.Errorf("viewer status = %v, want ok", m["status"])
	}
}
