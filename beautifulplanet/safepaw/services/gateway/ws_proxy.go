// =============================================================
// SafePaw Gateway - WebSocket Reverse Proxy
// =============================================================
// Explicit WebSocket upgrade handler for proxying WS connections
// to the OpenClaw backend. While Go's httputil.ReverseProxy can
// handle upgrades, this explicit handler gives us:
//   - Dedicated logging for WS connections
//   - Timeout control on the initial handshake
//   - Clean bidirectional copy with proper error handling
//   - No interference from the body scanner middleware
//
// The approach: detect Upgrade header → hijack client conn →
// dial backend → replay the upgrade request → bidirectional copy.
// =============================================================

package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"safepaw/gateway/middleware"
)

const (
	wsDialTimeout = 10 * time.Second
	wsBufferSize  = 32 * 1024 // 32KB copy buffer
)

// isWebSocketUpgrade checks if a request is a WebSocket upgrade.
// The Connection header may contain multiple comma-separated values
// (e.g. "keep-alive, Upgrade"), so we check if "upgrade" appears
// anywhere in the header rather than requiring an exact match.
func isWebSocketUpgrade(r *http.Request) bool {
	return headerContains(r.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// headerContains checks if a comma-separated header value includes a token (case-insensitive).
func headerContains(header, token string) bool {
	for _, v := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(v), token) {
			return true
		}
	}
	return false
}

// wsProxy creates a handler that proxies WebSocket connections to the backend.
// It hijacks the client connection and creates a raw TCP tunnel to the backend,
// then copies data bidirectionally until either side closes.
func wsProxy(target *url.URL) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Determine backend address
		backendAddr := target.Host
		if !strings.Contains(backendAddr, ":") {
			if target.Scheme == "https" || target.Scheme == "wss" {
				backendAddr += ":443"
			} else {
				backendAddr += ":80"
			}
		}

		log.Printf("[WS] Upgrade request: %s -> %s (remote=%s)",
			r.URL.Path, backendAddr, r.RemoteAddr)

		// Dial the backend
		backendConn, err := net.DialTimeout("tcp", backendAddr, wsDialTimeout)
		if err != nil {
			log.Printf("[WS] Backend dial failed: %v", err)
			http.Error(w, `{"error":"backend_unavailable"}`, http.StatusBadGateway)
			return
		}
		defer backendConn.Close()

		// Hijack the client connection
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			log.Println("[WS] ResponseWriter does not support hijacking")
			http.Error(w, `{"error":"websocket_unsupported"}`, http.StatusInternalServerError)
			return
		}

		clientConn, clientBuf, err := hijacker.Hijack()
		if err != nil {
			log.Printf("[WS] Hijack failed: %v", err)
			return
		}
		defer clientConn.Close()

		// Replay the original HTTP request to the backend (the upgrade handshake)
		// Rewrite the Host header to match the backend
		r.Host = target.Host
		r.URL.Host = target.Host
		r.URL.Scheme = "http"
		if target.Scheme == "https" || target.Scheme == "wss" {
			r.URL.Scheme = "https"
		}

		// Forward the request to the backend
		if err := r.Write(backendConn); err != nil {
			log.Printf("[WS] Failed to write upgrade request to backend: %v", err)
			return
		}

		// Flush any buffered data from the hijacked reader
		if clientBuf.Reader.Buffered() > 0 {
			buffered := make([]byte, clientBuf.Reader.Buffered())
			if _, err := clientBuf.Read(buffered); err == nil {
				_, _ = backendConn.Write(buffered)
			}
		}

		log.Printf("[WS] Tunnel established: %s <-> %s (path=%s)",
			r.RemoteAddr, backendAddr, r.URL.Path)

		// Bidirectional copy — when either side closes, both are torn down
		done := make(chan struct{}, 2)
		reqID := r.Header.Get("X-Request-ID")

		// Backend → Client: scan output for dangerous content
		go func() {
			scanner := middleware.NewScanningReader(backendConn, reqID, r.URL.Path)
			buf := make([]byte, wsBufferSize)
			_, _ = io.CopyBuffer(clientConn, scanner, buf)
			done <- struct{}{}
		}()

		// Client → Backend: pass through (input already scanned by body scanner)
		go func() {
			buf := make([]byte, wsBufferSize)
			_, _ = io.CopyBuffer(backendConn, clientConn, buf)
			done <- struct{}{}
		}()

		// Wait for either direction to finish
		<-done

		log.Printf("[WS] Tunnel closed: %s (path=%s)", r.RemoteAddr, r.URL.Path)
	})
}
