// =============================================================
// SafePaw Wizard - Setup Verification Endpoint
// =============================================================
// POST /api/v1/setup/verify performs a real round-trip check:
//   1. Is an API key configured?
//   2. Is the gateway reachable?
//   3. Can we reach the backend (OpenClaw) through the gateway?
// Returns per-check pass/fail with plain-English messages.
// =============================================================

package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type verifyCheck struct {
	Name    string `json:"name"`
	Pass    bool   `json:"pass"`
	Message string `json:"message"`
}

type verifyResponse struct {
	Checks  []verifyCheck `json:"checks"`
	Overall bool          `json:"overall"`
}

func (h *Handler) handleSetupVerify(w http.ResponseWriter, r *http.Request) {
	var checks []verifyCheck

	// ── Check 1: API key configured ──
	env, err := ReadEnvFile(h.cfg.EnvFilePath)
	if err != nil {
		checks = append(checks, verifyCheck{
			Name:    "API Key",
			Pass:    false,
			Message: "Could not read configuration file. Check that the .env file exists.",
		})
		writeJSON(w, http.StatusOK, verifyResponse{Checks: checks, Overall: false})
		return
	}

	hasKey := env["ANTHROPIC_API_KEY"] != "" || env["OPENAI_API_KEY"] != ""
	if hasKey {
		provider := "Anthropic"
		if env["OPENAI_API_KEY"] != "" && env["ANTHROPIC_API_KEY"] == "" {
			provider = "OpenAI"
		}
		checks = append(checks, verifyCheck{
			Name:    "API Key",
			Pass:    true,
			Message: provider + " API key is configured.",
		})
	} else {
		checks = append(checks, verifyCheck{
			Name:    "API Key",
			Pass:    false,
			Message: "No AI provider API key found. Go back and add one.",
		})
	}

	// ── Check 2: Gateway reachable ──
	gatewayBase := "http://safepaw-gateway:8080"
	if gw := os.Getenv("GATEWAY_URL"); gw != "" {
		gatewayBase = gw
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	gatewayOK := false
	healthReq, err := http.NewRequestWithContext(ctx, "GET", gatewayBase+"/health", nil)
	if err == nil {
		resp, err := http.DefaultClient.Do(healthReq)
		if err == nil {
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			if resp.StatusCode == http.StatusOK {
				gatewayOK = true
			}
		}
	}

	if gatewayOK {
		checks = append(checks, verifyCheck{
			Name:    "Gateway",
			Pass:    true,
			Message: "SafePaw gateway is running and responding.",
		})
	} else {
		checks = append(checks, verifyCheck{
			Name:    "Gateway",
			Pass:    false,
			Message: "Cannot reach the gateway. Make sure all services are running (check the Prerequisites page).",
		})
	}

	// ── Check 3: Backend reachable through gateway ──
	// Mint a short-lived token and send a proxied request through the gateway.
	// Any non-502/503 response means the backend is alive.
	secret := env["AUTH_SECRET"]
	backendOK := false

	if !gatewayOK {
		checks = append(checks, verifyCheck{
			Name:    "Backend (AI Service)",
			Pass:    false,
			Message: "Skipped — gateway must be reachable first.",
		})
	} else if secret == "" || len(secret) < 32 {
		// No auth secret — try an unauthenticated request if auth is disabled
		authEnabled := env["AUTH_ENABLED"]
		if authEnabled == "false" {
			probeReq, err := http.NewRequestWithContext(ctx, "GET", gatewayBase+"/", nil)
			if err == nil {
				resp, err := http.DefaultClient.Do(probeReq)
				if err == nil {
					defer resp.Body.Close()
					_, _ = io.Copy(io.Discard, resp.Body)
					// 502/503 means backend is down; anything else means it's reachable
					if resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusServiceUnavailable {
						backendOK = true
					}
				}
			}
		}

		if backendOK {
			checks = append(checks, verifyCheck{
				Name:    "Backend (AI Service)",
				Pass:    true,
				Message: "AI backend is reachable through the gateway.",
			})
		} else {
			checks = append(checks, verifyCheck{
				Name:    "Backend (AI Service)",
				Pass:    false,
				Message: "AUTH_SECRET is not configured or too short. Set it in Security settings to enable full verification.",
			})
		}
	} else {
		// Mint a short-lived token
		now := time.Now().Unix()
		payload, _ := json.Marshal(map[string]interface{}{
			"sub": "wizard-verify", "iat": now, "exp": now + 15, "scope": "proxy",
		})
		payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		token := payloadB64 + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

		probeReq, err := http.NewRequestWithContext(ctx, "GET", gatewayBase+"/", nil)
		if err == nil {
			probeReq.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(probeReq)
			if err == nil {
				defer resp.Body.Close()
				_, _ = io.Copy(io.Discard, resp.Body)
				if resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusServiceUnavailable {
					backendOK = true
				}
			}
		}

		if backendOK {
			checks = append(checks, verifyCheck{
				Name:    "Backend (AI Service)",
				Pass:    true,
				Message: "AI backend is reachable through the gateway. The full chain works!",
			})
		} else {
			checks = append(checks, verifyCheck{
				Name:    "Backend (AI Service)",
				Pass:    false,
				Message: "Gateway is up but the AI backend is not responding. Make sure the OpenClaw container is running.",
			})
		}
	}

	ip := clientIP(r)
	overall := true
	for _, c := range checks {
		if !c.Pass {
			overall = false
			break
		}
	}

	if overall {
		log.Printf("[INFO] Setup verification passed for %s", sanitizeLog(ip))
	} else {
		log.Printf("[WARN] Setup verification had failures for %s", sanitizeLog(ip))
	}

	writeJSON(w, http.StatusOK, verifyResponse{Checks: checks, Overall: overall})
}
