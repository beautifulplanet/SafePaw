package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestAudit_TextMode(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, false)

	l.LoginSuccess("10.0.0.1")

	output := buf.String()
	if !strings.Contains(output, "[AUDIT]") {
		t.Errorf("expected [AUDIT] prefix, got %q", output)
	}
	if !strings.Contains(output, "action=login") {
		t.Errorf("expected action=login, got %q", output)
	}
	if !strings.Contains(output, "actor=10.0.0.1") {
		t.Errorf("expected actor=10.0.0.1, got %q", output)
	}
	if !strings.Contains(output, "outcome=success") {
		t.Errorf("expected outcome=success, got %q", output)
	}
}

func TestAudit_JSONMode(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, true)

	l.LoginFailure("192.168.1.1", "bad_password")

	var ev Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, buf.String())
	}

	if ev.Type != "audit" {
		t.Errorf("expected type=audit, got %q", ev.Type)
	}
	if ev.Action != "login" {
		t.Errorf("expected action=login, got %q", ev.Action)
	}
	if ev.Actor != "192.168.1.1" {
		t.Errorf("expected actor=192.168.1.1, got %q", ev.Actor)
	}
	if ev.Outcome != "failure" {
		t.Errorf("expected outcome=failure, got %q", ev.Outcome)
	}
	if ev.Details["reason"] != "bad_password" {
		t.Errorf("expected reason=bad_password, got %q", ev.Details["reason"])
	}
}

func TestAudit_ConfigChange(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, true)

	l.ConfigChange("10.0.0.1", []string{"AUTH_ENABLED", "AUTH_SECRET"})

	var ev Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if ev.Action != "config_update" {
		t.Errorf("expected action=config_update, got %q", ev.Action)
	}
	if ev.Resource != "env" {
		t.Errorf("expected resource=env, got %q", ev.Resource)
	}
	if ev.Details["keys_changed"] != "AUTH_ENABLED,AUTH_SECRET" {
		t.Errorf("expected keys_changed, got %q", ev.Details["keys_changed"])
	}
}

func TestAudit_ServiceRestart(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, true)

	l.ServiceRestart("10.0.0.1", "gateway", "success")

	var ev Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if ev.Action != "service_restart" {
		t.Errorf("expected action=service_restart, got %q", ev.Action)
	}
	if ev.Resource != "gateway" {
		t.Errorf("expected resource=gateway, got %q", ev.Resource)
	}
}

func TestAudit_TimestampIsUTC(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, true)

	l.LoginSuccess("10.0.0.1")

	var ev Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if ev.Timestamp.Location().String() != "UTC" {
		t.Errorf("expected UTC timestamp, got %v", ev.Timestamp.Location())
	}
}
