// =============================================================
// SafePaw Wizard - .env file read/write for config API
// =============================================================
// Parses and writes KEY=VALUE .env files. Used by GET/PUT
// /api/v1/config to show masked config and update allowed keys.
// =============================================================

package api

import (
	"bufio"
	"bytes"
	"os"
	"strings"
)

// Keys that are considered secrets: mask value in GET responses.
var secretKeys = map[string]bool{
	"REDIS_PASSWORD": true, "POSTGRES_PASSWORD": true, "WIZARD_ADMIN_PASSWORD": true,
	"AUTH_SECRET": true, "ANTHROPIC_API_KEY": true, "OPENAI_API_KEY": true,
	"DISCORD_BOT_TOKEN": true, "TELEGRAM_BOT_TOKEN": true, "SLACK_BOT_TOKEN": true,
	"SLACK_APP_TOKEN": true,
}

// readEnvFile reads path and returns a map of key -> value.
// Comment lines and empty lines are skipped. Invalid lines are skipped.
func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is from internal config, not user input
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
		out[key] = val
	}
	return out, scanner.Err()
}

// maskValue returns a masked string for secrets (e.g. "***xxxx" for last 4 chars).
func maskValue(key, value string) string {
	if value == "" {
		return ""
	}
	if !secretKeys[key] {
		return value
	}
	if len(value) <= 4 {
		return "***"
	}
	return "***" + value[len(value)-4:]
}

// allowedConfigKeys is the set of keys that PUT /api/v1/config may update.
// Excludes REDIS_PASSWORD, POSTGRES_* to avoid breaking the stack by accident.
var allowedConfigKeys = map[string]bool{
	"WIZARD_ADMIN_PASSWORD": true,
	"WIZARD_TOTP_SECRET":    true, // Optional; when set, login requires TOTP code (MFA)
	"AUTH_ENABLED":          true, "AUTH_SECRET": true, "AUTH_DEFAULT_TTL_HOURS": true, "AUTH_MAX_TTL_HOURS": true,
	"TLS_ENABLED": true, "TLS_CERT_FILE": true, "TLS_KEY_FILE": true, "TLS_PORT": true,
	"RATE_LIMIT": true, "RATE_LIMIT_WINDOW_SEC": true,
	"ANTHROPIC_API_KEY": true, "OPENAI_API_KEY": true,
	"DISCORD_BOT_TOKEN": true, "TELEGRAM_BOT_TOKEN": true, "SLACK_BOT_TOKEN": true, "SLACK_APP_TOKEN": true,
	"SIGNAL_CLI_PATH": true,
}

// writeEnvFile updates path by replacing values for keys in updates and
// appending any new keys. Preserves comments, blank lines, and key order.
func writeEnvFile(path string, updates map[string]string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- path is from internal config, not user input
	if err != nil {
		return err
	}
	updated := make(map[string]bool)
	var out strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		idx := strings.Index(trimmed, "=")
		if idx <= 0 {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		if newVal, ok := updates[key]; ok {
			out.WriteString(key + "=" + escapeEnvValue(newVal) + "\n")
			updated[key] = true
		} else {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	for k, v := range updates {
		if !updated[k] {
			out.WriteString(k + "=" + escapeEnvValue(v) + "\n")
		}
	}
	return os.WriteFile(path, []byte(out.String()), 0600)
}

func escapeEnvValue(v string) string {
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, " \t\n\"#'") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}
