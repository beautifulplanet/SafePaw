package main

import (
	"encoding/json"
	"testing"
	"time"
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
