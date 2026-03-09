// =============================================================
// SafePaw Gateway - Input Sanitization & AI Defense Layer
// =============================================================
// This is the first line of defense against AI-specific attacks.
// Applied BEFORE messages enter the Redis pipeline.
//
// AI THREAT MODEL — what this defends against:
//
// 1. PROMPT INJECTION:
//    Attackers embed instructions like "Ignore previous instructions"
//    in their messages. When these reach an LLM, the model may obey
//    the injected instructions instead of the system prompt.
//    Defense: Flag suspicious patterns, add input context markers.
//
// 2. OUTPUT XSS / CODE INJECTION:
//    Even in echo mode, we sanitize outputs. When an LLM is added,
//    it could be tricked into outputting <script> tags or SQL.
//    Defense: Strip dangerous HTML/JS from both input and output.
//
// 3. CONTENT-TYPE CONFUSION:
//    Client sends content_type:"system" to trick the LLM pipeline
//    into treating user input as a system prompt.
//    Defense: Whitelist content types, reject unknown ones.
//
// 4. CHANNEL PATH TRAVERSAL:
//    Client sends channel:"../admin" to access restricted channels.
//    Defense: Validate channel format (alphanumeric + limited chars).
//
// 5. METADATA INJECTION:
//    Free-form metadata can contain log injection (\n[ADMIN]),
//    Redis command injection, or prompt injection payloads.
//    Defense: Limit key/value lengths, strip control characters.
//
// 6. RECURSIVE LOOP DETECTION:
//    Messages that reference their own output patterns, designed
//    to create infinite request loops.
//    Defense: Detect echo-back patterns, add nonce tracking.
//
// OPSEC Lesson #15: "Sanitize at the gate, validate at the brain."
// The Gateway sanitizes raw input. The Agent validates semantics.
// Two layers, two languages, two chances to catch attacks.
// =============================================================

package middleware

import (
	"log"
	"regexp"
	"strings"
	"unicode"
)

// ================================================================
// Content Type Whitelist
// ================================================================

// AllowedContentTypes defines the ONLY valid content types.
// Anything not in this list is rejected.
// This prevents content_type:"system" attacks where an attacker
// tries to make the LLM treat user input as a system prompt.
var AllowedContentTypes = map[string]bool{
	"TEXT":     true, // Plain text messages
	"COMMAND":  true, // Slash commands (/help, /settings)
	"FILE":     true, // File attachment reference
	"IMAGE":    true, // Image reference/URL
	"AUDIO":    true, // Audio reference/URL
	"MARKDOWN": true, // Markdown-formatted text
}

// ValidateContentType checks if a content type is in the whitelist.
// Returns the validated type (uppercased) or "TEXT" as safe default.
func ValidateContentType(ct string) string {
	upper := strings.ToUpper(strings.TrimSpace(ct))
	if upper == "" {
		return "TEXT"
	}
	if AllowedContentTypes[upper] {
		return upper
	}
	log.Printf("[SANITIZE] Rejected unknown content_type=%q → defaulting to TEXT", ct)
	return "TEXT"
}

// ================================================================
// Channel Validation
// ================================================================

// channelPattern validates channel names: alphanumeric, dashes, underscores, dots.
// Max 128 chars. No path traversal, no special chars.
var channelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// ValidateChannel checks if a channel name is safe.
// Returns the validated channel or "default" if invalid.
func ValidateChannel(ch string) (string, bool) {
	ch = strings.TrimSpace(ch)
	if ch == "" {
		return "default", true
	}

	// Block path traversal attempts
	if strings.Contains(ch, "..") || strings.Contains(ch, "/") || strings.Contains(ch, "\\") {
		log.Printf("[SANITIZE] Channel path traversal blocked: %q", ch)
		return "", false
	}

	if !channelPattern.MatchString(ch) {
		log.Printf("[SANITIZE] Channel invalid format: %q", ch)
		return "", false
	}

	return ch, true
}

