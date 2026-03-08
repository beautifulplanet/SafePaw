// =============================================================
// SafePaw Gateway - Structured Logger (Zero-Dependency)
// =============================================================
// SIEM-compatible structured logging for production environments.
//
// STRATEGY: Intercepts Go's standard log.Printf output and converts
// it to structured JSON when LOG_FORMAT=json. This means ALL existing
// log.Printf("[AUTH] Rejected ...") calls automatically become:
//
//   {"ts":"2024-01-01T00:00:00Z","level":"warn","component":"AUTH","msg":"Rejected ...","fields":{...}}
//
// The parser understands SafePaw's log prefix convention:
//   [AUTH], [SCANNER], [SECURITY], [PROXY], [WS], [RATELIMIT],
//   [REVOKE], [OUTPUT-SCAN], [SANITIZE], [SHUTDOWN], [CONFIG],
//   [SERVER], [FATAL]
//
// In text mode (default, dev): standard log output unchanged.
// In JSON mode (LOG_FORMAT=json): every line becomes a JSON object.
//
// Call InstallJSONLogger() from main() to activate.
// =============================================================

package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogLevel represents severity.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// componentLevel maps log prefixes to severity levels.
var componentLevel = map[string]LogLevel{
	"AUTH":        LevelInfo,
	"SCANNER":     LevelWarn,
	"SECURITY":    LevelWarn,
	"PROXY":       LevelInfo,
	"WS":          LevelInfo,
	"RATELIMIT":   LevelInfo,
	"REVOKE":      LevelWarn,
	"OUTPUT-SCAN": LevelWarn,
	"SANITIZE":    LevelInfo,
	"SHUTDOWN":    LevelInfo,
	"CONFIG":      LevelInfo,
	"SERVER":      LevelInfo,
	"FATAL":       LevelError,
}

// warnKeywords elevate a line to warn if found in the message.
var warnKeywords = []string{"Rejected", "DENIED", "Blocked", "failed", "error", "risk="}

// prefixPattern matches SafePaw log convention: [COMPONENT]
var prefixPattern = regexp.MustCompile(`^\[([A-Z_-]+)\]\s*(.*)$`)

// kvPattern matches key=value pairs in log messages.
var kvPattern = regexp.MustCompile(`(\w+)=("[^"]*"|[^\s]+)`)

// InstallJSONLogger replaces the standard log output with a JSON writer.
// Call from main() before any logging.
// In text mode (LOG_FORMAT != "json"), this is a no-op.
func InstallJSONLogger() bool {
	if !strings.EqualFold(os.Getenv("LOG_FORMAT"), "json") {
		return false
	}
	log.SetFlags(0) // Remove timestamps (we add our own)
	log.SetOutput(&jsonLogWriter{out: os.Stdout})
	return true
}

