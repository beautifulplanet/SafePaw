// =============================================================
// SafePaw Gateway — OpenClaw Usage Collector
// =============================================================
// WebSocket client that connects to OpenClaw's control plane,
// authenticates with a shared gateway token, and periodically
// polls usage.cost and sessions.usage to build cost summaries
// with per-model breakdowns for the dashboard.
//
// Protocol: OpenClaw WS v3 (JSON frames)
//   1. Open WS to openclaw:18789
//   2. Receive connect.challenge event (nonce)
//   3. Send connect request with token auth
//   4. Send usage.cost + sessions.usage every pollInterval
//   5. Handle tick keepalives; reconnect on failure
// =============================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// UsageCollector connects to OpenClaw's WS API and polls cost data.
type UsageCollector struct {
	wsURL    string
	token    string
	warnUSD  float64
	critUSD  float64
	interval time.Duration

	mu           sync.RWMutex
	data         *CostUsageSummary
	sessionsData *SessionsUsageData
	status       CollectorStatus
	lastOK       time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

// CollectorStatus describes the current state of the collector.
type CollectorStatus string

const (
	StatusDisabled    CollectorStatus = "disabled"
	StatusConnecting  CollectorStatus = "connecting"
	StatusConnected   CollectorStatus = "connected"
	StatusError       CollectorStatus = "error"
	StatusUnavailable CollectorStatus = "unavailable"
)

// CostUsageSummary mirrors OpenClaw's usage.cost response.
type CostUsageSummary struct {
	UpdatedAt time.Time             `json:"updatedAt"`
	Days      int                   `json:"days"`
	Daily     []CostUsageDailyEntry `json:"daily"`
	Totals    CostUsageTotals       `json:"totals"`
}

// CostUsageDailyEntry is one day's token/cost breakdown.
type CostUsageDailyEntry struct {
	Date string `json:"date"` // "YYYY-MM-DD"
	CostUsageTotals
}

// CostUsageTotals holds aggregated cost/token data.
type CostUsageTotals struct {
	Input              int64   `json:"input"`
	Output             int64   `json:"output"`
	CacheRead          int64   `json:"cacheRead"`
	CacheWrite         int64   `json:"cacheWrite"`
	TotalTokens        int64   `json:"totalTokens"`
	TotalCost          float64 `json:"totalCost"`
	InputCost          float64 `json:"inputCost"`
	OutputCost         float64 `json:"outputCost"`
	CacheReadCost      float64 `json:"cacheReadCost"`
	CacheWriteCost     float64 `json:"cacheWriteCost"`
	MissingCostEntries int     `json:"missingCostEntries"`
}

// UsageResponse is the JSON returned by /admin/usage.
type UsageResponse struct {
	Status     string                `json:"status"`
	Collector  string                `json:"collector"`
	UpdatedAt  string                `json:"updatedAt,omitempty"`
	Alert      string                `json:"alert,omitempty"`
	WarnUSD    float64               `json:"warnThresholdUsd"`
	CritUSD    float64               `json:"critThresholdUsd"`
	TodayCost  float64               `json:"todayCostUsd"`
	PeriodCost float64               `json:"periodCostUsd"`
	Days       int                   `json:"days"`
	Daily      []CostUsageDailyEntry `json:"daily,omitempty"`
	Totals     *CostUsageTotals      `json:"totals,omitempty"`
	Models     []ModelUsageEntry     `json:"models,omitempty"`
	Sessions   *SessionsUsageData    `json:"sessions,omitempty"`
}

// SessionsUsageData holds parsed sessions.usage aggregates.
type SessionsUsageData struct {
	UpdatedAt  time.Time           `json:"updatedAt"`
	StartDate  string              `json:"startDate"`
	EndDate    string              `json:"endDate"`
	ByModel    []ModelUsageEntry   `json:"byModel"`
	ByProvider []ModelUsageEntry   `json:"byProvider"`
	Daily      []SessionDailyEntry `json:"daily"`
	ModelDaily []ModelDailyEntry   `json:"modelDaily,omitempty"`
	Messages   MessageCounts       `json:"messages"`
	Tools      ToolUsage           `json:"tools"`
}

// ModelUsageEntry represents per-model or per-provider cost breakdown.
type ModelUsageEntry struct {
	Provider string          `json:"provider,omitempty"`
	Model    string          `json:"model,omitempty"`
	Count    int             `json:"count"`
	Totals   CostUsageTotals `json:"totals"`
}

// SessionDailyEntry is a daily aggregate from sessions.usage.
type SessionDailyEntry struct {
	Date      string  `json:"date"`
	Tokens    int64   `json:"tokens"`
	Cost      float64 `json:"cost"`
	Messages  int     `json:"messages"`
	ToolCalls int     `json:"toolCalls"`
	Errors    int     `json:"errors"`
}

// ModelDailyEntry is a per-model-per-day breakdown from sessions.usage.
type ModelDailyEntry struct {
	Date     string  `json:"date"`
	Provider string  `json:"provider,omitempty"`
	Model    string  `json:"model,omitempty"`
	Tokens   int64   `json:"tokens"`
	Cost     float64 `json:"cost"`
	Count    int     `json:"count"`
}

// MessageCounts holds message count breakdown.
type MessageCounts struct {
	Total       int `json:"total"`
	User        int `json:"user"`
	Assistant   int `json:"assistant"`
	ToolCalls   int `json:"toolCalls"`
	ToolResults int `json:"toolResults"`
	Errors      int `json:"errors"`
}

// ToolUsage holds tool usage data.
type ToolUsage struct {
	TotalCalls  int         `json:"totalCalls"`
	UniqueTools int         `json:"uniqueTools"`
	Tools       []ToolEntry `json:"tools"`
}

// ToolEntry is an individual tool's usage count.
type ToolEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// WS protocol frame types
type wsRequest struct {
	Type   string      `json:"type"`
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

type wsResponse struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *wsError        `json:"error,omitempty"`
	Event   string          `json:"event,omitempty"`
}

type wsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type connectParams struct {
	MinProtocol int           `json:"minProtocol"`
	MaxProtocol int           `json:"maxProtocol"`
	Client      connectClient `json:"client"`
	Role        string        `json:"role"`
	Scopes      []string      `json:"scopes"`
	Auth        connectAuth   `json:"auth"`
}

type connectClient struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

type connectAuth struct {
	Token string `json:"token"`
}

// NewUsageCollector creates a new collector. If wsURL or token is empty, it stays disabled.
func NewUsageCollector(wsURL, token string, warnUSD, critUSD float64) *UsageCollector {
	ctx, cancel := context.WithCancel(context.Background())
	uc := &UsageCollector{
		wsURL:    wsURL,
		token:    token,
		warnUSD:  warnUSD,
		critUSD:  critUSD,
		interval: 60 * time.Second,
		status:   StatusDisabled,
		ctx:      ctx,
		cancel:   cancel,
	}

	if wsURL == "" || token == "" {
		log.Println("[COST] Usage collector disabled (OPENCLAW_WS_URL or OPENCLAW_GATEWAY_TOKEN not set)")
		return uc
	}

	uc.status = StatusConnecting
	go uc.run()
	return uc
}

// Snapshot returns the current cost data and alert level.
func (uc *UsageCollector) Snapshot() UsageResponse {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	resp := UsageResponse{
		Collector: string(uc.status),
		WarnUSD:   uc.warnUSD,
		CritUSD:   uc.critUSD,
	}

	if uc.data == nil {
		resp.Status = "unavailable"
		return resp
	}

	resp.Status = "ok"
	resp.UpdatedAt = uc.data.UpdatedAt.Format(time.RFC3339)
	resp.Days = uc.data.Days
	resp.Daily = uc.data.Daily
	resp.Totals = &uc.data.Totals
	resp.PeriodCost = uc.data.Totals.TotalCost

	// Today's cost = last daily entry if it matches today
	today := time.Now().UTC().Format("2006-01-02")
	if len(uc.data.Daily) > 0 {
		last := uc.data.Daily[len(uc.data.Daily)-1]
		if last.Date == today {
			resp.TodayCost = last.TotalCost
		}
	}

	// Alert level
	switch {
	case resp.TodayCost >= uc.critUSD:
		resp.Alert = "critical"
	case resp.TodayCost >= uc.warnUSD:
		resp.Alert = "warning"
	default:
		resp.Alert = "ok"
	}

	// Attach sessions data (model breakdown etc.)
	if uc.sessionsData != nil {
		resp.Models = uc.sessionsData.ByModel
		resp.Sessions = uc.sessionsData
	}

	return resp
}

// Stop shuts down the collector.
func (uc *UsageCollector) Stop() {
	uc.cancel()
}

// run is the main loop: connect → poll → reconnect on failure.
func (uc *UsageCollector) run() {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-uc.ctx.Done():
			return
		default:
		}

		err := uc.connectAndPoll()
		if err != nil {
			uc.mu.Lock()
			uc.status = StatusError
			uc.mu.Unlock()
			log.Printf("[COST] Connection error: %v (reconnecting in %v)", err, backoff)
		}

		select {
		case <-uc.ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff capped at maxBackoff
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectAndPoll opens a WS connection, authenticates, and polls until failure.
func (uc *UsageCollector) connectAndPoll() error {
	uc.mu.Lock()
	uc.status = StatusConnecting
	uc.mu.Unlock()

	ctx, cancel := context.WithTimeout(uc.ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, uc.wsURL, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow() //nolint:errcheck // best-effort cleanup

	// Set a generous read limit for usage.cost responses (they can be large)
	conn.SetReadLimit(1 << 20) // 1MB

	// Step 1: Receive connect.challenge
	challenge, err := uc.readFrame(conn)
	if err != nil {
		return fmt.Errorf("read challenge: %w", err)
	}
	if challenge.Event != "connect.challenge" {
		return fmt.Errorf("expected connect.challenge, got event=%q type=%q", challenge.Event, challenge.Type)
	}

	// Step 2: Send connect request with token auth
	connectID := uuid.NewString()
	connectReq := wsRequest{
		Type:   "req",
		ID:     connectID,
		Method: "connect",
		Params: connectParams{
			MinProtocol: 3,
			MaxProtocol: 3,
			Client: connectClient{
				ID:       "gateway-client",
				Version:  "1.0.0",
				Platform: "linux",
				Mode:     "backend",
			},
			Role:   "operator",
			Scopes: []string{"operator.read"},
			Auth:   connectAuth{Token: uc.token},
		},
	}

	if err := uc.writeFrame(conn, connectReq); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	// Step 3: Read connect response (hello-ok)
	helloResp, err := uc.readFrame(conn)
	if err != nil {
		return fmt.Errorf("read hello: %w", err)
	}
	if helloResp.ID == connectID && !helloResp.OK {
		errMsg := "unknown"
		if helloResp.Error != nil {
			errMsg = helloResp.Error.Message
		}
		return fmt.Errorf("connect rejected: %s", errMsg)
	}

	uc.mu.Lock()
	uc.status = StatusConnected
	uc.mu.Unlock()
	log.Println("[COST] Connected to OpenClaw usage API")

	// Reset backoff on successful connect (caller manages this via return nil pattern)
	// Step 4: Poll loop
	return uc.pollLoop(conn)
}

// pollLoop periodically sends usage.cost requests and processes responses.
func (uc *UsageCollector) pollLoop(conn *websocket.Conn) error {
	// Fetch immediately on connect
	if err := uc.fetchUsage(conn); err != nil {
		return err
	}

	ticker := time.NewTicker(uc.interval)
	defer ticker.Stop()

	// Also read incoming frames (tick events) in a separate goroutine
	errCh := make(chan error, 1)
	frameCh := make(chan wsResponse, 16)
	go func() {
		for {
			frame, err := uc.readFrame(conn)
			if err != nil {
				errCh <- err
				return
			}
			frameCh <- frame
		}
	}()

	// Map of pending request IDs
	pending := make(map[string]chan wsResponse)
	var pendingMu sync.Mutex

	for {
		select {
		case <-uc.ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "shutdown")
			return nil

		case err := <-errCh:
			return fmt.Errorf("read: %w", err)

		case frame := <-frameCh:
			if frame.Type == "res" && frame.ID != "" {
				pendingMu.Lock()
				if ch, ok := pending[frame.ID]; ok {
					ch <- frame
					delete(pending, frame.ID)
				}
				pendingMu.Unlock()
			}
			// tick events are ignored (they just keep the connection alive)

		case <-ticker.C:
			// Send usage.cost
			reqID := uuid.NewString()
			req := wsRequest{
				Type:   "req",
				ID:     reqID,
				Method: "usage.cost",
				Params: map[string]interface{}{"days": 30},
			}

			ch := make(chan wsResponse, 1)
			pendingMu.Lock()
			pending[reqID] = ch
			pendingMu.Unlock()

			if err := uc.writeFrame(conn, req); err != nil {
				return fmt.Errorf("write usage.cost: %w", err)
			}

			// Wait for response with timeout
			select {
			case resp := <-ch:
				if err := uc.processUsageResponse(resp); err != nil {
					log.Printf("[COST] Failed to process usage response: %v", err)
				}
			case <-time.After(15 * time.Second):
				log.Println("[COST] usage.cost request timed out")
				pendingMu.Lock()
				delete(pending, reqID)
				pendingMu.Unlock()
			case <-uc.ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "shutdown")
				return nil
			}

			// Send sessions.usage
			startDate, endDate := sessionsUsageDateRange()
			sessID := uuid.NewString()
			sessReq := wsRequest{
				Type:   "req",
				ID:     sessID,
				Method: "sessions.usage",
				Params: map[string]interface{}{
					"startDate": startDate,
					"endDate":   endDate,
					"limit":     1,
				},
			}

			sessCh := make(chan wsResponse, 1)
			pendingMu.Lock()
			pending[sessID] = sessCh
			pendingMu.Unlock()

			if err := uc.writeFrame(conn, sessReq); err != nil {
				return fmt.Errorf("write sessions.usage: %w", err)
			}

			select {
			case resp := <-sessCh:
				if err := uc.processSessionsUsageResponse(resp); err != nil {
					log.Printf("[COST] Failed to process sessions.usage: %v", err)
				}
			case <-time.After(15 * time.Second):
				log.Println("[COST] sessions.usage request timed out")
				pendingMu.Lock()
				delete(pending, sessID)
				pendingMu.Unlock()
			case <-uc.ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "shutdown")
				return nil
			}
		}
	}
}

