package middleware

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
)

func TestInferLevel_KnownComponents(t *testing.T) {
	tests := []struct {
		component string
		msg       string
		want      LogLevel
	}{
		{"AUTH", "Authenticated sub=user1", LevelInfo},
		{"AUTH", "Rejected: bad token", LevelWarn},
		{"SCANNER", "Risk detected", LevelWarn},
		{"SECURITY", "DENIED: bad origin", LevelWarn},
		{"PROXY", "Forwarding request", LevelInfo},
		{"PROXY", "Backend error: connection failed", LevelWarn},
		{"FATAL", "Server crashed", LevelError},
		{"UNKNOWN", "Some message", LevelInfo},
		{"UNKNOWN", "Something failed", LevelWarn},
	}

	for _, tt := range tests {
		got := inferLevel(tt.component, tt.msg)
		if got != tt.want {
			t.Errorf("inferLevel(%q, %q) = %v, want %v", tt.component, tt.msg, got, tt.want)
		}
	}
}

func TestExtractKV(t *testing.T) {
	msg := `Rejected: bad token (remote=10.0.0.1 request_id=abc-123 scope="admin")`
	kv := extractKV(msg)

	if kv["remote"] != "10.0.0.1" {
		t.Errorf("expected remote=10.0.0.1, got %q", kv["remote"])
	}
	if kv["request_id"] != "abc-123" {
		t.Errorf("expected request_id=abc-123, got %q", kv["request_id"])
	}
	if kv["scope"] != "admin" {
		t.Errorf("expected scope=admin, got %q", kv["scope"])
	}
}

func TestExtractKV_NoFields(t *testing.T) {
	kv := extractKV("Simple message with no fields")
	if kv != nil {
		t.Errorf("expected nil, got %v", kv)
	}
}

func TestCleanMsg(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Rejected: bad token (remote=10.0.0.1)", "Rejected: bad token"},
		{"No parens here", "No parens here"},
		{"", ""},
	}
	for _, tt := range tests {
		got := cleanMsg(tt.input)
		if got != tt.want {
			t.Errorf("cleanMsg(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestJSONLogWriter_ParsesPrefix(t *testing.T) {
	var buf bytes.Buffer
	w := &jsonLogWriter{out: &buf}

	w.Write([]byte("[AUTH] Rejected: bad token (remote=10.0.0.1 request_id=abc)\n"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, buf.String())
	}

	if entry["component"] != "AUTH" {
		t.Errorf("expected component=AUTH, got %v", entry["component"])
	}
	if entry["level"] != "warn" {
		t.Errorf("expected level=warn (contains 'Rejected'), got %v", entry["level"])
	}
	if entry["service"] != "safepaw-gateway" {
		t.Errorf("expected service=safepaw-gateway, got %v", entry["service"])
	}
	if _, ok := entry["ts"]; !ok {
		t.Error("expected ts field")
	}

	fields := entry["fields"].(map[string]interface{})
	if fields["remote"] != "10.0.0.1" {
		t.Errorf("expected fields.remote=10.0.0.1, got %v", fields["remote"])
	}
}

func TestJSONLogWriter_NoPrefix(t *testing.T) {
	var buf bytes.Buffer
	w := &jsonLogWriter{out: &buf}

	w.Write([]byte("Simple startup message\n"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if _, ok := entry["component"]; ok {
		t.Error("expected no component for unprefixed messages")
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info, got %v", entry["level"])
	}
}

func TestJSONLogWriter_EmptyLine(t *testing.T) {
	var buf bytes.Buffer
	w := &jsonLogWriter{out: &buf}

	w.Write([]byte("   \n"))

	if buf.Len() != 0 {
		t.Errorf("expected no output for empty line, got %q", buf.String())
	}
}

func TestLogger_JSONMode(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: true, component: "TEST"}

	l.Info("request processed", F("method", "GET"), F("status", 200))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, buf.String())
	}

	if entry["component"] != "TEST" {
		t.Errorf("expected component=TEST, got %v", entry["component"])
	}
	if entry["msg"] != "request processed" {
		t.Errorf("expected msg='request processed', got %v", entry["msg"])
	}
	if entry["method"] != "GET" {
		t.Errorf("expected method=GET, got %v", entry["method"])
	}
	if entry["status"] != float64(200) {
		t.Errorf("expected status=200, got %v", entry["status"])
	}
}

func TestLogger_TextMode(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: false, component: "PROXY"}

	l.Warn("backend unreachable", F("target", "localhost:3000"))

	output := buf.String()
	if !strings.Contains(output, "[PROXY]") {
		t.Errorf("expected [PROXY] prefix, got %q", output)
	}
	if !strings.Contains(output, "backend unreachable") {
		t.Errorf("expected message in output, got %q", output)
	}
	if !strings.Contains(output, "target=localhost:3000") {
		t.Errorf("expected field in output, got %q", output)
	}
}

