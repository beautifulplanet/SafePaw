package middleware

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScanOutput_ScriptTag(t *testing.T) {
	risk, triggers := ScanOutput(`Here is some code: <script>alert("xss")</script>`)
	if risk != OutputRiskHigh {
		t.Errorf("risk = %s, want high", risk)
	}
	if !containsStr(triggers, "script_tag") {
		t.Errorf("triggers = %v, expected script_tag", triggers)
	}
}

func TestScanOutput_IframeTag(t *testing.T) {
	risk, triggers := ScanOutput(`<iframe src="https://evil.com"></iframe>`)
	if risk != OutputRiskHigh {
		t.Errorf("risk = %s, want high", risk)
	}
	if !containsStr(triggers, "iframe_tag") {
		t.Errorf("triggers = %v, expected iframe_tag", triggers)
	}
}

func TestScanOutput_EventHandler(t *testing.T) {
	risk, _ := ScanOutput(`<div onclick="steal()">click</div>`)
	if risk < OutputRiskMedium {
		t.Errorf("risk = %s, want >= medium", risk)
	}
}

func TestScanOutput_JavascriptURI(t *testing.T) {
	risk, _ := ScanOutput(`<a href="javascript:void(0)">link</a>`)
	if risk < OutputRiskMedium {
		t.Errorf("risk = %s, want >= medium", risk)
	}
}

func TestScanOutput_APIKeyLeak(t *testing.T) {
	// Build at runtime so OPSEC hook does not flag a literal sk-* string
	risk, triggers := ScanOutput("Your API key is sk-" + strings.Repeat("x", 24))
	if risk != OutputRiskHigh {
		t.Errorf("risk = %s, want high", risk)
	}
	if !containsStr(triggers, "api_key_leak") {
		t.Errorf("triggers = %v, expected api_key_leak", triggers)
	}
}

func TestScanOutput_SystemPromptLeak(t *testing.T) {
	risk, triggers := ScanOutput(`Sure! My system prompt: "You are a helpful assistant..."`)
	if risk != OutputRiskHigh {
		t.Errorf("risk = %s, want high", risk)
	}
	if !containsStr(triggers, "system_prompt_leak") {
		t.Errorf("triggers = %v, expected system_prompt_leak", triggers)
	}
}

func TestScanOutput_SafeContent(t *testing.T) {
	safe := []string{
		"Hello, how can I help you today?",
		`{"response": "The weather is sunny."}`,
		"Here is a Python example:\n```python\nprint('hello')\n```",
		"",
	}
	for _, s := range safe {
		risk, triggers := ScanOutput(s)
		if risk != OutputRiskNone {
			t.Errorf("ScanOutput(%q) = risk=%s triggers=%v, want none", s, risk, triggers)
		}
	}
}

func TestSanitizeOutput(t *testing.T) {
	input := `Hello <script>alert(1)</script> and <iframe src=x> world`
	result := SanitizeOutput(input)
	if strings.Contains(result, "<script>") {
		t.Errorf("script tag not removed: %q", result)
	}
	if strings.Contains(result, "<iframe") {
		t.Errorf("iframe tag not removed: %q", result)
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "world") {
		t.Errorf("safe content lost: %q", result)
	}
}

func TestOutputScanner_HTTPMiddleware(t *testing.T) {
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response":"<script>alert(1)</script>"}`))
	})

	handler := OutputScanner(1024*1024, backend)
	req := httptest.NewRequest("GET", "/chat", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Errorf("script tag should be sanitized in response: %q", body)
	}
	if rec.Header().Get("X-SafePaw-Output-Risk") != "high" {
		t.Errorf("output risk header = %q, want high", rec.Header().Get("X-SafePaw-Output-Risk"))
	}
	// Content-Length must match actual body after sanitization
	cl := rec.Header().Get("Content-Length")
	if cl == "" {
		t.Fatal("Content-Length header missing after sanitization")
	}
	if cl != fmt.Sprintf("%d", len(body)) {
		t.Errorf("Content-Length = %s, want %d (actual body length)", cl, len(body))
	}
}

func TestOutputScanner_SafePassthrough(t *testing.T) {
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response":"perfectly safe content"}`))
	})

	handler := OutputScanner(1024*1024, backend)
	req := httptest.NewRequest("GET", "/chat", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if body != `{"response":"perfectly safe content"}` {
		t.Errorf("safe content should pass unchanged, got: %q", body)
	}
	if rec.Header().Get("X-SafePaw-Output-Risk") != "none" {
		t.Errorf("output risk = %q, want none", rec.Header().Get("X-SafePaw-Output-Risk"))
	}
}

func TestOutputScanner_BinaryPassthrough(t *testing.T) {
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x00, 0x01, 0x02, 0x03})
	})

	handler := OutputScanner(1024*1024, backend)
	req := httptest.NewRequest("GET", "/binary", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Body.Len() != 4 {
		t.Errorf("binary content should pass through, got %d bytes", rec.Body.Len())
	}
}

