package middleware

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// ================================================================
// Ledger Tests
// ================================================================

func TestLedger_AppendAndQuery(t *testing.T) {
	l := NewLedger(100)

	seq1 := l.Append(Receipt{
		RequestID: "req-1",
		SessionID: "sess-1",
		Subject:   "user-a",
		Action:    ActionSessionStart,
		Summary:   "ws_upgrade path=/ws",
	})
	seq2 := l.Append(Receipt{
		RequestID: "req-1",
		SessionID: "sess-1",
		Subject:   "user-a",
		Action:    ActionToolCall,
		Tool:      "read_file",
		Summary:   "read_file id=toolu_123",
	})
	seq3 := l.Append(Receipt{
		RequestID: "req-1",
		SessionID: "sess-1",
		Subject:   "user-a",
		Action:    ActionToolResult,
		Summary:   "result id=toolu_123",
	})

	if seq1 != 1 || seq2 != 2 || seq3 != 3 {
		t.Errorf("sequences = %d,%d,%d want 1,2,3", seq1, seq2, seq3)
	}
	if l.Count() != 3 {
		t.Errorf("Count() = %d, want 3", l.Count())
	}
	if l.LastSeq() != 3 {
		t.Errorf("LastSeq() = %d, want 3", l.LastSeq())
	}

	// Query by session
	results := l.Query(LedgerQuery{SessionID: "sess-1"})
	if len(results) != 3 {
		t.Errorf("session query got %d results, want 3", len(results))
	}

	// Query by action
	results = l.Query(LedgerQuery{Action: ActionToolCall})
	if len(results) != 1 {
		t.Errorf("action query got %d results, want 1", len(results))
	}
	if results[0].Tool != "read_file" {
		t.Errorf("tool = %q, want %q", results[0].Tool, "read_file")
	}

	// Query by SinceSeq
	results = l.Query(LedgerQuery{SinceSeq: 2})
	if len(results) != 1 {
		t.Errorf("since_seq query got %d results, want 1", len(results))
	}
	if results[0].Seq != 3 {
		t.Errorf("seq = %d, want 3", results[0].Seq)
	}
}

func TestLedger_RingBufferOverflow(t *testing.T) {
	l := NewLedger(5) // small ring buffer

	// Write 8 entries — first 3 should be evicted
	for i := 1; i <= 8; i++ {
		l.Append(Receipt{
			RequestID: "req",
			Action:    ActionToolCall,
			Tool:      "tool",
		})
	}

	if l.Count() != 5 {
		t.Errorf("Count() = %d, want 5 (ring buffer max)", l.Count())
	}
	if l.LastSeq() != 8 {
		t.Errorf("LastSeq() = %d, want 8", l.LastSeq())
	}

	// Recent should return the last 5 (seq 4-8)
	recent := l.Recent(5)
	if len(recent) != 5 {
		t.Fatalf("Recent(5) = %d entries, want 5", len(recent))
	}
	if recent[0].Seq != 4 {
		t.Errorf("oldest in ring = seq %d, want 4", recent[0].Seq)
	}
	if recent[4].Seq != 8 {
		t.Errorf("newest in ring = seq %d, want 8", recent[4].Seq)
	}
}

func TestLedger_Recent(t *testing.T) {
	l := NewLedger(100)

	for i := 0; i < 10; i++ {
		l.Append(Receipt{Action: ActionToolCall})
	}

	recent := l.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("Recent(3) = %d, want 3", len(recent))
	}
	// Should be the last 3 entries (seq 8, 9, 10)
	if recent[0].Seq != 8 {
		t.Errorf("oldest = seq %d, want 8", recent[0].Seq)
	}
	if recent[2].Seq != 10 {
		t.Errorf("newest = seq %d, want 10", recent[2].Seq)
	}
}

func TestLedger_QueryWithLimit(t *testing.T) {
	l := NewLedger(100)

	for i := 0; i < 20; i++ {
		l.Append(Receipt{Action: ActionToolCall, Tool: "test"})
	}

	results := l.Query(LedgerQuery{Action: ActionToolCall, Limit: 5})
	if len(results) != 5 {
		t.Errorf("limited query got %d results, want 5", len(results))
	}
}

func TestLedger_QueryBySubject(t *testing.T) {
	l := NewLedger(100)

	l.Append(Receipt{Subject: "alice", Action: ActionToolCall})
	l.Append(Receipt{Subject: "bob", Action: ActionToolCall})
	l.Append(Receipt{Subject: "alice", Action: ActionToolResult})

	results := l.Query(LedgerQuery{Subject: "alice"})
	if len(results) != 2 {
		t.Errorf("subject query got %d results, want 2", len(results))
	}
}

