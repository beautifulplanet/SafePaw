package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestNewUsageCollector_Disabled(t *testing.T) {
	// Empty URL/token → disabled collector, no goroutine
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	if uc.status != StatusDisabled {
		t.Errorf("expected status=disabled, got %s", uc.status)
	}

	snap := uc.Snapshot()
	if snap.Status != "unavailable" {
		t.Errorf("expected snapshot status=unavailable, got %s", snap.Status)
	}
	if snap.Collector != string(StatusDisabled) {
		t.Errorf("expected collector=disabled, got %s", snap.Collector)
	}
}

func TestSnapshot_WithData(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")

	// Inject data directly
	uc.mu.Lock()
	uc.data = &CostUsageSummary{
		UpdatedAt: time.Now(),
		Days:      7,
		Daily: []CostUsageDailyEntry{
			{Date: today, CostUsageTotals: CostUsageTotals{TotalCost: 0.50}},
		},
		Totals: CostUsageTotals{TotalCost: 3.50},
	}
	uc.mu.Unlock()

	snap := uc.Snapshot()
	if snap.Status != "ok" {
		t.Errorf("expected status=ok, got %s", snap.Status)
	}
	if snap.TodayCost != 0.50 {
		t.Errorf("expected todayCost=0.50, got %f", snap.TodayCost)
	}
	if snap.PeriodCost != 3.50 {
		t.Errorf("expected periodCost=3.50, got %f", snap.PeriodCost)
	}
	if snap.Alert != "ok" {
		t.Errorf("expected alert=ok, got %s", snap.Alert)
	}
}

func TestSnapshot_AlertLevels(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")

	tests := []struct {
		todayCost float64
		wantAlert string
	}{
		{0.50, "ok"},
		{1.00, "warning"},
		{5.00, "warning"},
		{10.00, "critical"},
		{99.00, "critical"},
	}

	for _, tc := range tests {
		uc.mu.Lock()
		uc.data = &CostUsageSummary{
			UpdatedAt: time.Now(),
			Days:      1,
			Daily: []CostUsageDailyEntry{
				{Date: today, CostUsageTotals: CostUsageTotals{TotalCost: tc.todayCost}},
			},
			Totals: CostUsageTotals{TotalCost: tc.todayCost},
		}
		uc.mu.Unlock()

		snap := uc.Snapshot()
		if snap.Alert != tc.wantAlert {
			t.Errorf("cost=$%.2f: expected alert=%s, got %s", tc.todayCost, tc.wantAlert, snap.Alert)
		}
	}
}

func TestProcessUsageResponse_Success(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt": float64(time.Now().UnixMilli()),
		"days":      7,
		"daily": []map[string]interface{}{
			{"date": "2026-03-07", "totalCost": 1.23, "totalTokens": 1000},
		},
		"totals": map[string]interface{}{
			"totalCost":   5.67,
			"totalTokens": 50000,
		},
	})

	resp := wsResponse{
		Type:    "res",
		ID:      "test-1",
		OK:      true,
		Payload: json.RawMessage(payload),
	}

	if err := uc.processUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uc.mu.RLock()
	defer uc.mu.RUnlock()
	if uc.data == nil {
		t.Fatal("expected data to be populated")
	}
	if uc.data.Days != 7 {
		t.Errorf("expected days=7, got %d", uc.data.Days)
	}
	if uc.data.Totals.TotalCost != 5.67 {
		t.Errorf("expected totalCost=5.67, got %f", uc.data.Totals.TotalCost)
	}
}

func TestProcessUsageResponse_Error(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	resp := wsResponse{
		Type:  "res",
		ID:    "test-2",
		OK:    false,
		Error: &wsError{Code: "auth", Message: "invalid token"},
	}

	err := uc.processUsageResponse(resp)
	if err == nil {
		t.Fatal("expected error for failed response")
	}
	if err.Error() != "usage.cost error: invalid token" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestProcessUsageResponse_BadJSON(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	resp := wsResponse{
		Type:    "res",
		ID:      "test-3",
		OK:      true,
		Payload: json.RawMessage(`{not valid json}`),
	}

	err := uc.processUsageResponse(resp)
	if err == nil {
		t.Fatal("expected error for bad JSON payload")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"ab", 1, "a..."},
	}
	for _, tc := range tests {
		got := truncate(tc.input, tc.n)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.want)
		}
	}
}