func TestLogger_MinLevel(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: false, minLevel: LevelWarn}

	l.Info("should be filtered")
	l.Debug("should be filtered")
	l.Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should be filtered") {
		t.Errorf("expected filtered messages to not appear, got %q", output)
	}
	if !strings.Contains(output, "should appear") {
		t.Errorf("expected warn message to appear, got %q", output)
	}
}

func TestLogger_SecurityEvent(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: true, component: "AUTH"}

	l.SecurityEvent("brute_force", "blocked", F("ip", "10.0.0.1"), F("attempts", 50))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["msg"] != "security_event" {
		t.Errorf("expected msg=security_event, got %v", entry["msg"])
	}
	if entry["event_type"] != "brute_force" {
		t.Errorf("expected event_type=brute_force, got %v", entry["event_type"])
	}
	if entry["action"] != "blocked" {
		t.Errorf("expected action=blocked, got %v", entry["action"])
	}
	if entry["ip"] != "10.0.0.1" {
		t.Errorf("expected ip=10.0.0.1, got %v", entry["ip"])
	}
}

func TestLogger_AuditEvent(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: true}

	l.AuditEvent("admin", "revoke_token", "user:alice", F("reason", "compromised"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["msg"] != "audit" {
		t.Errorf("expected msg=audit, got %v", entry["msg"])
	}
	if entry["actor"] != "admin" {
		t.Errorf("expected actor=admin, got %v", entry["actor"])
	}
	if entry["action"] != "revoke_token" {
		t.Errorf("expected action=revoke_token, got %v", entry["action"])
	}
	if entry["resource"] != "user:alice" {
		t.Errorf("expected resource=user:alice, got %v", entry["resource"])
	}
}

func TestLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	base := &Logger{out: &buf, jsonMode: true, component: "BASE"}
	child := base.WithComponent("CHILD")

	child.Info("hello")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["component"] != "CHILD" {
		t.Errorf("expected component=CHILD, got %v", entry["component"])
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{LogLevel(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("LogLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestInstallJSONLogger_Disabled(t *testing.T) {
	// When LOG_FORMAT is not "json", InstallJSONLogger should return false
	t.Setenv("LOG_FORMAT", "text")
	if InstallJSONLogger() {
		t.Error("expected false when LOG_FORMAT != json")
	}
}

func TestInstallJSONLogger_Enabled(t *testing.T) {
	// Save and restore global log state
	origFlags := log.Flags()
	origOutput := log.Writer()
	defer func() {
		log.SetFlags(origFlags)
		log.SetOutput(origOutput)
	}()

	t.Setenv("LOG_FORMAT", "json")
	if !InstallJSONLogger() {
		t.Error("expected true when LOG_FORMAT=json")
	}
}

func TestLogger_ErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: true, component: "TEST"}

	l.Error("something broke", F("code", 500))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if entry["level"] != "error" {
		t.Errorf("expected level=error, got %v", entry["level"])
	}
	if entry["msg"] != "something broke" {
		t.Errorf("expected msg='something broke', got %v", entry["msg"])
	}
}

func TestLogger_DebugLevel(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: false, minLevel: LevelDebug}

	l.Debug("trace info", F("detail", "abc"))

	output := buf.String()
	if !strings.Contains(output, "trace info") {
		t.Errorf("expected debug message in output, got %q", output)
	}
}

func TestLogger_TextMode_NoComponent(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{out: &buf, jsonMode: false}

	l.Info("startup complete")

	output := buf.String()
	if strings.Contains(output, "[") {
		t.Errorf("expected no component prefix, got %q", output)
	}
	if !strings.Contains(output, "startup complete") {
		t.Errorf("expected message in output, got %q", output)
	}
}