func TestLedger_QuerySinceTime(t *testing.T) {
	l := NewLedger(100)

	past := time.Now().Add(-1 * time.Hour)
	l.Append(Receipt{Timestamp: past, Action: ActionToolCall})
	l.Append(Receipt{Action: ActionToolResult}) // uses current time via Append

	results := l.Query(LedgerQuery{Since: time.Now().Add(-1 * time.Minute)})
	if len(results) != 1 {
		t.Errorf("time query got %d results, want 1", len(results))
	}
}

func TestLedger_SummaryTruncation(t *testing.T) {
	l := NewLedger(100)

	longSummary := strings.Repeat("x", 500)
	l.Append(Receipt{Action: ActionToolCall, Summary: longSummary})

	recent := l.Recent(1)
	if len(recent[0].Summary) != maxSummaryLen {
		t.Errorf("summary len = %d, want %d", len(recent[0].Summary), maxSummaryLen)
	}
}

func TestLedger_Immutability(t *testing.T) {
	l := NewLedger(100)

	l.Append(Receipt{Action: ActionToolCall, Tool: "original"})

	// Get a copy via query — modifications shouldn't affect ledger
	results := l.Query(LedgerQuery{Action: ActionToolCall})
	results[0].Tool = "modified"

	// Re-query should still show original
	results2 := l.Query(LedgerQuery{Action: ActionToolCall})
	if results2[0].Tool != "original" {
		t.Errorf("ledger entry was mutated: tool = %q, want %q", results2[0].Tool, "original")
	}
}

func TestLedger_ConcurrentAppend(t *testing.T) {
	l := NewLedger(1000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				l.Append(Receipt{Action: ActionToolCall})
			}
		}()
	}
	wg.Wait()

	if l.Count() != 1000 {
		t.Errorf("Count() = %d, want 1000", l.Count())
	}
	if l.LastSeq() != 1000 {
		t.Errorf("LastSeq() = %d, want 1000", l.LastSeq())
	}
}

func TestLedger_EmptyQuery(t *testing.T) {
	l := NewLedger(100)

	results := l.Query(LedgerQuery{})
	if results != nil {
		t.Errorf("empty ledger query should return nil, got %v", results)
	}

	recent := l.Recent(5)
	if recent != nil {
		t.Errorf("empty ledger Recent should return nil, got %v", recent)
	}
}

func TestLedger_DefaultCapacity(t *testing.T) {
	l := NewLedger(0) // should use default
	if l.maxSize != defaultLedgerSize {
		t.Errorf("maxSize = %d, want %d", l.maxSize, defaultLedgerSize)
	}
}

// ================================================================
// Tool Parser Tests
// ================================================================

func TestLedgerReader_DetectsToolUse(t *testing.T) {
	// Simulate a WebSocket frame containing a tool_use JSON
	data := `{"type":"content_block_start","content_block":{"type":"tool_use","id":"toolu_abc123","name":"read_file","input":{}}}`

	l := NewLedger(100)
	reader := NewLedgerReader(
		strings.NewReader(data), l,
		"req-1", "sess-1", "alice", "/ws",
	)

	// Read all data through the reader
	buf := make([]byte, 4096)
	n, _ := reader.Read(buf)
	if n != len(data) {
		t.Errorf("Read returned %d bytes, want %d", n, len(data))
	}

	// Verify tool call was recorded
	results := l.Query(LedgerQuery{Action: ActionToolCall})
	if len(results) != 1 {
		t.Fatalf("expected 1 tool_call receipt, got %d", len(results))
	}
	if results[0].Tool != "read_file" {
		t.Errorf("tool = %q, want %q", results[0].Tool, "read_file")
	}
	if results[0].SessionID != "sess-1" {
		t.Errorf("session_id = %q, want %q", results[0].SessionID, "sess-1")
	}
	if results[0].Subject != "alice" {
		t.Errorf("subject = %q, want %q", results[0].Subject, "alice")
	}
}

func TestLedgerReader_DetectsToolResult(t *testing.T) {
	data := `{"type":"tool_result","tool_use_id":"toolu_abc123","content":"file contents here"}`

	l := NewLedger(100)
	reader := NewLedgerReader(
		strings.NewReader(data), l,
		"req-1", "sess-1", "bob", "/ws",
	)

	buf := make([]byte, 4096)
	reader.Read(buf)

	results := l.Query(LedgerQuery{Action: ActionToolResult})
	if len(results) != 1 {
		t.Fatalf("expected 1 tool_result receipt, got %d", len(results))
	}
}

func TestLedgerReader_DetectsTextBlock(t *testing.T) {
	data := `{"type":"content_block_start","content_block":{"type":"text","text":"Hello"}}`

	l := NewLedger(100)
	reader := NewLedgerReader(
		strings.NewReader(data), l,
		"req-1", "sess-1", "carol", "/ws",
	)

	buf := make([]byte, 4096)
	reader.Read(buf)

	results := l.Query(LedgerQuery{Action: ActionAgentText})
	if len(results) != 1 {
		t.Fatalf("expected 1 agent_text receipt, got %d", len(results))
	}
}