func TestPricingTable_NotEmpty(t *testing.T) {
	if len(PricingTable) == 0 {
		t.Fatal("PricingTable should not be empty")
	}
	for _, p := range PricingTable {
		if p.Provider == "" || p.Model == "" {
			t.Errorf("pricing entry has empty provider/model: %+v", p)
		}
		if p.InputPerM <= 0 || p.OutputPerM <= 0 {
			t.Errorf("pricing entry has non-positive cost: %+v", p)
		}
	}
}

func TestNewUsageCollector_EnabledThenStop(t *testing.T) {
	// A valid URL + token starts the collector goroutine; Stop() cancels it.
	uc := NewUsageCollector("ws://127.0.0.1:1/fake", "token", 1.0, 10.0)
	if uc.status != StatusConnecting {
		t.Errorf("expected status=connecting, got %s", uc.status)
	}
	uc.Stop()
	// Give goroutine time to exit
	time.Sleep(50 * time.Millisecond)
}

func TestSnapshot_NoDaily(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	uc.mu.Lock()
	uc.data = &CostUsageSummary{
		UpdatedAt: time.Now(),
		Days:      7,
		Daily:     nil,
		Totals:    CostUsageTotals{TotalCost: 2.00},
	}
	uc.mu.Unlock()

	snap := uc.Snapshot()
	if snap.TodayCost != 0 {
		t.Errorf("expected todayCost=0 with no daily entries, got %f", snap.TodayCost)
	}
	if snap.Alert != "ok" {
		t.Errorf("expected alert=ok, got %s", snap.Alert)
	}
}

func TestSnapshot_YesterdayOnly(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	uc.mu.Lock()
	uc.data = &CostUsageSummary{
		UpdatedAt: time.Now(),
		Days:      7,
		Daily: []CostUsageDailyEntry{
			{Date: yesterday, CostUsageTotals: CostUsageTotals{TotalCost: 5.00}},
		},
		Totals: CostUsageTotals{TotalCost: 5.00},
	}
	uc.mu.Unlock()

	snap := uc.Snapshot()
	if snap.TodayCost != 0 {
		t.Errorf("expected todayCost=0 when last entry is yesterday, got %f", snap.TodayCost)
	}
}

