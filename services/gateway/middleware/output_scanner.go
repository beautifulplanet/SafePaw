// =============================================================
// SafePaw Gateway - Output Validation (Defense in Depth)
// =============================================================
// Scans responses FROM the backend (OpenClaw) before they reach
// the client. This catches:
//
//   1. XSS payloads that an LLM was tricked into generating
//   2. Data exfiltration payloads (URLs to external servers)
//   3. Leaked system prompts or API keys
//
// WHY scan outputs?
//   If prompt injection succeeds (bypasses the input scanner),
//   the LLM may produce dangerous output. Output scanning is
//   the last line of defense before malicious content reaches
//   the client's browser.
//
// OPSEC Lesson #15: "Sanitize at the gate, validate at the brain,
// AND scan the exit." Three chances to catch an attack.
// =============================================================

package middleware

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OutputRisk indicates the assessed risk level of a response.
type OutputRisk int

const (
	OutputRiskNone   OutputRisk = 0
	OutputRiskLow    OutputRisk = 1
	OutputRiskMedium OutputRisk = 2
	OutputRiskHigh   OutputRisk = 3
)

func (r OutputRisk) String() string {
	switch r {
	case OutputRiskNone:
		return "none"
	case OutputRiskLow:
		return "low"
	case OutputRiskMedium:
		return "medium"
	case OutputRiskHigh:
		return "high"
	default:
		return "unknown"
	}
}

