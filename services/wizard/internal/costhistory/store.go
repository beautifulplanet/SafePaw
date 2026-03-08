// Package costhistory persists daily cost snapshots from the gateway
// into Postgres for historical analysis and trend detection.
package costhistory

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // Postgres driver
)

// Store handles Postgres operations for cost history data.
type Store struct {
	db *sql.DB
}

// StoreConfig holds Postgres connection parameters.
type StoreConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

// NewStore creates a Store and verifies the connection.
func NewStore(cfg StoreConfig) (*Store, error) {
	if cfg.Port == 0 {
		cfg.Port = 5432
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.Printf("[COST-STORE] Connected to Postgres at %s:%d/%s", cfg.Host, cfg.Port, cfg.DBName)
	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DailySnapshot represents one day's cost/token data for upsert.
type DailySnapshot struct {
	Date           string
	PromptTokens   int64
	CompletionTkns int64
	TotalTokens    int64
	TotalCostUSD   float64
	Messages       int
	ToolCalls      int
}

// ModelSnapshot represents one model's cost data for a given day.
type ModelSnapshot struct {
	Date         string
	Provider     string
	Model        string
	RequestCount int
	PromptTokens int64
	CompletionTk int64
	TotalTokens  int64
	TotalCostUSD float64
}

// UpsertDailySnapshot writes or updates a daily cost snapshot.
func (s *Store) UpsertDailySnapshot(ctx context.Context, snap DailySnapshot) error {
	const query = `
		INSERT INTO gateway.cost_daily_snapshots
			(date, prompt_tokens, completion_tokens, total_tokens, total_cost_usd, messages, tool_calls)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (date) DO UPDATE SET
			prompt_tokens     = EXCLUDED.prompt_tokens,
			completion_tokens = EXCLUDED.completion_tokens,
			total_tokens      = EXCLUDED.total_tokens,
			total_cost_usd    = EXCLUDED.total_cost_usd,
			messages          = EXCLUDED.messages,
			tool_calls        = EXCLUDED.tool_calls,
			snapshot_at       = NOW()`

	_, err := s.db.ExecContext(ctx, query,
		snap.Date, snap.PromptTokens, snap.CompletionTkns, snap.TotalTokens,
		snap.TotalCostUSD, snap.Messages, snap.ToolCalls,
	)
	return err
}

// UpsertModelSnapshot writes or updates a per-model cost snapshot.
func (s *Store) UpsertModelSnapshot(ctx context.Context, snap ModelSnapshot) error {
	const query = `
		INSERT INTO gateway.cost_model_snapshots
			(date, provider, model, request_count, prompt_tokens, completion_tokens, total_tokens, total_cost_usd)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (date, provider, model) DO UPDATE SET
			request_count     = EXCLUDED.request_count,
			prompt_tokens     = EXCLUDED.prompt_tokens,
			completion_tokens = EXCLUDED.completion_tokens,
			total_tokens      = EXCLUDED.total_tokens,
			total_cost_usd    = EXCLUDED.total_cost_usd,
			snapshot_at       = NOW()`

	_, err := s.db.ExecContext(ctx, query,
		snap.Date, snap.Provider, snap.Model, snap.RequestCount,
		snap.PromptTokens, snap.CompletionTk, snap.TotalTokens, snap.TotalCostUSD,
	)
	return err
}

// Cleanup removes snapshots older than retentionDays.
func (s *Store) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	var deleted int64
	err := s.db.QueryRowContext(ctx,
		"SELECT gateway.cleanup_old_cost_snapshots($1)", retentionDays,
	).Scan(&deleted)
	return deleted, err
}