func TestProcessUsageResponse_ErrorNoDetail(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	resp := wsResponse{Type: "res", ID: "test-4", OK: false}
	err := uc.processUsageResponse(resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "usage.cost error: unknown" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCollectorStatus_String(t *testing.T) {
	tests := []struct {
		s    CollectorStatus
		want string
	}{
		{StatusDisabled, "disabled"},
		{StatusConnecting, "connecting"},
		{StatusConnected, "connected"},
		{StatusError, "error"},
		{StatusUnavailable, "unavailable"},
	}
	for _, tc := range tests {
		if string(tc.s) != tc.want {
			t.Errorf("CollectorStatus %q != %q", tc.s, tc.want)
		}
	}
}

func TestProcessUsageResponse_CriticalAlert(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 5.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")
	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt": float64(time.Now().UnixMilli()),
		"days":      1,
		"daily": []map[string]interface{}{
			{"date": today, "totalCost": 10.0, "totalTokens": 1000},
		},
		"totals": map[string]interface{}{
			"totalCost":   10.0,
			"totalTokens": 1000,
		},
	})

	resp := wsResponse{Type: "res", ID: "crit-test", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessUsageResponse_WarningAlert(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 5.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")
	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt": float64(time.Now().UnixMilli()),
		"days":      1,
		"daily": []map[string]interface{}{
			{"date": today, "totalCost": 2.0, "totalTokens": 500},
		},
		"totals": map[string]interface{}{
			"totalCost":   2.0,
			"totalTokens": 500,
		},
	})

	resp := wsResponse{Type: "res", ID: "warn-test", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessUsageResponse_NoDailyData(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 5.0)
	defer uc.Stop()

	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt": float64(time.Now().UnixMilli()),
		"days":      0,
		"daily":     []map[string]interface{}{},
		"totals": map[string]interface{}{
			"totalCost":   0.0,
			"totalTokens": 0,
		},
	})

	resp := wsResponse{Type: "res", ID: "empty-test", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ========================= sessions.usage tests =========================

func TestProcessSessionsUsageResponse_Success(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt": float64(time.Now().UnixMilli()),
		"startDate": "2026-02-07",
		"endDate":   "2026-03-08",
		"sessions":  []interface{}{},
		"totals":    map[string]interface{}{"totalCost": 25.0, "totalTokens": 500000},
		"aggregates": map[string]interface{}{
			"byModel": []map[string]interface{}{
				{
					"provider": "anthropic",
					"model":    "claude-sonnet-4-20250514",
					"count":    150,
					"totals": map[string]interface{}{
						"input": 100000, "output": 50000, "cacheRead": 20000, "cacheWrite": 5000,
						"totalTokens": 175000, "totalCost": 15.0,
						"inputCost": 7.5, "outputCost": 5.0, "cacheReadCost": 1.5, "cacheWriteCost": 1.0,
						"missingCostEntries": 0,
					},
				},
				{
					"provider": "openai",
					"model":    "gpt-4o",
					"count":    80,
					"totals": map[string]interface{}{
						"input": 80000, "output": 40000, "totalTokens": 120000, "totalCost": 10.0,
						"inputCost": 6.0, "outputCost": 4.0,
						"missingCostEntries": 0,
					},
				},
			},
			"byProvider": []map[string]interface{}{
				{
					"provider": "anthropic",
					"count":    150,
					"totals":   map[string]interface{}{"totalCost": 15.0, "totalTokens": 175000},
				},
			},
			"daily": []map[string]interface{}{
				{"date": "2026-03-07", "tokens": 50000, "cost": 5.0, "messages": 20, "toolCalls": 10, "errors": 0},
				{"date": "2026-03-08", "tokens": 60000, "cost": 6.0, "messages": 25, "toolCalls": 15, "errors": 1},
			},
			"messages":   map[string]interface{}{"total": 200, "user": 80, "assistant": 80, "toolCalls": 30, "toolResults": 10, "errors": 0},
			"tools":      map[string]interface{}{"totalCalls": 30, "uniqueTools": 3, "tools": []map[string]interface{}{{"name": "read_file", "count": 15}, {"name": "write_file", "count": 10}, {"name": "search", "count": 5}}},
		},
	})

	resp := wsResponse{Type: "res", ID: "sess-1", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processSessionsUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uc.mu.RLock()
	defer uc.mu.RUnlock()

	if uc.sessionsData == nil {
		t.Fatal("expected sessionsData to be populated")
	}
	if len(uc.sessionsData.ByModel) != 2 {
		t.Errorf("expected 2 models, got %d", len(uc.sessionsData.ByModel))
	}
	if uc.sessionsData.ByModel[0].Provider != "anthropic" {
		t.Errorf("expected first model provider=anthropic, got %s", uc.sessionsData.ByModel[0].Provider)
	}
	if uc.sessionsData.ByModel[0].Count != 150 {
		t.Errorf("expected first model count=150, got %d", uc.sessionsData.ByModel[0].Count)
	}
	if uc.sessionsData.ByModel[0].Totals.TotalCost != 15.0 {
		t.Errorf("expected first model cost=15.0, got %f", uc.sessionsData.ByModel[0].Totals.TotalCost)
	}
	if uc.sessionsData.ByModel[1].Model != "gpt-4o" {
		t.Errorf("expected second model=gpt-4o, got %s", uc.sessionsData.ByModel[1].Model)
	}
	if len(uc.sessionsData.Daily) != 2 {
		t.Errorf("expected 2 daily entries, got %d", len(uc.sessionsData.Daily))
	}
	if uc.sessionsData.Daily[1].Messages != 25 {
		t.Errorf("expected daily[1] messages=25, got %d", uc.sessionsData.Daily[1].Messages)
	}
	if uc.sessionsData.Messages.Total != 200 {
		t.Errorf("expected messages.total=200, got %d", uc.sessionsData.Messages.Total)
	}
	if uc.sessionsData.Tools.TotalCalls != 30 {
		t.Errorf("expected tools.totalCalls=30, got %d", uc.sessionsData.Tools.TotalCalls)
	}
	if len(uc.sessionsData.Tools.Tools) != 3 {
		t.Errorf("expected 3 tool entries, got %d", len(uc.sessionsData.Tools.Tools))
	}
	if uc.sessionsData.StartDate != "2026-02-07" {
		t.Errorf("expected startDate=2026-02-07, got %s", uc.sessionsData.StartDate)
	}
}