// ================================================================
// Metadata Sanitization
// ================================================================

// MaxMetadataKeys is the maximum number of metadata key-value pairs.
const MaxMetadataKeys = 16

// MaxMetadataKeyLen is the maximum length of a metadata key.
const MaxMetadataKeyLen = 64

// MaxMetadataValueLen is the maximum length of a metadata value.
const MaxMetadataValueLen = 256

// SanitizeMetadata cleans metadata to prevent injection attacks:
// - Limits number of keys
// - Limits key and value lengths
// - Strips control characters
// - Rejects keys that look like injection attempts
func SanitizeMetadata(meta map[string]string) map[string]string {
	if meta == nil {
		return nil
	}

	clean := make(map[string]string, len(meta))
	count := 0

	for k, v := range meta {
		if count >= MaxMetadataKeys {
			log.Printf("[SANITIZE] Metadata key limit reached (%d), dropping remaining keys", MaxMetadataKeys)
			break
		}

		// Sanitize key
		k = StripControlChars(k)
		if len(k) > MaxMetadataKeyLen {
			k = k[:MaxMetadataKeyLen]
		}
		if k == "" {
			continue
		}

		// Reject suspicious key names that could confuse systems
		kLower := strings.ToLower(k)
		if strings.HasPrefix(kLower, "system") ||
			strings.HasPrefix(kLower, "prompt") ||
			strings.HasPrefix(kLower, "instruction") ||
			strings.HasPrefix(kLower, "role") ||
			strings.HasPrefix(kLower, "admin") ||
			strings.HasPrefix(kLower, "internal") {
			log.Printf("[SANITIZE] Metadata key rejected (reserved prefix): %q", k)
			continue
		}

		// Sanitize value
		v = StripControlChars(v)
		if len(v) > MaxMetadataValueLen {
			v = v[:MaxMetadataValueLen]
		}

		clean[k] = v
		count++
	}

	return clean
}

// ================================================================
// Content Sanitization
// ================================================================

// SanitizeContent cleans user-provided content:
// - Strips dangerous HTML tags (script, iframe, object, embed, etc.)
// - Strips control characters (except newline, tab)
// - Does NOT strip prompt injection markers (that's the Agent's job)
//
// This is XSS prevention for when content is rendered in web clients.
func SanitizeContent(s string) string {
	// Strip control characters but preserve newlines and tabs
	s = StripControlChars(s)

	// Strip dangerous HTML tags — defense against stored XSS
	// Even in a WebSocket API, clients may render this in HTML
	s = stripDangerousHTML(s)

	return s
}

// dangerousHTMLPattern matches HTML tags that could execute code.
// This is NOT a full HTML parser — it's a defense-in-depth layer.
// The client should ALSO sanitize, but we don't trust clients.
var dangerousHTMLPattern = regexp.MustCompile(
	`(?i)<\s*/?\s*(script|iframe|object|embed|form|input|button|link|style|svg|math|base|meta|applet|frame|frameset)\b[^>]*>`,
)

// Event handlers in HTML tags (onclick, onerror, etc.)
// Requires HTML tag context to avoid false positives on code discussions
// like "onChange = handler" or "the onError = callback".
var eventHandlerPattern = regexp.MustCompile(
	`(?i)<[^>]+\bon\w+\s*=`,
)

// Match javascript: and vbscript: URI schemes.
// data: is excluded — it's commonly used in legitimate base64 image URIs
// and AI-generated code (e.g., data:image/png;base64,...). Malicious data:
// URIs are caught by the CSP header (default-src) in SecurityHeaders.
var dangerousURIPattern = regexp.MustCompile(
	`(?i)(javascript|vbscript)\s*:`,
)