// fetchUsage sends usage.cost + sessions.usage requests and blocks until
// both responses are received.
func (uc *UsageCollector) fetchUsage(conn *websocket.Conn) error {
	// Send usage.cost
	costID := uuid.NewString()
	costReq := wsRequest{
		Type:   "req",
		ID:     costID,
		Method: "usage.cost",
		Params: map[string]interface{}{"days": 30},
	}
	if err := uc.writeFrame(conn, costReq); err != nil {
		return fmt.Errorf("write usage.cost: %w", err)
	}

	// Send sessions.usage
	startDate, endDate := sessionsUsageDateRange()
	sessID := uuid.NewString()
	sessReq := wsRequest{
		Type:   "req",
		ID:     sessID,
		Method: "sessions.usage",
		Params: map[string]interface{}{
			"startDate": startDate,
			"endDate":   endDate,
			"limit":     1,
		},
	}
	if err := uc.writeFrame(conn, sessReq); err != nil {
		return fmt.Errorf("write sessions.usage: %w", err)
	}

	// Read frames until we get both responses (skip events)
	gotCost, gotSess := false, false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && (!gotCost || !gotSess) {
		frame, err := uc.readFrame(conn)
		if err != nil {
			return fmt.Errorf("read usage response: %w", err)
		}
		if frame.Type != "res" {
			continue // skip events
		}
		switch frame.ID {
		case costID:
			if err := uc.processUsageResponse(frame); err != nil {
				return err
			}
			gotCost = true
		case sessID:
			if err := uc.processSessionsUsageResponse(frame); err != nil {
				log.Printf("[COST] Failed to process sessions.usage: %v", err)
			}
			gotSess = true
		}
	}
	if !gotCost {
		return fmt.Errorf("usage.cost response timed out")
	}
	if !gotSess {
		log.Println("[COST] sessions.usage response timed out (non-fatal)")
	}
	return nil
}