func TestProcessSessionsUsageResponse_Error(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	resp := wsResponse{
		Type:  "res",
		ID:    "sess-err",
		OK:    false,
		Error: &wsError{Code: "forbidden", Message: "insufficient scope"},
	}

	err := uc.processSessionsUsageResponse(resp)
	if err == nil {
		t.Fatal("expected error for failed sessions.usage response")
	}
	if err.Error() != "sessions.usage error: insufficient scope" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestProcessSessionsUsageResponse_BadJSON(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	resp := wsResponse{
		Type:    "res",
		ID:      "sess-bad",
		OK:      true,
		Payload: json.RawMessage(`{not valid}`),
	}

	err := uc.processSessionsUsageResponse(resp)
	if err == nil {
		t.Fatal("expected error for bad JSON payload")
	}
}

func TestProcessSessionsUsageResponse_EmptyAggregates(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt":  float64(time.Now().UnixMilli()),
		"startDate":  "2026-03-08",
		"endDate":    "2026-03-08",
		"sessions":   []interface{}{},
		"totals":     map[string]interface{}{"totalCost": 0.0, "totalTokens": 0},
		"aggregates": map[string]interface{}{
			"messages": map[string]interface{}{"total": 0},
			"tools":    map[string]interface{}{"totalCalls": 0, "uniqueTools": 0},
			"daily":    []interface{}{},
		},
	})

	resp := wsResponse{Type: "res", ID: "sess-empty", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processSessionsUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uc.mu.RLock()
	defer uc.mu.RUnlock()

	if uc.sessionsData == nil {
		t.Fatal("expected sessionsData to be populated even when empty")
	}
	// Verify nil slices are initialized to empty (for clean JSON)
	if uc.sessionsData.ByModel == nil {
		t.Error("expected ByModel to be non-nil empty slice")
	}
	if uc.sessionsData.ByProvider == nil {
		t.Error("expected ByProvider to be non-nil empty slice")
	}
	if uc.sessionsData.Daily == nil {
		t.Error("expected Daily to be non-nil empty slice")
	}
	if uc.sessionsData.Tools.Tools == nil {
		t.Error("expected Tools.Tools to be non-nil empty slice")
	}
}

func TestProcessSessionsUsageResponse_NilDailyAndTools(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	// Aggregates with NO daily, NO tools.tools — test nil guard paths
	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt":  float64(time.Now().UnixMilli()),
		"startDate":  "2026-03-08",
		"endDate":    "2026-03-08",
		"sessions":   []interface{}{},
		"totals":     map[string]interface{}{"totalCost": 0.0},
		"aggregates": map[string]interface{}{
			"messages": map[string]interface{}{"total": 0},
			"tools":    map[string]interface{}{"totalCalls": 0, "uniqueTools": 0},
		},
	})

	resp := wsResponse{Type: "res", ID: "sess-nil", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processSessionsUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uc.mu.RLock()
	defer uc.mu.RUnlock()

	// daily was absent from JSON → parsed as nil → should be set to empty
	if uc.sessionsData.Daily == nil {
		t.Error("expected Daily to be non-nil after nil guard")
	}
	if len(uc.sessionsData.Daily) != 0 {
		t.Errorf("expected 0 daily entries, got %d", len(uc.sessionsData.Daily))
	}
}

