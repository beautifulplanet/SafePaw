package main

import (
	"net/http"
	"testing"
)

func TestIsWebSocketUpgrade(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		upgrade    string
		want       bool
	}{
		{"valid WS", "Upgrade", "websocket", true},
		{"case insensitive", "upgrade", "WebSocket", true},
		{"multi-value connection", "keep-alive, Upgrade", "websocket", true},
		{"multi-value mixed case", "Keep-Alive, upgrade", "WebSocket", true},
		{"no upgrade header", "", "websocket", false},
		{"no websocket header", "Upgrade", "", false},
		{"keep-alive only", "keep-alive", "", false},
		{"empty", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/ws", nil)
			if tc.connection != "" {
				r.Header.Set("Connection", tc.connection)
			}
			if tc.upgrade != "" {
				r.Header.Set("Upgrade", tc.upgrade)
			}

			if got := isWebSocketUpgrade(r); got != tc.want {
				t.Errorf("isWebSocketUpgrade() = %v, want %v", got, tc.want)
			}
		})
	}
}