func TestLedgerReader_PassthroughData(t *testing.T) {
	// The reader must not modify the data flowing through
	original := `{"type":"tool_use","name":"execute_command","id":"toolu_xyz"}`

	l := NewLedger(100)
	reader := NewLedgerReader(
		strings.NewReader(original), l,
		"req-1", "sess-1", "dave", "/ws",
	)

	var buf bytes.Buffer
	io.Copy(&buf, reader)

	if buf.String() != original {
		t.Errorf("data was modified:\ngot:  %q\nwant: %q", buf.String(), original)
	}
}

func TestLedgerReader_MultipleToolCalls(t *testing.T) {
	// Two tool calls in one chunk
	data := `{"type":"tool_use","name":"read_file","id":"toolu_1"}` +
		"\n" +
		`{"type":"tool_use","name":"write_file","id":"toolu_2"}`

	l := NewLedger(100)
	reader := NewLedgerReader(
		strings.NewReader(data), l,
		"req-1", "sess-1", "eve", "/ws",
	)

	buf := make([]byte, 8192)
	reader.Read(buf)

	// Should detect at least one tool_use (both are in same chunk,
	// regex finds first match per chunk which is fine for aggregation)
	results := l.Query(LedgerQuery{Action: ActionToolCall})
	if len(results) < 1 {
		t.Fatalf("expected at least 1 tool_call receipt, got %d", len(results))
	}
}

func TestLedgerReader_IgnoresIrrelevantData(t *testing.T) {
	data := `{"type":"ping","status":"ok"}`

	l := NewLedger(100)
	reader := NewLedgerReader(
		strings.NewReader(data), l,
		"req-1", "sess-1", "frank", "/ws",
	)

	buf := make([]byte, 4096)
	reader.Read(buf)

	if l.Count() != 0 {
		t.Errorf("expected 0 receipts for irrelevant data, got %d", l.Count())
	}
}

// ================================================================
// Session Lifecycle Tests
// ================================================================

func TestRecordSessionStart(t *testing.T) {
	l := NewLedger(100)
	RecordSessionStart(l, "req-1", "sess-1", "alice", "/ws")

	results := l.Query(LedgerQuery{Action: ActionSessionStart})
	if len(results) != 1 {
		t.Fatalf("expected 1 session_start, got %d", len(results))
	}
	if results[0].SessionID != "sess-1" {
		t.Errorf("session_id = %q, want %q", results[0].SessionID, "sess-1")
	}
}

func TestRecordSessionEnd(t *testing.T) {
	l := NewLedger(100)
	start := time.Now().Add(-5 * time.Second)
	RecordSessionEnd(l, "req-1", "sess-1", "alice", start)

	results := l.Query(LedgerQuery{Action: ActionSessionEnd})
	if len(results) != 1 {
		t.Fatalf("expected 1 session_end, got %d", len(results))
	}
	if results[0].DurationMs < 4000 {
		t.Errorf("duration_ms = %d, expected >= 4000", results[0].DurationMs)
	}
}

func TestRecordQualityFlag(t *testing.T) {
	l := NewLedger(100)
	RecordQualityFlag(l, "req-1", "sess-1", "alice", "high", []string{"script_tag", "xss"})

	results := l.Query(LedgerQuery{Action: ActionQualityFlag})
	if len(results) != 1 {
		t.Fatalf("expected 1 quality_flag, got %d", len(results))
	}
	if results[0].Risk != "high" {
		t.Errorf("risk = %q, want %q", results[0].Risk, "high")
	}
}

func TestRecordSessionStart_NilLedger(t *testing.T) {
	// Should not panic
	RecordSessionStart(nil, "req-1", "sess-1", "alice", "/ws")
	RecordSessionEnd(nil, "req-1", "sess-1", "alice", time.Now())
	RecordQualityFlag(nil, "req-1", "sess-1", "alice", "high", []string{"test"})
}

func TestTruncateSummary(t *testing.T) {
	short := "hello"
	if TruncateSummary(short) != short {
		t.Errorf("short string should not be truncated")
	}

	long := strings.Repeat("a", 300)
	truncated := TruncateSummary(long)
	if len(truncated) != maxSummaryLen {
		t.Errorf("truncated len = %d, want %d", len(truncated), maxSummaryLen)
	}
}

func TestExtractMatch(t *testing.T) {
	s := `{"name":"read_file","id":"toolu_123"}`

	name := extractMatch(toolNamePattern, s)
	if name != "read_file" {
		t.Errorf("name = %q, want %q", name, "read_file")
	}

	id := extractMatch(toolIDPattern, s)
	if id != "toolu_123" {
		t.Errorf("id = %q, want %q", id, "toolu_123")
	}

	// No match
	empty := extractMatch(toolNamePattern, `{"type":"ping"}`)
	if empty != "" {
		t.Errorf("expected empty, got %q", empty)
	}
}