var outputPatterns = []struct {
	pattern *regexp.Regexp
	risk    OutputRisk
	name    string
}{
	// XSS payloads the LLM might generate
	{regexp.MustCompile(`(?i)<\s*script\b[^>]*>`), OutputRiskHigh, "script_tag"},
	{regexp.MustCompile(`(?i)<\s*iframe\b[^>]*>`), OutputRiskHigh, "iframe_tag"},
	// Event handlers — require HTML tag context (< ... on*=")
	{regexp.MustCompile(`(?i)<[^>]+\bon\w+\s*=\s*["']`), OutputRiskMedium, "event_handler"},
	{regexp.MustCompile(`(?i)javascript\s*:`), OutputRiskMedium, "javascript_uri"},

	// System prompt / secret leakage — require verbatim leak indicators:
	// "my system prompt is:", "here are my internal instructions:"
	// but NOT general discussion like "What is a system prompt?"
	{regexp.MustCompile(`(?i)(my|the|here\s+are|following)\s+(system\s*prompt|internal\s*instructions?)\s*(is|are|:)\s*[:=]?`), OutputRiskHigh, "system_prompt_leak"},
	{regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16})`), OutputRiskHigh, "api_key_leak"},

	// Data exfiltration (LLM told to embed URLs to external servers)
	{regexp.MustCompile(`(?i)<\s*img\b[^>]+src\s*=\s*["']https?://[^"']+["']`), OutputRiskMedium, "external_image"},
}

// ScanOutput assesses the risk level of response content.
// Performance: scans raw content first. Only if the raw scan is clean
// does it pay the cost of normalizing (base64 decode + unicode) as a
// second-chance pass to catch encoding-based evasion.
func ScanOutput(content string) (OutputRisk, []string) {
	maxRisk := OutputRiskNone
	var triggered []string

	// Fast path: scan the original content
	for _, p := range outputPatterns {
		if p.pattern.MatchString(content) {
			if p.risk > maxRisk {
				maxRisk = p.risk
			}
			triggered = append(triggered, p.name)
		}
	}

	// If raw scan already found something, skip expensive normalization
	if maxRisk > OutputRiskNone {
		return maxRisk, triggered
	}

	// Slow path: decode/normalize, then re-scan for encoded evasion
	normalized := normalizeForScan(content)
	if normalized != content {
		for _, p := range outputPatterns {
			if p.pattern.MatchString(normalized) {
				if p.risk > maxRisk {
					maxRisk = p.risk
				}
				triggered = append(triggered, p.name+"_encoded")
			}
		}
	}

	return maxRisk, triggered
}

// normalizeForScan decodes base64 segments (up to 2 rounds for nested
// encoding) and normalizes unicode confusables so regex patterns can
// catch encoding-based evasion attempts.
//
// Residual risk: visually-confusable Unicode characters (e.g. ꜱ U+A731
// for 's', or Cyrillic а U+0430 for Latin 'a') are NOT normalized here.
// A full confusable mapping would require ICU/unicode confusable tables
// and risks false positives on legitimate multilingual content.
func normalizeForScan(s string) string {
	// 1. Attempt base64 decode on segments that look like base64.
	//    Run up to 2 rounds to catch nested encoding (e.g. base64(base64(payload))).
	normalized := s
	for round := 0; round < 2; round++ {
		prev := normalized
		normalized = b64Pattern.ReplaceAllStringFunc(normalized, func(match string) string {
			decoded, err := base64.StdEncoding.DecodeString(match)
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(match)
			}
			if err == nil && utf8.Valid(decoded) {
				return string(decoded)
			}
			return match
		})
		if normalized == prev {
			break // nothing changed, stop early
		}
	}

	// 2. Normalize unicode confusables (fullwidth chars → ASCII)
	normalized = normalizeUnicode(normalized)

	return normalized
}

// b64Pattern matches potential base64-encoded segments (20+ chars, valid alphabet).
var b64Pattern = regexp.MustCompile(`[A-Za-z0-9+/]{20,}={0,2}`)

// normalizeUnicode replaces fullwidth and other unicode confusable
// characters with their ASCII equivalents.
func normalizeUnicode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			// Fullwidth ASCII variants (！to～) → normal ASCII
			b.WriteRune(r - 0xFEE0)
		} else if unicode.Is(unicode.Zs, r) {
			// All unicode space chars → ASCII space
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeOutput strips dangerous patterns from output content.
func SanitizeOutput(content string) string {
	s := content
	s = dangerousHTMLPattern.ReplaceAllString(s, "[filtered]")
	s = eventHandlerPattern.ReplaceAllString(s, "[filtered]=")
	s = dangerousURIPattern.ReplaceAllString(s, "[filtered]:")
	return s
}

// OutputScanner is middleware that scans HTTP response bodies from the
// backend for dangerous content. It buffers JSON/text responses, scans
// them, and either passes them through (with risk headers) or sanitizes
// dangerous patterns.
//
// Binary responses, streaming responses, and large bodies are passed
// through unscanned for performance.
func OutputScanner(maxScanSize int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only scan responses to GET/POST for text/json content
		cw := &captureWriter{
			ResponseWriter: w,
			maxScan:        maxScanSize,
			buf:            &bytes.Buffer{},
		}
		next.ServeHTTP(cw, r)

		if !cw.shouldScan() {
			return
		}

		body := cw.buf.String()
		risk, triggers := ScanOutput(body)

		if risk > OutputRiskNone {
			log.Printf("[OUTPUT-SCAN] risk=%s triggers=%v path=%s body_len=%d request_id=%s",
				risk, triggers, r.URL.Path, len(body), r.Header.Get("X-Request-ID"))
		}

		if risk >= OutputRiskHigh {
			body = SanitizeOutput(body)
			log.Printf("[OUTPUT-SCAN] Sanitized high-risk output for path=%s request_id=%s",
				r.URL.Path, r.Header.Get("X-Request-ID"))
		}

		if sc := GetSecurityContext(r); sc != nil {
			sc.OutputScan = &ScanDecision{
				Risk:      risk.String(),
				Triggers:  triggers,
				Sanitized: risk >= OutputRiskHigh,
			}
		}

		w.Header().Set("X-SafePaw-Output-Risk", risk.String())
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if !cw.headerWritten {
			w.WriteHeader(cw.statusCode)
		}
		w.Write([]byte(body)) // #nosec G104 -- best-effort scanned response write
	})
}

type captureWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
	buf           *bytes.Buffer
	maxScan       int64
	passthrough   bool // true if we gave up scanning
	contentType   string
}

func (cw *captureWriter) WriteHeader(code int) {
	cw.statusCode = code
	cw.contentType = cw.Header().Get("Content-Type")

	if !cw.isScannable() {
		cw.passthrough = true
		cw.headerWritten = true
		cw.ResponseWriter.WriteHeader(code)
	}
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	if cw.statusCode == 0 {
		cw.WriteHeader(http.StatusOK)
	}

	if cw.passthrough {
		return cw.ResponseWriter.Write(b)
	}

	if int64(cw.buf.Len())+int64(len(b)) > cw.maxScan {
		// Too large to scan — flush buffer and switch to passthrough
		cw.passthrough = true
		if !cw.headerWritten {
			cw.headerWritten = true
			cw.ResponseWriter.WriteHeader(cw.statusCode)
		}
		if cw.buf.Len() > 0 {
			cw.ResponseWriter.Write(cw.buf.Bytes()) // #nosec G104 -- flushing buffered data on passthrough
			cw.buf.Reset()
		}
		return cw.ResponseWriter.Write(b)
	}

	return cw.buf.Write(b)
}

func (cw *captureWriter) isScannable() bool {
	ct := strings.ToLower(cw.contentType)
	// Skip text/html — those are the backend's own UI pages (static assets),
	// not LLM-generated content. Scanning them destroys legitimate <script>,
	// <link>, <meta>, and <style> tags that the UI needs to function.
	if strings.Contains(ct, "html") {
		return false
	}
	return strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/")
}

func (cw *captureWriter) shouldScan() bool {
	return !cw.passthrough && cw.buf.Len() > 0
}

// ScanningReader wraps an io.Reader and scans chunks of data
// for dangerous output patterns. Used for WebSocket stream scanning.
type ScanningReader struct {
	source io.Reader
	reqID  string
	path   string
}

// NewScanningReader creates a reader that scans data as it flows through.
func NewScanningReader(source io.Reader, reqID, path string) *ScanningReader {
	return &ScanningReader{source: source, reqID: reqID, path: path}
}

// Read scans WebSocket stream data for dangerous patterns.
// IMPORTANT: This is LOG-ONLY for WebSocket streams. Unlike HTTP
// responses, WebSocket data is framed binary — modifying the payload
// length without updating frame headers corrupts the stream and causes
// black screens in the client. We log the risk but pass data through
// unmodified. The HTTP OutputScanner middleware handles sanitization
// for non-WebSocket responses.
func (sr *ScanningReader) Read(p []byte) (int, error) {
	n, err := sr.source.Read(p)
	if n > 0 {
		chunk := string(p[:n])
		risk, triggers := ScanOutput(chunk)
		if risk > OutputRiskNone {
			log.Printf("[OUTPUT-SCAN:WS] risk=%s triggers=%v path=%s chunk_len=%d request_id=%s",
				risk, triggers, SanitizeLogValue(sr.path), n, sr.reqID)
		}
		// DO NOT sanitize WebSocket frames — modifying payload length
		// without updating frame headers corrupts the binary stream.
		// Risk is logged above for audit; the receipt ledger tracks details.
	}
	return n, err
}