// processUsageResponse parses the usage.cost response and updates the cache.
func (uc *UsageCollector) processUsageResponse(resp wsResponse) error {
	if !resp.OK {
		errMsg := "unknown"
		if resp.Error != nil {
			errMsg = resp.Error.Message
		}
		return fmt.Errorf("usage.cost error: %s", errMsg)
	}

	// Parse the raw response — OpenClaw returns updatedAt as epoch ms
	var raw struct {
		UpdatedAt float64               `json:"updatedAt"`
		Days      int                   `json:"days"`
		Daily     []CostUsageDailyEntry `json:"daily"`
		Totals    CostUsageTotals       `json:"totals"`
	}
	if err := json.Unmarshal(resp.Payload, &raw); err != nil {
		return fmt.Errorf("unmarshal usage: %w", err)
	}

	summary := &CostUsageSummary{
		UpdatedAt: time.UnixMilli(int64(raw.UpdatedAt)),
		Days:      raw.Days,
		Daily:     raw.Daily,
		Totals:    raw.Totals,
	}

	uc.mu.Lock()
	uc.data = summary
	uc.lastOK = time.Now()
	uc.mu.Unlock()

	// Log alert level
	today := time.Now().UTC().Format("2006-01-02")
	var todayCost float64
	if len(summary.Daily) > 0 {
		last := summary.Daily[len(summary.Daily)-1]
		if last.Date == today {
			todayCost = last.TotalCost
		}
	}

	if todayCost >= uc.critUSD {
		log.Printf("[COST] CRITICAL: today's cost $%.2f exceeds critical threshold $%.2f", todayCost, uc.critUSD)
	} else if todayCost >= uc.warnUSD {
		log.Printf("[COST] WARNING: today's cost $%.2f exceeds warning threshold $%.2f", todayCost, uc.warnUSD)
	}

	return nil
}

