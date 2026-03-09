// =============================================================
// SafePaw Gateway - WebSocket Tool Call Parser
// =============================================================
// Extracts tool call events from the WebSocket byte stream
// between OpenClaw and the client. This is the "Level 2 audit"
// that traces WHAT the agent did after it passed through the
// gateway security checks.
//
// PARSING STRATEGY:
//   The WebSocket proxy is a raw TCP tunnel — we see interleaved
//   WebSocket frames. Rather than implementing a full WS frame
//   parser (which would add complexity), we scan the raw byte
//   stream for JSON patterns that indicate tool activity.
//
//   This is the same approach the output scanner uses: pattern
//   matching on content as it flows through. It's pragmatic,
//   low-overhead, and catches the events we care about.
//
// RECOGNIZED PATTERNS (Claude/OpenClaw API format):
//   - "type":"tool_use"   → agent is calling a tool
//   - "type":"tool_result" → tool returned a result
//   - "name":"<tool>"     → extracts tool name
//   - "type":"text"       → agent text generation
//
// LIMITATIONS:
//   - Chunked frames may split a JSON object across reads
//   - We use a carryover buffer to handle boundary splits
//   - Very large tool inputs/outputs are truncated in summaries
// =============================================================

package middleware

import (
	"io"
	"log"
	"regexp"
	"strings"
	"time"
)

// JSON patterns for tool call detection in the byte stream.
// These match Claude API / OpenClaw message format.
var (
	toolUsePattern    = regexp.MustCompile(`"type"\s*:\s*"tool_use"`)
	toolResultPattern = regexp.MustCompile(`"type"\s*:\s*"tool_result"`)
	toolNamePattern   = regexp.MustCompile(`"name"\s*:\s*"([^"]{1,100})"`)
	textBlockPattern  = regexp.MustCompile(`"type"\s*:\s*"(?:text|content_block_start)"`)
	toolIDPattern     = regexp.MustCompile(`"id"\s*:\s*"([^"]{1,100})"`)
)

// LedgerReader wraps an io.Reader and records tool call events
// in the receipt ledger as data flows through the WebSocket tunnel.
// It's designed to be inserted in the backend→client stream path,
// alongside (or wrapping) the existing ScanningReader.
type LedgerReader struct {
	source    io.Reader
	ledger    *Ledger
	requestID string
	sessionID string
	subject   string
	path      string
	carryover []byte // leftover bytes from previous read (for split JSON)
}

// NewLedgerReader creates a reader that extracts tool calls and
// records them in the ledger as data flows through.
func NewLedgerReader(source io.Reader, ledger *Ledger, requestID, sessionID, subject, path string) *LedgerReader {
	return &LedgerReader{
		source:    source,
		ledger:    ledger,
		requestID: requestID,
		sessionID: sessionID,
		subject:   subject,
		path:      path,
	}
}

const maxCarryover = 4096 // max bytes to carry across reads for split detection

func (lr *LedgerReader) Read(p []byte) (int, error) {
	n, err := lr.source.Read(p)
	if n > 0 {
		lr.processChunk(p[:n])
	}
	return n, err
}

// processChunk scans a chunk of the byte stream for tool call patterns.
// It prepends any carryover from the previous read to handle JSON
// objects split across read boundaries.
func (lr *LedgerReader) processChunk(chunk []byte) {
	// Prepend carryover for boundary handling
	var data string
	if len(lr.carryover) > 0 {
		combined := make([]byte, len(lr.carryover)+len(chunk))
		copy(combined, lr.carryover)
		copy(combined[len(lr.carryover):], chunk)
		data = string(combined)
		lr.carryover = nil
	} else {
		data = string(chunk)
	}

	// Save tail as carryover for next read (handles split JSON)
	if len(data) > maxCarryover {
		lr.carryover = []byte(data[len(data)-maxCarryover:])
	} else {
		lr.carryover = []byte(data)
	}

	// Detect tool_use → record tool call
	if toolUsePattern.MatchString(data) {
		toolName := extractMatch(toolNamePattern, data)
		toolID := extractMatch(toolIDPattern, data)
		summary := "tool_call"
		if toolName != "" {
			summary = toolName
		}

		lr.ledger.Append(Receipt{
			Timestamp: time.Now().UTC(),
			RequestID: lr.requestID,
			SessionID: lr.sessionID,
			Subject:   lr.subject,
			Action:    ActionToolCall,
			Tool:      toolName,
			Summary:   TruncateSummary(summary + " id=" + toolID),
		})

		log.Printf("[LEDGER] tool_call: tool=%s session=%s request_id=%s",
			SanitizeLogValue(toolName), lr.sessionID, lr.requestID)
	}

	// Detect tool_result → record tool result
	if toolResultPattern.MatchString(data) {
		toolID := extractMatch(toolIDPattern, data)
		lr.ledger.Append(Receipt{
			Timestamp: time.Now().UTC(),
			RequestID: lr.requestID,
			SessionID: lr.sessionID,
			Subject:   lr.subject,
			Action:    ActionToolResult,
			Summary:   TruncateSummary("result id=" + toolID),
		})
	}

	// Detect text blocks (agent generating non-tool output)
	if textBlockPattern.MatchString(data) && !toolUsePattern.MatchString(data) {
		lr.ledger.Append(Receipt{
			Timestamp: time.Now().UTC(),
			RequestID: lr.requestID,
			SessionID: lr.sessionID,
			Subject:   lr.subject,
			Action:    ActionAgentText,
		})
	}
}

// extractMatch returns the first submatch group from a regex, or "".
func extractMatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// RecordSessionStart writes a session_start receipt when a WS tunnel opens.
func RecordSessionStart(ledger *Ledger, requestID, sessionID, subject, path string) {
	if ledger == nil {
		return
	}
	ledger.Append(Receipt{
		Timestamp: time.Now().UTC(),
		RequestID: requestID,
		SessionID: sessionID,
		Subject:   subject,
		Action:    ActionSessionStart,
		Summary:   TruncateSummary("ws_upgrade path=" + path),
	})
}

// RecordSessionEnd writes a session_end receipt when a WS tunnel closes.
func RecordSessionEnd(ledger *Ledger, requestID, sessionID, subject string, startTime time.Time) {
	if ledger == nil {
		return
	}
	ledger.Append(Receipt{
		Timestamp:  time.Now().UTC(),
		RequestID:  requestID,
		SessionID:  sessionID,
		Subject:    subject,
		Action:     ActionSessionEnd,
		DurationMs: time.Since(startTime).Milliseconds(),
	})
}

// RecordQualityFlag writes a quality_flag receipt when output scanning
// detects risky content in the agent's output stream.
func RecordQualityFlag(ledger *Ledger, requestID, sessionID, subject, risk string, triggers []string) {
	if ledger == nil {
		return
	}
	ledger.Append(Receipt{
		Timestamp: time.Now().UTC(),
		RequestID: requestID,
		SessionID: sessionID,
		Subject:   subject,
		Action:    ActionQualityFlag,
		Risk:      risk,
		Summary:   TruncateSummary("triggers=" + strings.Join(triggers, ",")),
	})
}
