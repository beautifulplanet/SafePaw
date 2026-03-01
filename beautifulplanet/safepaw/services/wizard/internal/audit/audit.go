// =============================================================
// SafePaw Wizard - Audit Logger
// =============================================================
// Structured audit trail for all admin actions. Every mutation
// (login, config change, service restart) is recorded with:
//
//   who   — IP address / session identifier
//   what  — action performed
//   when  — timestamp (UTC)
//   where — resource affected
//   how   — outcome (success/failure)
//
// Output format (LOG_FORMAT=json):
//   {"ts":"...","type":"audit","actor":"10.0.0.1","action":"login","resource":"admin","outcome":"success"}
//
// Output format (default text):
//   [AUDIT] action=login actor=10.0.0.1 resource=admin outcome=success
//
// In production, pipe stdout to a log aggregator (Fluentd, Vector,
// CloudWatch) for compliance and forensics.
// =============================================================

package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Event represents an auditable action.
type Event struct {
	Timestamp time.Time         `json:"ts"`
	Type      string            `json:"type"`
	Actor     string            `json:"actor"`
	Action    string            `json:"action"`
	Resource  string            `json:"resource"`
	Outcome   string            `json:"outcome"`
	Details   map[string]string `json:"details,omitempty"`
}

// Logger writes structured audit events.
type Logger struct {
	mu       sync.Mutex
	out      io.Writer
	jsonMode bool
}

// New creates an audit logger.
func New() *Logger {
	jsonMode := strings.EqualFold(os.Getenv("LOG_FORMAT"), "json")
	return &Logger{
		out:      os.Stdout,
		jsonMode: jsonMode,
	}
}

// NewWithWriter creates an audit logger writing to a custom writer (for testing).
func NewWithWriter(w io.Writer, jsonMode bool) *Logger {
	return &Logger{out: w, jsonMode: jsonMode}
}

// Log records an audit event.
func (l *Logger) Log(actor, action, resource, outcome string, details map[string]string) {
	ev := Event{
		Timestamp: time.Now().UTC(),
		Type:      "audit",
		Actor:     actor,
		Action:    action,
		Resource:  resource,
		Outcome:   outcome,
		Details:   details,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.jsonMode {
		data, _ := json.Marshal(ev)
		l.out.Write(data)
		l.out.Write([]byte("\n"))
	} else {
		msg := fmt.Sprintf("[AUDIT] action=%s actor=%s resource=%s outcome=%s",
			action, actor, resource, outcome)
		for k, v := range details {
			msg += fmt.Sprintf(" %s=%s", k, v)
		}
		l.out.Write([]byte(msg + "\n"))
	}

	log.Printf("[AUDIT] %s %s on %s by %s (%s)", outcome, action, resource, actor, ev.Timestamp.Format(time.RFC3339))
}

// LoginSuccess records a successful admin login.
func (l *Logger) LoginSuccess(ip string) {
	l.Log(ip, "login", "admin", "success", nil)
}

// LoginFailure records a failed admin login.
func (l *Logger) LoginFailure(ip, reason string) {
	l.Log(ip, "login", "admin", "failure", map[string]string{"reason": reason})
}

// ConfigChange records a configuration update.
func (l *Logger) ConfigChange(ip string, keys []string) {
	l.Log(ip, "config_update", "env", "success", map[string]string{
		"keys_changed": strings.Join(keys, ","),
	})
}

// ServiceRestart records a service restart request.
func (l *Logger) ServiceRestart(ip, service, outcome string) {
	l.Log(ip, "service_restart", service, outcome, nil)
}