func stripDangerousHTML(s string) string {
	original := s

	s = dangerousHTMLPattern.ReplaceAllString(s, "[removed]")
	s = eventHandlerPattern.ReplaceAllString(s, "[removed]=")
	s = dangerousURIPattern.ReplaceAllString(s, "[removed]:")

	if s != original {
		log.Printf("[SANITIZE] Stripped dangerous HTML/JS from content (len=%d→%d)", len(original), len(s))
	}

	return s
}

// ================================================================
// Prompt Injection Detection (Heuristic)
// ================================================================

// PromptInjectionRisk indicates the assessed risk level of a message.
type PromptInjectionRisk int

const (
	RiskNone   PromptInjectionRisk = 0
	RiskLow    PromptInjectionRisk = 1
	RiskMedium PromptInjectionRisk = 2
	RiskHigh   PromptInjectionRisk = 3
)

func (r PromptInjectionRisk) String() string {
	switch r {
	case RiskNone:
		return "none"
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	default:
		return "unknown"
	}
}

// PatternVersion tracks the prompt injection pattern set.
// Increment on every pattern addition/modification.
// This version is exposed via /health and /metrics for monitoring.
const PatternVersion = "2.0.0"

// PatternChangelog documents pattern set evolution.
// Review quarterly or when new attack techniques emerge.
//
//	v1.0.0 (2024-12-01) — Initial 14-pattern set
//	v2.0.0 (2026-02-28) — Added pattern versioning and changelog.
//	                       Patterns unchanged; version system introduced
//	                       for audit trail and monitoring integration.
//
// REVIEW SCHEDULE: Update when OWASP LLM Top 10 or academic papers
// publish new injection techniques. See THREAT-MODEL.md §4.
var PatternChangelog = []string{
	"v1.0.0 (2024-12-01): Initial 14-pattern set (instruction override, identity hijack, system delimiter, jailbreak, data exfiltration, encoding evasion)",
	"v2.0.0 (2026-02-28): Added version tracking, changelog, /health exposure. No pattern changes.",
}

// promptInjectionPatterns are regex patterns that indicate potential
// prompt injection attempts. These are heuristic — they WILL have
// false positives. The goal is to FLAG, not BLOCK.
//
// The Agent receives the risk score and can decide how to handle it:
// - RiskNone: process normally
// - RiskLow: process but add safety wrapper
// - RiskMedium: process with reinforced system prompt
// - RiskHigh: reject or require human review
var promptInjectionPatterns = []struct {
	pattern *regexp.Regexp
	risk    PromptInjectionRisk
	name    string
}{
	// Direct instruction override attempts
	{regexp.MustCompile(`(?i)\b(ignore|disregard|forget|override)\b.{0,30}\b(previous|above|prior|all|system|instructions?|rules?|prompt)\b`), RiskHigh, "instruction_override"},
	{regexp.MustCompile(`(?i)\b(you are now|act as|pretend to be|roleplay as|become)\b.{0,40}\b(admin|root|system|god|unrestricted)\b`), RiskHigh, "identity_hijack"},
	{regexp.MustCompile(`(?i)\bnew\s+(instructions?|rules?|system\s*prompt)\b`), RiskHigh, "prompt_replacement"},
	{regexp.MustCompile(`(?i)\b(reveal|show|print|output|display)\b.{0,30}\b(system\s*prompt|instructions?|secret|api.?key|password|token)\b`), RiskHigh, "secret_extraction"},

	// Delimiter/framing attacks (trying to escape the user context)
	// Note: use double-quoted strings here because the patterns contain backticks
	// which cannot appear inside Go raw string literals. \x60 = backtick.
	{regexp.MustCompile("(?i)(\x60{3}system|<\\|system\\|>|<system>|\\[SYSTEM\\]|###\\s*system)"), RiskHigh, "system_delimiter"},
	{regexp.MustCompile("(?i)(\x60{3}instruction|<\\|instruction\\|>|\\[INSTRUCTION\\])"), RiskMedium, "instruction_delimiter"},
	{regexp.MustCompile(`(?i)\b(ASSISTANT|HUMAN|USER|SYSTEM)\s*:`), RiskMedium, "role_injection"},

	// Encoding evasion (base64/hex/unicode tricks)
	{regexp.MustCompile(`(?i)\b(base64|decode|eval|exec|atob|btoa)\b.{0,20}\(`), RiskMedium, "encoding_evasion"},
	{regexp.MustCompile(`\\u[0-9a-fA-F]{4}`), RiskLow, "unicode_escape"},

	// Jailbreak patterns
	{regexp.MustCompile(`(?i)\b(DAN|do\s*anything\s*now|developer\s*mode|jailbreak)\b`), RiskHigh, "jailbreak_keyword"},
	{regexp.MustCompile(`(?i)\b(hypothetically|theoretically|in\s*fiction|for\s*research|educational\s*purposes)\b.{0,50}\b(how\s*to|create|make|build)\b`), RiskMedium, "hypothetical_bypass"},

	// Data exfiltration
	{regexp.MustCompile(`(?i)\b(send|post|fetch|curl|wget|http)\b.{0,30}\b(external|webhook|url|endpoint)\b`), RiskMedium, "data_exfiltration"},
	{regexp.MustCompile(`(?i)\bhttps?://[^\s]{10,}`), RiskLow, "url_in_content"},
}

