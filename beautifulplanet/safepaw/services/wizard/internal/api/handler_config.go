// =============================================================
// SafePaw Wizard - Config API (GET/PUT .env)
// =============================================================
// GET returns current config with secrets masked.
// PUT updates only allowed keys; preserves file structure.
// =============================================================

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

var configWriteMu sync.Mutex

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	env, err := readEnvFile(h.cfg.EnvFilePath)
	if err != nil {
		log.Printf("[WARN] Config read failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to read config"})
		return
	}
	masked := make(map[string]string, len(env))
	for k, v := range env {
		masked[k] = maskValue(k, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": masked})
}

func (h *Handler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024) // 32KB max
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid JSON"})
		return
	}
	updates := make(map[string]string)
	for k, v := range body {
		if !allowedConfigKeys[k] {
			log.Printf("[WARN] Config PUT rejected unknown key: %q", k)
			continue
		}
		updates[k] = v
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	configWriteMu.Lock()
	defer configWriteMu.Unlock()
	if err := writeEnvFile(h.cfg.EnvFilePath, updates); err != nil {
		log.Printf("[WARN] Config write failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to write config"})
		return
	}

	// Invalidate existing sessions when credentials change so users must re-login with new password/TOTP
	if _, touchedPassword := updates["WIZARD_ADMIN_PASSWORD"]; touchedPassword {
		h.ReloadCredsFromEnv()
		h.BumpSessionGen()
	} else if _, touchedTOTP := updates["WIZARD_TOTP_SECRET"]; touchedTOTP {
		h.ReloadCredsFromEnv()
		h.BumpSessionGen()
	}

	configIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		configIP = fwd
	}
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	h.audit.ConfigChange(configIP, keys)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