func TestProcessSessionsUsageResponse_WithModelDaily(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	payload, _ := json.Marshal(map[string]interface{}{
		"updatedAt": float64(time.Now().UnixMilli()),
		"startDate": "2026-03-01",
		"endDate":   "2026-03-08",
		"sessions":  []interface{}{},
		"totals":    map[string]interface{}{"totalCost": 5.0},
		"aggregates": map[string]interface{}{
			"byModel":  []map[string]interface{}{{"provider": "anthropic", "model": "claude-sonnet-4-20250514", "count": 50, "totals": map[string]interface{}{"totalCost": 5.0, "totalTokens": 100000}}},
			"messages": map[string]interface{}{"total": 10},
			"tools":    map[string]interface{}{"totalCalls": 5, "uniqueTools": 1, "tools": []map[string]interface{}{{"name": "read_file", "count": 5}}},
			"daily":    []map[string]interface{}{{"date": "2026-03-08", "tokens": 10000, "cost": 1.0, "messages": 5, "toolCalls": 2, "errors": 0}},
			"modelDaily": []map[string]interface{}{
				{"date": "2026-03-08", "provider": "anthropic", "model": "claude-sonnet-4-20250514", "tokens": 10000, "cost": 1.0, "count": 5},
			},
		},
	})

	resp := wsResponse{Type: "res", ID: "sess-md", OK: true, Payload: json.RawMessage(payload)}
	if err := uc.processSessionsUsageResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uc.mu.RLock()
	defer uc.mu.RUnlock()

	if len(uc.sessionsData.ModelDaily) != 1 {
		t.Fatalf("expected 1 modelDaily entry, got %d", len(uc.sessionsData.ModelDaily))
	}
	if uc.sessionsData.ModelDaily[0].Provider != "anthropic" {
		t.Errorf("expected modelDaily provider=anthropic, got %s", uc.sessionsData.ModelDaily[0].Provider)
	}
	if uc.sessionsData.ModelDaily[0].Count != 5 {
		t.Errorf("expected modelDaily count=5, got %d", uc.sessionsData.ModelDaily[0].Count)
	}
}

func TestSnapshot_WithSessionsData(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")

	// Inject both cost and sessions data
	uc.mu.Lock()
	uc.data = &CostUsageSummary{
		UpdatedAt: time.Now(),
		Days:      7,
		Daily: []CostUsageDailyEntry{
			{Date: today, CostUsageTotals: CostUsageTotals{TotalCost: 0.50}},
		},
		Totals: CostUsageTotals{TotalCost: 3.50},
	}
	uc.sessionsData = &SessionsUsageData{
		UpdatedAt: time.Now(),
		StartDate: "2026-03-01",
		EndDate:   "2026-03-08",
		ByModel: []ModelUsageEntry{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Count: 100,
				Totals: CostUsageTotals{TotalCost: 2.50, TotalTokens: 100000}},
			{Provider: "openai", Model: "gpt-4o", Count: 50,
				Totals: CostUsageTotals{TotalCost: 1.00, TotalTokens: 50000}},
		},
		ByProvider: []ModelUsageEntry{
			{Provider: "anthropic", Count: 100,
				Totals: CostUsageTotals{TotalCost: 2.50}},
		},
		Daily:    []SessionDailyEntry{{Date: today, Tokens: 10000, Cost: 0.50, Messages: 5}},
		Messages: MessageCounts{Total: 50, User: 20, Assistant: 20, ToolCalls: 10},
		Tools:    ToolUsage{TotalCalls: 10, UniqueTools: 2, Tools: []ToolEntry{{Name: "read", Count: 7}}},
	}
	uc.mu.Unlock()

	snap := uc.Snapshot()
	if snap.Status != "ok" {
		t.Errorf("expected status=ok, got %s", snap.Status)
	}
	if snap.Models == nil {
		t.Fatal("expected Models to be populated")
	}
	if len(snap.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(snap.Models))
	}
	if snap.Models[0].Provider != "anthropic" {
		t.Errorf("expected first model provider=anthropic, got %s", snap.Models[0].Provider)
	}
	if snap.Sessions == nil {
		t.Fatal("expected Sessions to be populated")
	}
	if snap.Sessions.Messages.Total != 50 {
		t.Errorf("expected sessions.messages.total=50, got %d", snap.Sessions.Messages.Total)
	}
}

