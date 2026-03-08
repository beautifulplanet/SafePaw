package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSingleJoiningSlash(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"/api/", "/v1", "/api/v1"},
		{"/api", "/v1", "/api/v1"},
		{"/api/", "v1", "/api/v1"},
		{"/api", "v1", "/api/v1"},
		{"/", "/", "/"},
		{"", "/test", "/test"},
	}
	for _, tc := range tests {
		got := singleJoiningSlash(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("singleJoiningSlash(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestBodyScanner_SkipsGET(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := bodyScanner(1024, inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET should pass through, got %d", rec.Code)
	}
}

func TestBodyScanner_ScansJSON(t *testing.T) {
	var gotRisk string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotRisk = r.Header.Get("X-SafePaw-Risk")
	})
	handler := bodyScanner(4096, inner)
	body := `{"message": "hello world"}`
	req := httptest.NewRequest("POST", "/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if gotRisk == "" {
		t.Error("X-SafePaw-Risk header should be set after scanning")
	}
}

func TestBodyScanner_PreservesBody(t *testing.T) {
	var bodyAfterScan []byte
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		bodyAfterScan, _ = io.ReadAll(r.Body)
	})
	handler := bodyScanner(4096, inner)
	original := `{"content": "test"}`
	req := httptest.NewRequest("POST", "/api", bytes.NewBufferString(original))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if string(bodyAfterScan) != original {
		t.Errorf("body after scan = %q, want %q", string(bodyAfterScan), original)
	}
}

func TestBodyScanner_SkipsNonJSON(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := bodyScanner(4096, inner)
	req := httptest.NewRequest("POST", "/upload", bytes.NewBufferString("binary-data"))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("non-JSON should pass through, got %d", rec.Code)
	}
}
