package costhistory

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Poller periodically fetches cost data from the gateway and persists
// it to Postgres via a Persister.
type Poller struct {
	store      Persister
	gatewayURL string
	envReader  EnvReader
	interval   time.Duration
	client     *http.Client

	mu      sync.RWMutex
	lastOK  time.Time
	lastErr error

	ctx    context.Context
	cancel context.CancelFunc
}

// Persister abstracts the database write operations for cost history.
type Persister interface {
	UpsertDailySnapshot(ctx context.Context, snap DailySnapshot) error
	UpsertModelSnapshot(ctx context.Context, snap ModelSnapshot) error
	Close() error
}

// Querier abstracts database read operations for cost history.
type Querier interface {
	ListDailySnapshots(ctx context.Context, days int) ([]DailyRow, error)
	ListModelSnapshots(ctx context.Context, days int) ([]ModelRow, error)
	GetTrend(ctx context.Context, days int) (*TrendResult, error)
}

// EnvReader provides AUTH_SECRET from the .env file.
// This allows the poller to mint gateway admin tokens without
// direct access to the secret.
type EnvReader func() (map[string]string, error)

// PollerConfig holds the configuration for the cost poller.
type PollerConfig struct {
	Store      Persister
	GatewayURL string    // e.g., "http://safepaw-gateway:8080"
	EnvReader  EnvReader // reads AUTH_SECRET from .env
	Interval   time.Duration
}

// NewPoller creates and starts a background cost poller.
func NewPoller(cfg PollerConfig) *Poller {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Poller{
		store:      cfg.Store,
		gatewayURL: cfg.GatewayURL,
		envReader:  cfg.EnvReader,
		interval:   cfg.Interval,
		client:     &http.Client{Timeout: 10 * time.Second},
		ctx:        ctx,
		cancel:     cancel,
	}

	go p.run()
	log.Printf("[COST-POLLER] Started (interval=%v, gateway=%s)", cfg.Interval, cfg.GatewayURL)
	return p
}

// Stop shuts down the poller.
func (p *Poller) Stop() {
	p.cancel()
}

// Status returns the last successful poll time and any error.
func (p *Poller) Status() (lastOK time.Time, lastErr error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastOK, p.lastErr
}

func (p *Poller) run() {
	// Initial delay — wait for gateway to be ready
	select {
	case <-time.After(30 * time.Second):
	case <-p.ctx.Done():
		return
	}

	// First poll
	p.poll()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *Poller) poll() {
	ctx, cancel := context.WithTimeout(p.ctx, 15*time.Second)
	defer cancel()

	data, err := p.fetchUsage(ctx)
	if err != nil {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		log.Printf("[COST-POLLER] Fetch failed: %v", err)
		return
	}

	if err := p.persist(ctx, data); err != nil {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		log.Printf("[COST-POLLER] Persist failed: %v", err)
		return
	}

	p.mu.Lock()
	p.lastOK = time.Now()
	p.lastErr = nil
	p.mu.Unlock()
}

// gatewayUsageResponse mirrors the gateway's /admin/usage JSON.
type gatewayUsageResponse struct {
	Status string `json:"status"`
	Daily  []struct {
		Date        string  `json:"date"`
		TotalCost   float64 `json:"totalCost"`
		TotalTokens int64   `json:"totalTokens"`
		Input       int64   `json:"input"`
		Output      int64   `json:"output"`
	} `json:"daily"`
	Models []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Count    int    `json:"count"`
		Totals   struct {
			TotalCost   float64 `json:"totalCost"`
			TotalTokens int64   `json:"totalTokens"`
			Input       int64   `json:"input"`
			Output      int64   `json:"output"`
		} `json:"totals"`
	} `json:"models"`
	Sessions *struct {
		Daily []struct {
			Date      string  `json:"date"`
			Tokens    int64   `json:"tokens"`
			Cost      float64 `json:"cost"`
			Messages  int     `json:"messages"`
			ToolCalls int     `json:"toolCalls"`
		} `json:"daily"`
	} `json:"sessions"`
}

func (p *Poller) fetchUsage(ctx context.Context) (*gatewayUsageResponse, error) {
	// Read AUTH_SECRET for token minting
	env, err := p.envReader()
	if err != nil {
		return nil, fmt.Errorf("read env: %w", err)
	}
	secret := env["AUTH_SECRET"]
	if secret == "" || len(secret) < 32 {
		return nil, fmt.Errorf("AUTH_SECRET not available or too short")
	}

	// Mint short-lived admin token (same pattern as handleGatewayUsage)
	now := time.Now().Unix()
	payload, _ := json.Marshal(map[string]interface{}{
		"sub": "wizard-cost-poller", "iat": now, "exp": now + 30, "scope": "admin",
	})
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	token := payloadB64 + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, "GET", p.gatewayURL+"/admin/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var data gatewayUsageResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if data.Status != "ok" {
		return nil, fmt.Errorf("gateway status: %s", data.Status)
	}

	return &data, nil
}

func (p *Poller) persist(ctx context.Context, data *gatewayUsageResponse) error {
	persisted := 0

	// Persist daily snapshots — use sessions data if available (has message/tool counts)
	if data.Sessions != nil && len(data.Sessions.Daily) > 0 {
		for _, d := range data.Sessions.Daily {
			snap := DailySnapshot{
				Date:         d.Date,
				TotalTokens:  d.Tokens,
				TotalCostUSD: d.Cost,
				Messages:     d.Messages,
				ToolCalls:    d.ToolCalls,
			}
			if err := p.store.UpsertDailySnapshot(ctx, snap); err != nil {
				return fmt.Errorf("upsert daily %s: %w", d.Date, err)
			}
			persisted++
		}
	} else if len(data.Daily) > 0 {
		// Fallback to usage.cost daily data (no message/tool counts)
		for _, d := range data.Daily {
			snap := DailySnapshot{
				Date:           d.Date,
				PromptTokens:   d.Input,
				CompletionTkns: d.Output,
				TotalTokens:    d.TotalTokens,
				TotalCostUSD:   d.TotalCost,
			}
			if err := p.store.UpsertDailySnapshot(ctx, snap); err != nil {
				return fmt.Errorf("upsert daily %s: %w", d.Date, err)
			}
			persisted++
		}
	}

	// Persist model snapshots
	for _, m := range data.Models {
		if m.Provider == "" && m.Model == "" {
			continue // skip entries without identification
		}
		// Determine date: use today since models are aggregated
		today := time.Now().UTC().Format("2006-01-02")
		snap := ModelSnapshot{
			Date:         today,
			Provider:     m.Provider,
			Model:        m.Model,
			RequestCount: m.Count,
			PromptTokens: m.Totals.Input,
			CompletionTk: m.Totals.Output,
			TotalTokens:  m.Totals.TotalTokens,
			TotalCostUSD: m.Totals.TotalCost,
		}
		if err := p.store.UpsertModelSnapshot(ctx, snap); err != nil {
			return fmt.Errorf("upsert model %s/%s: %w", m.Provider, m.Model, err)
		}
		persisted++
	}

	log.Printf("[COST-POLLER] Persisted %d snapshots", persisted)
	return nil
}
