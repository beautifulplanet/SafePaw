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
		_ = db.Close() // #nosec G104 -- best-effort cleanup on failed ping
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

// DailyRow is a daily snapshot returned from a history query.
type DailyRow struct {
	Date         string  `json:"date"`
	TotalTokens  int64   `json:"totalTokens"`
	TotalCostUSD float64 `json:"totalCost"`
	PromptTokens int64   `json:"promptTokens"`
	CompletionTk int64   `json:"completionTokens"`
	Messages     int     `json:"messages"`
	ToolCalls    int     `json:"toolCalls"`
}

// ModelRow is a per-model snapshot returned from a history query.
type ModelRow struct {
	Date         string  `json:"date"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	RequestCount int     `json:"requestCount"`
	TotalTokens  int64   `json:"totalTokens"`
	TotalCostUSD float64 `json:"totalCost"`
	PromptTokens int64   `json:"promptTokens"`
	CompletionTk int64   `json:"completionTokens"`
}

// ListDailySnapshots returns daily snapshots for the last N days, ordered by date descending.
func (s *Store) ListDailySnapshots(ctx context.Context, days int) ([]DailyRow, error) {
	if days <= 0 {
		days = 30
	}
	const query = `
		SELECT date::text, COALESCE(total_tokens,0), COALESCE(total_cost_usd,0),
		       COALESCE(input_tokens,0), COALESCE(output_tokens,0),
		       COALESCE(messages_total,0), COALESCE(tool_calls,0)
		FROM gateway.cost_daily_snapshots
		WHERE date >= CURRENT_DATE - $1
		ORDER BY date DESC`

	rows, err := s.db.QueryContext(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("query daily snapshots: %w", err)
	}
	defer rows.Close()

	var out []DailyRow
	for rows.Next() {
		var r DailyRow
		if err := rows.Scan(&r.Date, &r.TotalTokens, &r.TotalCostUSD,
			&r.PromptTokens, &r.CompletionTk, &r.Messages, &r.ToolCalls); err != nil {
			return nil, fmt.Errorf("scan daily row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListModelSnapshots returns per-model snapshots for the last N days, ordered by cost descending.
func (s *Store) ListModelSnapshots(ctx context.Context, days int) ([]ModelRow, error) {
	if days <= 0 {
		days = 30
	}
	const query = `
		SELECT date::text, provider, model, COALESCE(request_count,0),
		       COALESCE(total_tokens,0), COALESCE(total_cost_usd,0),
		       COALESCE(input_tokens,0), COALESCE(output_tokens,0)
		FROM gateway.cost_model_snapshots
		WHERE date >= CURRENT_DATE - $1
		ORDER BY total_cost_usd DESC, date DESC`

	rows, err := s.db.QueryContext(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("query model snapshots: %w", err)
	}
	defer rows.Close()

	var out []ModelRow
	for rows.Next() {
		var r ModelRow
		if err := rows.Scan(&r.Date, &r.Provider, &r.Model, &r.RequestCount,
			&r.TotalTokens, &r.TotalCostUSD, &r.PromptTokens, &r.CompletionTk); err != nil {
			return nil, fmt.Errorf("scan model row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TrendResult holds trend analysis comparing recent vs prior period.
type TrendResult struct {
	RecentDays     int     `json:"recentDays"`
	RecentCost     float64 `json:"recentCost"`
	RecentTokens   int64   `json:"recentTokens"`
	PriorCost      float64 `json:"priorCost"`
	PriorTokens    int64   `json:"priorTokens"`
	CostChangeP    float64 `json:"costChangePct"`  // % change
	TokenChangeP   float64 `json:"tokenChangePct"` // % change
	DailyAvgRecent float64 `json:"dailyAvgRecent"`
	DailyAvgPrior  float64 `json:"dailyAvgPrior"`
	AnomalyScore   float64 `json:"anomalyScore"` // 0-1, >0.7 = alert
}

// GetTrend compares the last `days` vs the prior `days` period.
func (s *Store) GetTrend(ctx context.Context, days int) (*TrendResult, error) {
	if days <= 0 {
		days = 7
	}
	const query = `
		WITH recent AS (
			SELECT COALESCE(SUM(total_cost_usd),0) AS cost,
			       COALESCE(SUM(total_tokens),0) AS tokens,
			       COUNT(*) AS n
			FROM gateway.cost_daily_snapshots
			WHERE date >= CURRENT_DATE - $1 AND date < CURRENT_DATE
		),
		prior AS (
			SELECT COALESCE(SUM(total_cost_usd),0) AS cost,
			       COALESCE(SUM(total_tokens),0) AS tokens,
			       COUNT(*) AS n
			FROM gateway.cost_daily_snapshots
			WHERE date >= CURRENT_DATE - ($1 * 2) AND date < CURRENT_DATE - $1
		)
		SELECT recent.cost, recent.tokens, recent.n,
		       prior.cost, prior.tokens, prior.n
		FROM recent, prior`

	var rc, pc float64
	var rt, pt int64
	var rn, pn int
	if err := s.db.QueryRowContext(ctx, query, days).Scan(&rc, &rt, &rn, &pc, &pt, &pn); err != nil {
		return nil, fmt.Errorf("query trend: %w", err)
	}

	tr := &TrendResult{
		RecentDays:   days,
		RecentCost:   rc,
		RecentTokens: rt,
		PriorCost:    pc,
		PriorTokens:  pt,
	}

	// Cost change %
	if pc > 0 {
		tr.CostChangeP = ((rc - pc) / pc) * 100
	}
	// Token change %
	if pt > 0 {
		tr.TokenChangeP = ((float64(rt) - float64(pt)) / float64(pt)) * 100
	}

	// Daily averages
	if rn > 0 {
		tr.DailyAvgRecent = rc / float64(rn)
	}
	if pn > 0 {
		tr.DailyAvgPrior = pc / float64(pn)
	}

	// Anomaly score: 0-1 based on how much recent deviates from prior
	// Simple ratio-based: if recent is 2x+ prior, score approaches 1.0
	if tr.DailyAvgPrior > 0 {
		ratio := tr.DailyAvgRecent / tr.DailyAvgPrior
		if ratio > 1 {
			tr.AnomalyScore = 1 - (1 / ratio) // 2x→0.5, 3x→0.67, 5x→0.8, 10x→0.9
		}
	} else if tr.DailyAvgRecent > 0 {
		tr.AnomalyScore = 1.0 // spending appeared from nothing — max anomaly
	}

	return tr, nil
}