func TestSnapshot_WithoutSessionsData(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")

	// Only cost data, no sessions
	uc.mu.Lock()
	uc.data = &CostUsageSummary{
		UpdatedAt: time.Now(),
		Days:      7,
		Daily: []CostUsageDailyEntry{
			{Date: today, CostUsageTotals: CostUsageTotals{TotalCost: 0.50}},
		},
		Totals: CostUsageTotals{TotalCost: 3.50},
	}
	uc.mu.Unlock()

	snap := uc.Snapshot()
	if snap.Status != "ok" {
		t.Errorf("expected status=ok, got %s", snap.Status)
	}
	if snap.Models != nil {
		t.Errorf("expected Models to be nil when no sessions data, got %v", snap.Models)
	}
	if snap.Sessions != nil {
		t.Errorf("expected Sessions to be nil when no sessions data, got %v", snap.Sessions)
	}
}

func TestSessionsUsageDateRange(t *testing.T) {
	start, end := sessionsUsageDateRange()

	// Parse dates to verify format
	startTime, err := time.Parse("2006-01-02", start)
	if err != nil {
		t.Fatalf("invalid start date format: %v", err)
	}
	endTime, err := time.Parse("2006-01-02", end)
	if err != nil {
		t.Fatalf("invalid end date format: %v", err)
	}

	// Should be exactly 29 days apart (30-day window inclusive)
	diff := endTime.Sub(startTime).Hours() / 24
	if diff != 29 {
		t.Errorf("expected 29-day span, got %.0f days", diff)
	}

	// End should be today
	today := time.Now().UTC().Format("2006-01-02")
	if end != today {
		t.Errorf("expected end=%s, got %s", today, end)
	}
}

func TestProcessSessionsUsageResponse_ErrorNoDetail(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	resp := wsResponse{Type: "res", ID: "sess-nodetail", OK: false}
	err := uc.processSessionsUsageResponse(resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "sessions.usage error: unknown" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSnapshot_JSONSerialization(t *testing.T) {
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	today := time.Now().UTC().Format("2006-01-02")
	uc.mu.Lock()
	uc.data = &CostUsageSummary{
		UpdatedAt: time.Now(),
		Days:      1,
		Daily:     []CostUsageDailyEntry{{Date: today, CostUsageTotals: CostUsageTotals{TotalCost: 1.0}}},
		Totals:    CostUsageTotals{TotalCost: 1.0},
	}
	uc.sessionsData = &SessionsUsageData{
		UpdatedAt:  time.Now(),
		StartDate:  today,
		EndDate:    today,
		ByModel:    []ModelUsageEntry{{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Count: 10, Totals: CostUsageTotals{TotalCost: 1.0}}},
		ByProvider: []ModelUsageEntry{},
		Daily:      []SessionDailyEntry{},
		Messages:   MessageCounts{},
		Tools:      ToolUsage{Tools: []ToolEntry{}},
	}
	uc.mu.Unlock()

	snap := uc.Snapshot()

	// Verify it serializes to valid JSON
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}

	// Verify the JSON contains expected fields
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal snapshot JSON: %v", err)
	}

	if _, ok := parsed["models"]; !ok {
		t.Error("expected 'models' field in JSON output")
	}
	if _, ok := parsed["sessions"]; !ok {
		t.Error("expected 'sessions' field in JSON output")
	}

	// Verify models array has content
	models, ok := parsed["models"].([]interface{})
	if !ok || len(models) != 1 {
		t.Errorf("expected models array with 1 entry, got %v", parsed["models"])
	}
}