// jsonLogWriter converts text log lines to JSON.
type jsonLogWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *jsonLogWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}

	entry := make(map[string]interface{}, 8)
	entry["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	entry["service"] = "safepaw-gateway"

	component := ""
	msg := line

	if m := prefixPattern.FindStringSubmatch(line); len(m) == 3 {
		component = m[1]
		msg = m[2]
	}

	if component != "" {
		entry["component"] = component
	}

	level := inferLevel(component, msg)
	entry["level"] = level.String()
	entry["msg"] = cleanMsg(msg)

	fields := extractKV(msg)
	if len(fields) > 0 {
		entry["fields"] = fields
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	data, _ := json.Marshal(entry)
	_, _ = w.out.Write(data)
	_, _ = w.out.Write([]byte("\n"))
	return len(p), nil
}

func inferLevel(component, msg string) LogLevel {
	if level, ok := componentLevel[component]; ok {
		if level == LevelInfo {
			for _, kw := range warnKeywords {
				if strings.Contains(msg, kw) {
					return LevelWarn
				}
			}
		}
		return level
	}
	for _, kw := range warnKeywords {
		if strings.Contains(msg, kw) {
			return LevelWarn
		}
	}
	return LevelInfo
}

func extractKV(msg string) map[string]string {
	matches := kvPattern.FindAllStringSubmatch(msg, -1)
	if len(matches) == 0 {
		return nil
	}
	fields := make(map[string]string, len(matches))
	for _, m := range matches {
		if len(m) == 3 {
			val := m[2]
			val = strings.Trim(val, `"`)
			fields[m[1]] = val
		}
	}
	return fields
}

func cleanMsg(msg string) string {
	idx := strings.IndexByte(msg, '(')
	if idx > 0 {
		return strings.TrimSpace(msg[:idx])
	}
	return msg
}

// ================================================================
// Direct structured logger (for new code that wants explicit fields)
// ================================================================

// Logger is a structured, thread-safe logger for new code.
type Logger struct {
	mu        sync.Mutex
	out       io.Writer
	jsonMode  bool
	minLevel  LogLevel
	component string
}

// Field is a key-value pair for structured logging.
type Field struct {
	Key   string
	Value interface{}
}

// F creates a log field.
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

var defaultLogger *Logger
var loggerOnce sync.Once

// GetLogger returns the global logger (singleton).
func GetLogger() *Logger {
	loggerOnce.Do(func() {
		jsonMode := strings.EqualFold(os.Getenv("LOG_FORMAT"), "json")
		defaultLogger = &Logger{
			out:      os.Stdout,
			jsonMode: jsonMode,
			minLevel: LevelInfo,
		}
	})
	return defaultLogger
}

// WithComponent returns a new logger bound to a component tag.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		out:       l.out,
		jsonMode:  l.jsonMode,
		minLevel:  l.minLevel,
		component: component,
	}
}

func (l *Logger) logEntry(level LogLevel, msg string, fields ...Field) {
	if level < l.minLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.jsonMode {
		l.writeJSON(level, msg, fields)
	} else {
		l.writeText(level, msg, fields)
	}
}

func (l *Logger) writeJSON(level LogLevel, msg string, fields []Field) {
	entry := make(map[string]interface{}, len(fields)+5)
	entry["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	entry["level"] = level.String()
	entry["service"] = "safepaw-gateway"
	if l.component != "" {
		entry["component"] = l.component
	}
	entry["msg"] = msg
	for _, f := range fields {
		entry[f.Key] = f.Value
	}
	data, _ := json.Marshal(entry)
	_, _ = l.out.Write(data)
	_, _ = l.out.Write([]byte("\n"))
}

func (l *Logger) writeText(level LogLevel, msg string, fields []Field) {
	var b strings.Builder
	if l.component != "" {
		fmt.Fprintf(&b, "[%s] ", l.component)
	}
	b.WriteString(msg)
	for _, f := range fields {
		fmt.Fprintf(&b, " %s=%v", f.Key, f.Value)
	}
	b.WriteByte('\n')
	_, _ = l.out.Write([]byte(b.String()))
}

// Info logs at info level.
func (l *Logger) Info(msg string, fields ...Field) { l.logEntry(LevelInfo, msg, fields...) }

// Warn logs at warn level.
func (l *Logger) Warn(msg string, fields ...Field) { l.logEntry(LevelWarn, msg, fields...) }

// Error logs at error level.
func (l *Logger) Error(msg string, fields ...Field) { l.logEntry(LevelError, msg, fields...) }

// Debug logs at debug level.
func (l *Logger) Debug(msg string, fields ...Field) { l.logEntry(LevelDebug, msg, fields...) }

// SecurityEvent logs a SIEM-forwardable security event.
func (l *Logger) SecurityEvent(event, action string, fields ...Field) {
	allFields := make([]Field, 0, len(fields)+2)
	allFields = append(allFields, F("event_type", event), F("action", action))
	allFields = append(allFields, fields...)
	l.logEntry(LevelWarn, "security_event", allFields...)
}

// AuditEvent logs an auditable action (who did what, when).
func (l *Logger) AuditEvent(actor, action, resource string, fields ...Field) {
	allFields := make([]Field, 0, len(fields)+3)
	allFields = append(allFields, F("actor", actor), F("action", action), F("resource", resource))
	allFields = append(allFields, fields...)
	l.logEntry(LevelInfo, "audit", allFields...)
}
