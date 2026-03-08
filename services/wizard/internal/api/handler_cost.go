package api

import (
	"net/http"
	"strconv"

	"safepaw/wizard/internal/costhistory"
)

// handleCostHistory returns daily cost snapshots from Postgres.
// Query params:
//
//	days=N (default 30, max 365)
//
// Response: {"status":"ok","daily":[...]} or {"status":"unavailable"}
func (h *Handler) handleCostHistory(w http.ResponseWriter, r *http.Request) {
	if h.costQuerier == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}

	days := parseDays(r, 30, 365)
	rows, err := h.costQuerier.ListDailySnapshots(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "error",
			"error":  "database query failed",
		})
		return
	}
	if rows == nil {
		rows = []costhistory.DailyRow{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"days":   days,
		"daily":  rows,
	})
}

// handleCostModels returns per-model cost breakdown from Postgres.
// Query params:
//
//	days=N (default 30, max 365)
//
// Response: {"status":"ok","models":[...]} or {"status":"unavailable"}
func (h *Handler) handleCostModels(w http.ResponseWriter, r *http.Request) {
	if h.costQuerier == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}

	days := parseDays(r, 30, 365)
	rows, err := h.costQuerier.ListModelSnapshots(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "error",
			"error":  "database query failed",
		})
		return
	}
	if rows == nil {
		rows = []costhistory.ModelRow{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"days":   days,
		"models": rows,
	})
}

// handleCostTrends returns trend analysis comparing recent vs prior period.
// Query params:
//
//	days=N (default 7, max 90) — size of each comparison window
//
// Response: {"status":"ok","trend":{...}} or {"status":"unavailable"}
func (h *Handler) handleCostTrends(w http.ResponseWriter, r *http.Request) {
	if h.costQuerier == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}

	days := parseDays(r, 7, 90)
	trend, err := h.costQuerier.GetTrend(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "error",
			"error":  "database query failed",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"trend":  trend,
	})
}

// parseDays reads ?days=N from query string, clamped to [1, max].
func parseDays(r *http.Request, defaultDays, maxDays int) int {
	s := r.URL.Query().Get("days")
	if s == "" {
		return defaultDays
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultDays
	}
	if n > maxDays {
		return maxDays
	}
	return n
}