// =================== Mock WS Server Integration Tests ====================

// mockOpenClawHandler simulates the OpenClaw WS v3 protocol for testing.
// It sends connect.challenge, accepts connect, and responds to usage.cost
// and sessions.usage requests.
func mockOpenClawHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("mock: accept error: %v", err)
			return
		}
		defer conn.CloseNow() //nolint:errcheck

		ctx := r.Context()

		// Step 1: Send connect.challenge
		challenge, _ := json.Marshal(map[string]interface{}{
			"type":  "event",
			"event": "connect.challenge",
			"payload": map[string]interface{}{
				"nonce": "test-nonce-123",
			},
		})
		if err := conn.Write(ctx, websocket.MessageText, challenge); err != nil {
			return
		}

		// Step 2: Read connect request
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var connectReq wsRequest
		if err := json.Unmarshal(data, &connectReq); err != nil {
			return
		}

		// Send connect response (hello-ok)
		helloResp, _ := json.Marshal(map[string]interface{}{
			"type": "res",
			"id":   connectReq.ID,
			"ok":   true,
			"payload": map[string]interface{}{
				"protocol": 3,
			},
		})
		if err := conn.Write(ctx, websocket.MessageText, helloResp); err != nil {
			return
		}

		// Step 3: Handle requests
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var req wsRequest
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}

			var respPayload interface{}
			switch req.Method {
			case "usage.cost":
				respPayload = map[string]interface{}{
					"updatedAt": float64(time.Now().UnixMilli()),
					"days":      7,
					"daily": []map[string]interface{}{
						{"date": time.Now().UTC().Format("2006-01-02"), "totalCost": 1.50, "totalTokens": 10000, "input": 6000, "output": 4000},
					},
					"totals": map[string]interface{}{
						"totalCost": 8.50, "totalTokens": 80000, "input": 50000, "output": 30000,
					},
				}
			case "sessions.usage":
				respPayload = map[string]interface{}{
					"updatedAt": float64(time.Now().UnixMilli()),
					"startDate": "2026-02-07",
					"endDate":   "2026-03-08",
					"sessions":  []interface{}{},
					"totals":    map[string]interface{}{"totalCost": 8.50, "totalTokens": 80000},
					"aggregates": map[string]interface{}{
						"byModel": []map[string]interface{}{
							{"provider": "anthropic", "model": "claude-sonnet-4-20250514", "count": 50,
								"totals": map[string]interface{}{"totalCost": 6.0, "totalTokens": 60000}},
							{"provider": "openai", "model": "gpt-4o", "count": 20,
								"totals": map[string]interface{}{"totalCost": 2.5, "totalTokens": 20000}},
						},
						"byProvider": []map[string]interface{}{
							{"provider": "anthropic", "count": 50,
								"totals": map[string]interface{}{"totalCost": 6.0}},
						},
						"daily":    []map[string]interface{}{{"date": "2026-03-08", "tokens": 10000, "cost": 1.5, "messages": 5, "toolCalls": 3, "errors": 0}},
						"messages": map[string]interface{}{"total": 50, "user": 20, "assistant": 20, "toolCalls": 10},
						"tools":    map[string]interface{}{"totalCalls": 10, "uniqueTools": 2, "tools": []map[string]interface{}{{"name": "read_file", "count": 7}}},
					},
				}
			default:
				continue
			}

			resp, _ := json.Marshal(map[string]interface{}{
				"type":    "res",
				"id":      req.ID,
				"ok":      true,
				"payload": respPayload,
			})
			if err := conn.Write(ctx, websocket.MessageText, resp); err != nil {
				return
			}
		}
	}
}

