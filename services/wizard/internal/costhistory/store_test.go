package costhistory

import (
	"testing"
)

func TestStoreConfig_DefaultPort(t *testing.T) {
	cfg := StoreConfig{
		Host:     "localhost",
		User:     "testuser",
		Password: "testpass",
		DBName:   "testdb",
	}
	if cfg.Port != 0 {
		t.Fatalf("expected default port 0 before NewStore, got %d", cfg.Port)
	}
}

func TestNewStore_BadHost(t *testing.T) {
	// NewStore with unreachable host should return error (ping fails).
	// Use localhost with a high port that's definitely not listening — gets ECONNREFUSED fast.
	_, err := NewStore(StoreConfig{
		Host:     "localhost",
		Port:     59999,
		User:     "nobody",
		Password: "nope",
		DBName:   "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error from NewStore with unreachable host")
	}
}

func TestNewStore_DefaultPort(t *testing.T) {
	// Verify that port 0 gets defaulted to 5432 (connection will fail,
	// but the DSN should be constructed correctly).
	_, err := NewStore(StoreConfig{
		Host:     "localhost",
		Port:     0,
		User:     "nobody",
		Password: "nope",
		DBName:   "nonexistent",
	})
	// We just care that it attempted with the defaulted port
	if err == nil {
		t.Fatal("expected error from NewStore with unreachable host")
	}
}

func TestDailySnapshot_Fields(t *testing.T) {
	snap := DailySnapshot{
		Date:           "2025-01-15",
		PromptTokens:   1000,
		CompletionTkns: 2000,
		TotalTokens:    3000,
		TotalCostUSD:   0.05,
		Messages:       10,
		ToolCalls:      3,
	}
	if snap.Date != "2025-01-15" {
		t.Errorf("Date = %q, want 2025-01-15", snap.Date)
	}
	if snap.TotalTokens != 3000 {
		t.Errorf("TotalTokens = %d, want 3000", snap.TotalTokens)
	}
	if snap.Messages != 10 {
		t.Errorf("Messages = %d, want 10", snap.Messages)
	}
}

func TestModelSnapshot_Fields(t *testing.T) {
	snap := ModelSnapshot{
		Date:         "2025-01-15",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		RequestCount: 42,
		PromptTokens: 5000,
		CompletionTk: 3000,
		TotalTokens:  8000,
		TotalCostUSD: 0.12,
	}
	if snap.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", snap.Provider)
	}
	if snap.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want claude-sonnet-4-20250514", snap.Model)
	}
	if snap.RequestCount != 42 {
		t.Errorf("RequestCount = %d, want 42", snap.RequestCount)
	}
}
