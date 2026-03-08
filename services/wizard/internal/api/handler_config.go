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

// systemProfiles maps SYSTEM_PROFILE values to per-service resource limits.
// These are written as env vars that docker-compose.yml references.
var systemProfiles = map[string]map[string]string{
	"small": {
		"WIZARD_MEM_LIMIT":        "128M",
		"GATEWAY_MEM_LIMIT":       "256M",
		"OPENCLAW_MEM_LIMIT":      "2G",
		"REDIS_MEM_LIMIT":         "256M",
		"REDIS_MAXMEMORY":         "128mb",
		"POSTGRES_MEM_LIMIT":      "256M",
		"POSTGRES_SHARED_BUFFERS": "128MB",
		"POSTGRES_CACHE_SIZE":     "256MB",
	},
	"medium": {
		"WIZARD_MEM_LIMIT":        "128M",
		"GATEWAY_MEM_LIMIT":       "512M",
		"OPENCLAW_MEM_LIMIT":      "8G",
		"REDIS_MEM_LIMIT":         "512M",
		"REDIS_MAXMEMORY":         "256mb",
		"POSTGRES_MEM_LIMIT":      "1G",
		"POSTGRES_SHARED_BUFFERS": "512MB",
		"POSTGRES_CACHE_SIZE":     "1GB",
	},
	"large": {
		"WIZARD_MEM_LIMIT":        "256M",
		"GATEWAY_MEM_LIMIT":       "1G",
		"OPENCLAW_MEM_LIMIT":      "32G",
		"REDIS_MEM_LIMIT":         "2G",
		"REDIS_MAXMEMORY":         "1gb",
		"POSTGRES_MEM_LIMIT":      "4G",
		"POSTGRES_SHARED_BUFFERS": "2GB",
		"POSTGRES_CACHE_SIZE":     "4GB",
	},
	"very-large": {
		"WIZARD_MEM_LIMIT":        "256M",
		"GATEWAY_MEM_LIMIT":       "2G",
		"OPENCLAW_MEM_LIMIT":      "96G",
		"REDIS_MEM_LIMIT":         "4G",
		"REDIS_MAXMEMORY":         "2gb",
		"POSTGRES_MEM_LIMIT":      "8G",
		"POSTGRES_SHARED_BUFFERS": "4GB",
		"POSTGRES_CACHE_SIZE":     "8GB",
	},
}

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	env, err := ReadEnvFile(h.cfg.EnvFilePath)
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

	// When SYSTEM_PROFILE changes, expand the per-service memory vars
	if profile, ok := updates["SYSTEM_PROFILE"]; ok {
		if derived, valid := systemProfiles[profile]; valid {
			for k, v := range derived {
				updates[k] = v
			}
		} else {
			writeJSON(w, http.StatusBadRequest, errorResponse{"invalid SYSTEM_PROFILE: must be small, medium, large, or very-large"})
			return
		}
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
	} else if _, touchedOp := updates["WIZARD_OPERATOR_PASSWORD"]; touchedOp {
		h.ReloadCredsFromEnv()
		h.BumpSessionGen()
	} else if _, touchedViewer := updates["WIZARD_VIEWER_PASSWORD"]; touchedViewer {
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