func TestUsageCollector_MockWSIntegration(t *testing.T) {
	// Start mock OpenClaw WS server
	server := httptest.NewServer(mockOpenClawHandler(t))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	uc := NewUsageCollector(wsURL, "test-token", 5.0, 20.0)
	defer uc.Stop()

	// Wait for initial fetch to complete (up to 5 seconds)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		uc.mu.RLock()
		hasData := uc.data != nil
		hasSessions := uc.sessionsData != nil
		uc.mu.RUnlock()
		if hasData && hasSessions {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify cost data
	snap := uc.Snapshot()
	if snap.Status != "ok" {
		t.Fatalf("expected status=ok, got %s (collector=%s)", snap.Status, snap.Collector)
	}
	if snap.PeriodCost != 8.50 {
		t.Errorf("expected periodCost=8.50, got %f", snap.PeriodCost)
	}
	if snap.Days != 7 {
		t.Errorf("expected days=7, got %d", snap.Days)
	}

	// Verify sessions data
	if snap.Sessions == nil {
		t.Fatal("expected sessions data to be populated")
	}
	if len(snap.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(snap.Models))
	}
	if snap.Models[0].Provider != "anthropic" {
		t.Errorf("expected first model provider=anthropic, got %s", snap.Models[0].Provider)
	}
	if snap.Sessions.Messages.Total != 50 {
		t.Errorf("expected messages.total=50, got %d", snap.Sessions.Messages.Total)
	}
}

func TestUsageCollector_MockWSConnectRejected(t *testing.T) {
	// Server that rejects the connect request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow() //nolint:errcheck

		ctx := r.Context()

		// Send challenge
		challenge, _ := json.Marshal(map[string]interface{}{
			"type":  "event",
			"event": "connect.challenge",
		})
		conn.Write(ctx, websocket.MessageText, challenge) //nolint:errcheck

		// Read connect request
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var req wsRequest
		json.Unmarshal(data, &req) //nolint:errcheck

		// Reject
		resp, _ := json.Marshal(map[string]interface{}{
			"type": "res",
			"id":   req.ID,
			"ok":   false,
			"error": map[string]interface{}{
				"code":    "unauthorized",
				"message": "bad token",
			},
		})
		conn.Write(ctx, websocket.MessageText, resp) //nolint:errcheck
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	uc := NewUsageCollector(wsURL, "bad-token", 5.0, 20.0)

	// Give it a brief moment to connect and fail
	time.Sleep(300 * time.Millisecond)
	uc.Stop()

	snap := uc.Snapshot()
	if snap.Status != "unavailable" {
		t.Logf("status=%s (expected unavailable — collector hasn't received data)", snap.Status)
	}
	if snap.Collector != string(StatusError) && snap.Collector != string(StatusConnecting) {
		t.Logf("collector=%s (expected error or connecting after rejection)", snap.Collector)
	}
}

func TestUsageCollector_ReadFrame(t *testing.T) {
	// Test readFrame by starting a simple WS server that sends a single frame
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow() //nolint:errcheck

		frame, _ := json.Marshal(map[string]interface{}{
			"type":  "event",
			"event": "test.event",
		})
		conn.Write(r.Context(), websocket.MessageText, frame) //nolint:errcheck
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	uc := NewUsageCollector("", "", 1.0, 10.0) // disabled collector, just borrowing methods
	defer uc.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	frame, err := uc.readFrame(conn)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if frame.Event != "test.event" {
		t.Errorf("expected event=test.event, got %s", frame.Event)
	}
}

func TestUsageCollector_WriteFrame(t *testing.T) {
	// Test writeFrame by sending a frame and verifying server receives it
	received := make(chan wsRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow() //nolint:errcheck

		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var req wsRequest
		json.Unmarshal(data, &req) //nolint:errcheck
		received <- req
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	uc := NewUsageCollector("", "", 1.0, 10.0)
	defer uc.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	req := wsRequest{Type: "req", ID: "test-write", Method: "test.method"}
	if err := uc.writeFrame(conn, req); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	select {
	case got := <-received:
		if got.Method != "test.method" {
			t.Errorf("expected method=test.method, got %s", got.Method)
		}
		if got.ID != "test-write" {
			t.Errorf("expected id=test-write, got %s", got.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for frame")
	}
}