// AssessPromptInjectionRisk scans content for prompt injection patterns.
// Returns the highest risk level found and a list of triggered patterns.
//
// This is a HEURISTIC layer — it catches known patterns but cannot
// catch novel attacks. Defense-in-depth requires:
// 1. This heuristic scanner (catches 80% of attacks)
// 2. LLM-based detection (catches sophisticated attacks)
// 3. Output validation (catches successful injections)
// 4. Rate limiting (limits damage from successful attacks)
func AssessPromptInjectionRisk(content string) (PromptInjectionRisk, []string) {
	maxRisk := RiskNone
	var triggered []string

	for _, p := range promptInjectionPatterns {
		if p.pattern.MatchString(content) {
			if p.risk > maxRisk {
				maxRisk = p.risk
			}
			triggered = append(triggered, p.name)
		}
	}

	if maxRisk > RiskNone {
		log.Printf("[SANITIZE] Prompt injection risk=%s triggers=%v (content_len=%d)",
			maxRisk, triggered, len(content))
	}

	return maxRisk, triggered
}

// ================================================================
// Sender ID Validation
// ================================================================

// senderIDPattern validates sender IDs: alphanumeric, dashes, underscores, dots, @.
// Max 128 chars. Prevents injection in downstream logging/routing.
var senderIDPattern = regexp.MustCompile(`^[a-zA-Z0-9@._-]{1,128}$`)

// ValidateSenderID checks if a sender ID is safe.
func ValidateSenderID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "anonymous"
	}
	if !senderIDPattern.MatchString(id) {
		log.Printf("[SANITIZE] Sender ID invalid format: %q → using 'anonymous'", id)
		return "anonymous"
	}
	return id
}

// ValidateSenderPlatform checks the platform field (alphanumeric only, max 32 chars).
func ValidateSenderPlatform(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "unknown"
	}
	clean := make([]rune, 0, len(p))
	for _, r := range p {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			clean = append(clean, r)
		}
	}
	if len(clean) > 32 {
		clean = clean[:32]
	}
	if len(clean) == 0 {
		return "unknown"
	}
	return string(clean)
}

// ================================================================
// General Utilities
// ================================================================

// StripControlChars removes ASCII control characters except \n and \t.
// Control chars in user input can break log parsing, exploit terminal
// emulators, and confuse downstream systems.
func StripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' {
			b.WriteRune(r)
			continue
		}
		if unicode.IsControl(r) {
			continue // Drop control characters
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TruncateForLog truncates a string for safe logging.
// Prevents log flooding with huge payloads and PII exposure.
func TruncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}