func TestScanningReader(t *testing.T) {
	input := `backend says: <script>evil()</script> and more text`
	sr := NewScanningReader(strings.NewReader(input), "test-id", "/ws")

	out, err := io.ReadAll(sr)
	if err != nil {
		t.Fatal(err)
	}
	result := string(out)
	if strings.Contains(result, "<script>") {
		t.Errorf("script tag should be sanitized in stream: %q", result)
	}
}

func TestScanningReader_SafeContent(t *testing.T) {
	input := "Hello, this is a safe response from the AI."
	sr := NewScanningReader(bytes.NewReader([]byte(input)), "test-id", "/ws")

	out, err := io.ReadAll(sr)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != input {
		t.Errorf("safe content should pass unchanged: got %q", string(out))
	}
}

func TestOutputRisk_String(t *testing.T) {
	if OutputRiskNone.String() != "none" {
		t.Error("none")
	}
	if OutputRiskLow.String() != "low" {
		t.Error("low")
	}
	if OutputRiskMedium.String() != "medium" {
		t.Error("medium")
	}
	if OutputRiskHigh.String() != "high" {
		t.Error("high")
	}
	if OutputRisk(99).String() != "unknown" {
		t.Error("unknown case should return 'unknown'")
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// === Encoding evasion tests (Gap 2 fix) ===

func TestScanOutput_Base64EncodedScript(t *testing.T) {
	// Encode a script tag as base64 — scanner should decode and detect it
	payload := `<script>alert("xss")</script>`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	content := "Here is some data: " + encoded

	risk, triggers := ScanOutput(content)
	if risk < OutputRiskHigh {
		t.Errorf("base64-encoded script tag not detected: risk=%s triggers=%v", risk, triggers)
	}
}

func TestScanOutput_Base64EncodedAPIKey(t *testing.T) {
	payload := "sk-" + strings.Repeat("a", 24)
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	content := "Result: " + encoded

	risk, triggers := ScanOutput(content)
	if risk < OutputRiskHigh {
		t.Errorf("base64-encoded API key not detected: risk=%s triggers=%v", risk, triggers)
	}
}

func TestScanOutput_FullwidthUnicodeScript(t *testing.T) {
	// Fullwidth characters: ＜ｓｃｒｉｐｔ＞
	content := "\uff1c\uff53\uff43\uff52\uff49\uff50\uff54\uff1e"
	risk, triggers := ScanOutput(content)
	if risk < OutputRiskHigh {
		t.Errorf("fullwidth unicode script tag not detected: risk=%s triggers=%v", risk, triggers)
	}
}

func TestScanOutput_FullwidthJavascriptURI(t *testing.T) {
	// ｊａｖａｓｃｒｉｐｔ：
	content := "\uff4a\uff41\uff56\uff41\uff53\uff43\uff52\uff49\uff50\uff54\uff1a"
	risk, triggers := ScanOutput(content)
	if risk < OutputRiskMedium {
		t.Errorf("fullwidth javascript: URI not detected: risk=%s triggers=%v", risk, triggers)
	}
}

func TestNormalizeUnicode(t *testing.T) {
	// Fullwidth A = \uff21 should become A (0x41)
	result := normalizeUnicode("\uff21\uff22\uff23")
	if result != "ABC" {
		t.Errorf("normalizeUnicode = %q, want ABC", result)
	}
}

func TestNormalizeForScan_PlainText(t *testing.T) {
	// Plain text should come back unchanged
	input := "Hello, this is safe text with no encoding."
	result := normalizeForScan(input)
	if result != input {
		t.Errorf("normalizeForScan changed plain text: %q", result)
	}
}

func TestOutputScanner_LargeResponsePassthrough(t *testing.T) {
	// Response exceeds maxScanSize → should passthrough without scanning
	bigBody := strings.Repeat("A", 200)
	handler := OutputScanner(100, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(bigBody))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != bigBody {
		t.Errorf("expected full body passthrough, got len=%d", rr.Body.Len())
	}
}

func TestOutputScanner_MultiWriteExceedsMax(t *testing.T) {
	// Multiple small writes that together exceed maxScanSize
	handler := OutputScanner(50, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 0; i < 5; i++ {
			w.Write([]byte(strings.Repeat("B", 20)))
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.Len() != 100 {
		t.Errorf("expected 100 bytes, got %d", rr.Body.Len())
	}
}

func TestNormalizeForScan_Base64NoPadding(t *testing.T) {
	// Encode "<script>alert(1)</script>" in base64 without padding (RawStdEncoding)
	payload := base64.RawStdEncoding.EncodeToString([]byte(`<script>alert(1)</script>`))
	result := normalizeForScan(payload)
	if !strings.Contains(result, "<script>") {
		t.Errorf("expected decoded script tag in result, got %q", result)
	}
}

func TestScanningReader_MaliciousContent(t *testing.T) {
	malicious := `<script>alert("xss")</script>`
	reader := NewScanningReader(strings.NewReader(malicious), "req1", "/chat")
	buf := make([]byte, 1024)
	n, _ := reader.Read(buf)
	// Content should be sanitized (script tags removed)
	result := string(buf[:n])
	if strings.Contains(result, "<script>") {
		t.Errorf("expected script tags to be sanitized, got %q", result)
	}
}