// sessionsUsageDateRange returns the start/end dates for sessions.usage requests
// (30-day rolling window matching usage.cost).
func sessionsUsageDateRange() (string, string) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -29).Format("2006-01-02")
	end := now.Format("2006-01-02")
	return start, end
}

// processSessionsUsageResponse parses the sessions.usage response and updates the cache.
func (uc *UsageCollector) processSessionsUsageResponse(resp wsResponse) error {
	if !resp.OK {
		errMsg := "unknown"
		if resp.Error != nil {
			errMsg = resp.Error.Message
		}
		return fmt.Errorf("sessions.usage error: %s", errMsg)
	}

	var raw struct {
		UpdatedAt  float64 `json:"updatedAt"`
		StartDate  string  `json:"startDate"`
		EndDate    string  `json:"endDate"`
		Aggregates struct {
			ByModel    []ModelUsageEntry   `json:"byModel"`
			ByProvider []ModelUsageEntry   `json:"byProvider"`
			Daily      []SessionDailyEntry `json:"daily"`
			ModelDaily []ModelDailyEntry   `json:"modelDaily"`
			Messages   MessageCounts       `json:"messages"`
			Tools      ToolUsage           `json:"tools"`
		} `json:"aggregates"`
	}
	if err := json.Unmarshal(resp.Payload, &raw); err != nil {
		return fmt.Errorf("unmarshal sessions.usage: %w", err)
	}

	data := &SessionsUsageData{
		UpdatedAt:  time.UnixMilli(int64(raw.UpdatedAt)),
		StartDate:  raw.StartDate,
		EndDate:    raw.EndDate,
		ByModel:    raw.Aggregates.ByModel,
		ByProvider: raw.Aggregates.ByProvider,
		Daily:      raw.Aggregates.Daily,
		ModelDaily: raw.Aggregates.ModelDaily,
		Messages:   raw.Aggregates.Messages,
		Tools:      raw.Aggregates.Tools,
	}

	// Ensure slices are non-nil for clean JSON serialization
	if data.ByModel == nil {
		data.ByModel = []ModelUsageEntry{}
	}
	if data.ByProvider == nil {
		data.ByProvider = []ModelUsageEntry{}
	}
	if data.Daily == nil {
		data.Daily = []SessionDailyEntry{}
	}
	if data.Tools.Tools == nil {
		data.Tools.Tools = []ToolEntry{}
	}

	uc.mu.Lock()
	uc.sessionsData = data
	uc.mu.Unlock()

	log.Printf("[COST] sessions.usage: %d models, %d providers, %d daily entries",
		len(data.ByModel), len(data.ByProvider), len(data.Daily))
	return nil
}

// readFrame reads and decodes one JSON frame from the WS connection.
func (uc *UsageCollector) readFrame(conn *websocket.Conn) (wsResponse, error) {
	ctx, cancel := context.WithTimeout(uc.ctx, 120*time.Second)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err != nil {
		return wsResponse{}, err
	}

	var frame wsResponse
	if err := json.Unmarshal(data, &frame); err != nil {
		return wsResponse{}, fmt.Errorf("decode frame: %w (data=%s)", err, truncate(string(data), 200))
	}
	return frame, nil
}

// writeFrame encodes and sends one JSON frame to the WS connection.
func (uc *UsageCollector) writeFrame(conn *websocket.Conn, frame interface{}) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}

	ctx, cancel := context.WithTimeout(uc.ctx, 10*time.Second)
	defer cancel()

	return conn.Write(ctx, websocket.MessageText, data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
