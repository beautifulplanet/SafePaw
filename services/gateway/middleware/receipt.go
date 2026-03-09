// =============================================================
// SafePaw Gateway - Append-Only Receipt Ledger
// =============================================================
// Records an immutable, sequenced trail of every agent action
// that flows through the WebSocket proxy. Each receipt captures:
//
//   agent action → tool call → tool result → quality verdict
//
// WHY an append-only ledger?
//   The gateway audit trail (SecurityContext) answers "did the
//   request pass security checks?" The receipt ledger answers
//   "what did the agent actually DO once it got through?"
//
//   After thousands of completions, this traceability separates
//   "it ran" from "I can prove it ran correctly."
//
// DESIGN:
//   - Monotonic sequence numbers (no gaps, no reordering)
//   - Immutable entries (append-only, no updates or deletes)
//   - Bounded ring buffer to prevent unbounded memory growth
//   - Query by request_id, session_id, subject, or time range
//   - Zero external dependencies (no DB required)
//
// OPSEC Lesson #19: "If you can't prove what an agent did,
// you can't prove it did it correctly."
// =============================================================

package middleware

import (
	"sync"
	"sync/atomic"
	"time"
)

// Receipt actions — the lifecycle of an agent session.
const (
	ActionSessionStart = "session_start" // WebSocket tunnel opened
	ActionToolCall     = "tool_call"     // Agent invoked a tool
	ActionToolResult   = "tool_result"   // Tool returned a result
	ActionAgentText    = "agent_text"    // Agent produced text output
	ActionSessionEnd   = "session_end"   // WebSocket tunnel closed
	ActionQualityFlag  = "quality_flag"  // Output risk detected
)

// Receipt is a single immutable entry in the ledger.
// Once appended, it is never modified or deleted.
type Receipt struct {
	Seq        uint64    `json:"seq"`                   // monotonic sequence number
	Timestamp  time.Time `json:"ts"`                    // when this event was recorded
	RequestID  string    `json:"request_id"`            // ties to gateway audit record
	SessionID  string    `json:"session_id"`            // WebSocket session identifier
	Subject    string    `json:"subject,omitempty"`     // authenticated user (from X-Auth-Subject)
	Action     string    `json:"action"`                // action type (see constants above)
	Tool       string    `json:"tool,omitempty"`        // tool name for tool_call/tool_result
	Risk       string    `json:"risk,omitempty"`        // output risk level if flagged
	DurationMs int64     `json:"duration_ms,omitempty"` // elapsed time for completed actions
	Summary    string    `json:"summary,omitempty"`     // truncated description (max 200 chars)
}

const (
	defaultLedgerSize = 10000 // keep last 10k receipts in memory
	maxSummaryLen     = 200   // truncate summaries to prevent memory bloat
)

// Ledger is an append-only, bounded, in-memory receipt store.
// It uses a ring buffer: when full, the oldest entries are overwritten.
// Sequence numbers are globally monotonic and never reset.
type Ledger struct {
	mu      sync.RWMutex
	entries []Receipt
	head    int // next write position in ring buffer
	count   int // current number of valid entries (≤ maxSize)
	maxSize int // ring buffer capacity
	seq     atomic.Uint64
}

// NewLedger creates a receipt ledger with the given capacity.
// If maxSize ≤ 0, uses defaultLedgerSize.
func NewLedger(maxSize int) *Ledger {
	if maxSize <= 0 {
		maxSize = defaultLedgerSize
	}
	return &Ledger{
		entries: make([]Receipt, maxSize),
		maxSize: maxSize,
	}
}

// Append adds an immutable receipt to the ledger.
// Returns the assigned sequence number.
func (l *Ledger) Append(r Receipt) uint64 {
	r.Seq = l.seq.Add(1)
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	if len(r.Summary) > maxSummaryLen {
		r.Summary = r.Summary[:maxSummaryLen]
	}

	l.mu.Lock()
	l.entries[l.head] = r
	l.head = (l.head + 1) % l.maxSize
	if l.count < l.maxSize {
		l.count++
	}
	l.mu.Unlock()

	return r.Seq
}

// Count returns the number of receipts currently stored.
func (l *Ledger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

// LastSeq returns the most recent sequence number (0 if empty).
func (l *Ledger) LastSeq() uint64 {
	return l.seq.Load()
}

// LedgerQuery filters receipts when querying the ledger.
type LedgerQuery struct {
	RequestID string    // filter by request ID
	SessionID string    // filter by WebSocket session
	Subject   string    // filter by authenticated user
	Action    string    // filter by action type
	SinceSeq  uint64    // only entries with seq > SinceSeq
	Since     time.Time // only entries after this time
	Limit     int       // max results (0 = all matching)
}

// Query returns receipts matching the filter, ordered by sequence number.
// Returns a copy — callers cannot modify ledger entries.
func (l *Ledger) Query(q LedgerQuery) []Receipt {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.count == 0 {
		return nil
	}

	// Determine iteration start (oldest entry in ring buffer)
	start := 0
	if l.count == l.maxSize {
		start = l.head // oldest is at current write position (about to be overwritten)
	}

	limit := q.Limit
	if limit <= 0 || limit > l.count {
		limit = l.count
	}

	var results []Receipt
	for i := 0; i < l.count && len(results) < limit; i++ {
		idx := (start + i) % l.maxSize
		e := l.entries[idx]

		if q.SinceSeq > 0 && e.Seq <= q.SinceSeq {
			continue
		}
		if !q.Since.IsZero() && e.Timestamp.Before(q.Since) {
			continue
		}
		if q.RequestID != "" && e.RequestID != q.RequestID {
			continue
		}
		if q.SessionID != "" && e.SessionID != q.SessionID {
			continue
		}
		if q.Subject != "" && e.Subject != q.Subject {
			continue
		}
		if q.Action != "" && e.Action != q.Action {
			continue
		}

		results = append(results, e) // copy
	}

	return results
}

// Recent returns the last N receipts in sequence order.
func (l *Ledger) Recent(n int) []Receipt {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.count == 0 {
		return nil
	}
	if n <= 0 || n > l.count {
		n = l.count
	}

	results := make([]Receipt, n)

	// Start from (head - n) in ring buffer, wrapping around
	start := l.head - n
	if start < 0 {
		start += l.maxSize
	}
	for i := 0; i < n; i++ {
		idx := (start + i) % l.maxSize
		results[i] = l.entries[idx]
	}

	return results
}

// TruncateSummary safely truncates a string for use as a receipt summary.
func TruncateSummary(s string) string {
	if len(s) <= maxSummaryLen {
		return s
	}
	return s[:maxSummaryLen]
}
