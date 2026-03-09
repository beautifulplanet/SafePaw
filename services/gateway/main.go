// =============================================================
// SafePaw Gateway - Secure Reverse Proxy
// =============================================================
// Security-hardened reverse proxy that sits in front of OpenClaw.
//
// What it does:
// 1. Loads configuration from environment variables
// 2. Creates a reverse proxy to the OpenClaw backend
// 3. Applies security middleware (headers, rate limit, origin, auth)
// 4. Scans request bodies for prompt injection patterns
// 5. Proxies all HTTP and WebSocket traffic to OpenClaw
// 6. Handles graceful shutdown (SIGINT/SIGTERM)
//
// SafePaw Gateway is the security perimeter around OpenClaw.
// Every request passes through the defense layers first.
// =============================================================

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"safepaw/gateway/config"
	"safepaw/gateway/middleware"
)

func main() {
	if middleware.InstallJSONLogger() {
		log.Println("[CONFIG] Structured JSON logging enabled (LOG_FORMAT=json)")
	} else {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	}
	log.Println("=== SafePaw Gateway starting ===")

	// --------------------------------------------------------
	// Step 1: Load configuration
	// --------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Config load failed: %v", err)
	}
	log.Printf("[CONFIG] Port=%d ProxyTarget=%s TLS=%v Auth=%v",
		cfg.Port, cfg.ProxyTarget.String(), cfg.TLSEnabled, cfg.AuthEnabled)
	if !cfg.AuthEnabled {
		log.Println("[WARN] ============================================================")
		log.Println("[WARN]  AUTH_ENABLED=false — gateway is running WITHOUT authentication!")
		log.Println("[WARN]  Set AUTH_ENABLED=true and AUTH_SECRET in production.")
		log.Println("[WARN] ============================================================")
	}

	// --------------------------------------------------------
	// Step 2: Create reverse proxy to OpenClaw
	// --------------------------------------------------------
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = cfg.ProxyTarget.Scheme
			req.URL.Host = cfg.ProxyTarget.Host
			req.Host = cfg.ProxyTarget.Host

			// Preserve the original path  proxy everything
			if cfg.ProxyTarget.Path != "" && cfg.ProxyTarget.Path != "/" {
				req.URL.Path = singleJoiningSlash(cfg.ProxyTarget.Path, req.URL.Path)
			}

			// Strip hop-by-hop headers that shouldn't be forwarded
			req.Header.Del("X-SafePaw-Risk")     // Don't let clients spoof risk headers
			req.Header.Del("X-SafePaw-Triggers") // Don't let clients spoof trigger headers

			// Strip original auth credentials — backend uses X-Auth-Subject/X-Auth-Scope
			req.Header.Del("Authorization")
			q := req.URL.Query()
			if q.Has("token") {
				q.Del("token")
				req.URL.RawQuery = q.Encode()
			}

			log.Printf("[PROXY] %s %s -> %s%s (remote=%s request_id=%s)",
				req.Method, req.URL.Path, cfg.ProxyTarget.Host, req.URL.Path, req.RemoteAddr, req.Header.Get("X-Request-ID"))
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[PROXY] Backend error: %v (path=%s remote=%s request_id=%s)", err, r.URL.Path, r.RemoteAddr, r.Header.Get("X-Request-ID"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- best-effort HTTP response
				"error":   "bad_gateway",
				"message": "OpenClaw backend is unavailable",
			})
		},
		// Flush immediately for streaming responses (SSE, etc.)
		FlushInterval: -1,
	}

	// --------------------------------------------------------
	// Step 3: Set up rate limiter + brute force guard
	// --------------------------------------------------------
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit, cfg.RateLimitWindow)
	defer rateLimiter.Stop()

	// Shared Redis client for revocation + brute-force persistence
	var redisClient *middleware.RedisClient
	if cfg.RedisAddr != "" {
		redisClient = middleware.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword)
		log.Printf("[CONFIG] Redis connected at %s (persistent bans + revocation enabled)", cfg.RedisAddr)
	}

	bruteForce := middleware.NewBruteForceGuardWithRedis(5, 5*time.Minute, redisClient)
	defer bruteForce.Stop()

	// --------------------------------------------------------
	// Step 3b: Start cost monitoring collector
	// --------------------------------------------------------
	usageCollector := NewUsageCollector(
		cfg.OpenClawWSURL,
		cfg.OpenClawGatewayToken,
		cfg.CostAlertDailyWarn,
		cfg.CostAlertDailyCrit,
	)
	defer usageCollector.Stop()

	// --------------------------------------------------------
	// Step 3c: Create receipt ledger for agent action tracing
	// --------------------------------------------------------
	receiptLedger := middleware.NewLedger(0) // default 10k capacity
	log.Println("[CONFIG] Receipt ledger enabled (append-only, in-memory)")

	// --------------------------------------------------------
	// Step 4: Build HTTP routes with middleware stack
	// --------------------------------------------------------
	mux := http.NewServeMux()

	// Prometheus metrics
	metrics := middleware.NewMetrics()
	metrics.CostSnapshotFn = func() *middleware.CostSnapshot {
		snap := usageCollector.Snapshot()
		if snap.Status != "ok" {
			return nil
		}
		cs := &middleware.CostSnapshot{
			TotalCostUSD: snap.PeriodCost,
			TodayCostUSD: snap.TodayCost,
		}
		if snap.Totals != nil {
			cs.InputTokens = snap.Totals.Input
			cs.OutputTokens = snap.Totals.Output
			cs.CacheReadTokens = snap.Totals.CacheRead
			cs.CacheWriteTokens = snap.Totals.CacheWrite
		}
		return cs
	}
	mux.Handle("/metrics", metrics.Handler())

	// Health check  no auth, no middleware (used by Docker/k8s)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]interface{}{
			"status":          "ok",
			"service":         "safepaw-gateway",
			"pattern_version": middleware.PatternVersion,
			"timestamp":       time.Now().UTC().Format(time.RFC3339),
		}

		// Deep health check: probe the OpenClaw backend
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		healthURL := cfg.ProxyTarget.String() + "/health"
		req, _ := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			status["status"] = "degraded"
			status["backend"] = "unreachable"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			resp.Body.Close() // #nosec G104 -- close after status check
			if resp.StatusCode == http.StatusOK {
				status["backend"] = "healthy"
			} else {
				status["status"] = "degraded"
				status["backend"] = fmt.Sprintf("status_%d", resp.StatusCode)
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status) // #nosec G104 -- best-effort HTTP response
	})

	// Everything else -> reverse proxy to OpenClaw
	// WebSocket upgrades get the dedicated WS tunnel handler;
	// regular HTTP gets body scanning then reverse proxy.
	wsHandler := wsProxy(cfg.ProxyTarget, receiptLedger)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isWebSocketUpgrade(r) {
			wsHandler.ServeHTTP(w, r)
			return
		}
		middleware.OutputScanner(cfg.MaxBodySize, bodyScanner(cfg.MaxBodySize, proxy)).ServeHTTP(w, r)
	}))

	// Apply middleware (outermost first):
	// Request -> SecurityHeaders -> RequestID -> OriginCheck -> RateLimit -> [Auth] -> BodyScanner -> Proxy
	var handler http.Handler = mux

	// Auth middleware (only if enabled  disabled in dev by default)
	if cfg.AuthEnabled {
		auth, err := middleware.NewAuthenticator(cfg.AuthSecret, cfg.AuthDefaultTTL, cfg.AuthMaxTTL)
		if err != nil {
			log.Fatalf("[FATAL] Auth setup failed: %v", err)
		}

		revocations := middleware.NewRevocationListWithRedis(cfg.AuthMaxTTL, redisClient)
		defer revocations.Stop()

		// Admin endpoint for token revocation (requires admin scope)
		mux.Handle("/admin/revoke", middleware.AuthRequired(auth, "admin", revocations,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
					return
				}
				var body struct {
					Subject string `json:"subject"`
					Reason  string `json:"reason"`
				}
				if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil || body.Subject == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{"error": "subject is required"}) // #nosec G104 -- best-effort HTTP response
					return
				}
				revocations.Revoke(body.Subject, body.Reason)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- best-effort HTTP response
					"status":             "revoked",
					"subject":            body.Subject,
					"active_revocations": revocations.Count(),
				})
			})))

		// Cost monitoring endpoint (requires admin scope)
		mux.Handle("/admin/usage", middleware.AuthRequired(auth, "admin", revocations,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(usageCollector.Snapshot()) // #nosec G104 -- best-effort HTTP response
			})))

		// Pricing reference endpoint (requires admin scope)
		mux.Handle("/admin/pricing", middleware.AuthRequired(auth, "admin", revocations,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(PricingTable) // #nosec G104 -- best-effort HTTP response
			})))

		// Receipt ledger endpoint (requires admin scope)
		mux.Handle("/admin/ledger", middleware.AuthRequired(auth, "admin", revocations,
			ledgerHandler(receiptLedger)))

		log.Printf("[AUTH] Authentication ENABLED (default TTL=%v, max TTL=%v, revocation=enabled)",
			cfg.AuthDefaultTTL, cfg.AuthMaxTTL)
		handler = middleware.AuthRequiredWithGuard(auth, "proxy", revocations, bruteForce, handler)
	} else {
		log.Println("[SECURITY] ╔══════════════════════════════════════════════════════════════╗")
		log.Println("[SECURITY] ║  WARNING: Authentication is DISABLED                        ║")
		log.Println("[SECURITY] ║  All requests pass through without token validation.        ║")
		log.Println("[SECURITY] ║  Set AUTH_ENABLED=true and AUTH_SECRET for production.       ║")
		log.Println("[SECURITY] ╚══════════════════════════════════════════════════════════════╝")
		handler = middleware.StripAuthHeaders(handler)

		// Ledger endpoint without auth protection (dev mode only)
		mux.Handle("/admin/ledger", ledgerHandler(receiptLedger))
	}

	handler = middleware.RateLimitWithGuard(rateLimiter, bruteForce, handler)
	handler = middleware.BruteForceMiddleware(bruteForce, handler)
	handler = middleware.OriginCheck(cfg.AllowedOrigins, handler)
	handler = middleware.AuditEmitter(handler)
	handler = middleware.RequestID(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.MetricsMiddleware(metrics, handler)

	// --------------------------------------------------------
	// Step 5: Create and start HTTP server
	// --------------------------------------------------------
	server := &http.Server{
		Addr:           fmt.Sprintf(":%d", cfg.Port),
		Handler:        handler,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		MaxHeaderBytes: 1 << 16, // 64KB
	}

	go func() {
		if cfg.TLSEnabled {
			server.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
				CurvePreferences: []tls.CurveID{
					tls.X25519,
					tls.CurveP256,
				},
				CipherSuites: []uint16{
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				},
			}
			server.Addr = fmt.Sprintf(":%d", cfg.TLSPort)
			log.Printf("[SERVER] Listening on :%d (TLS ENABLED)", cfg.TLSPort)
			if err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("[FATAL] TLS server error: %v", err)
			}
		} else {
			log.Println("[SECURITY] ╔══════════════════════════════════════════════════════════════╗")
			log.Println("[SECURITY] ║  WARNING: TLS is DISABLED — traffic is unencrypted          ║")
			log.Println("[SECURITY] ║  Set TLS_ENABLED=true with cert/key for production.         ║")
			log.Println("[SECURITY] ╚══════════════════════════════════════════════════════════════╝")
			log.Printf("[SERVER] Listening on :%d (TLS disabled — dev mode)", cfg.Port)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("[FATAL] Server error: %v", err)
			}
		}
	}()

	// --------------------------------------------------------
	// Step 6: Graceful shutdown
	// --------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	shutdownStart := time.Now()
	log.Printf("[SHUTDOWN] Received signal: %v", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[SHUTDOWN] Server shutdown error: %v", err)
	}

	log.Printf("=== SafePaw Gateway stopped (shutdown: %v) ===",
		time.Since(shutdownStart).Round(time.Millisecond))
}

// bodyScanner is middleware that reads JSON request bodies on mutating
// methods (POST, PUT, PATCH) and scans for prompt injection patterns
// using the sanitize module. It adds an X-SafePaw-Risk header with
// the assessed risk level so OpenClaw (or logs) can see it.
//
// Non-JSON or GET/HEAD/OPTIONS requests pass through unscanned.
func bodyScanner(maxSize int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only scan mutating requests with bodies
		if r.Method != "POST" && r.Method != "PUT" && r.Method != "PATCH" {
			next.ServeHTTP(w, r)
			return
		}

		// Only scan JSON content types
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") && !strings.Contains(ct, "text/") {
			next.ServeHTTP(w, r)
			return
		}

		// Reject requests that declare a body larger than maxSize
		if r.ContentLength > maxSize {
			log.Printf("[SCANNER] Request too large: content_length=%d max=%d remote=%s request_id=%s",
				r.ContentLength, maxSize, r.RemoteAddr, r.Header.Get("X-Request-ID"))
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}

		// Read body (with size limit)
		if r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Read up to maxSize+1 to detect truncation
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxSize+1))
		r.Body.Close() // #nosec G104 -- body already read
		if err != nil {
			log.Printf("[SCANNER] Body read error: %v (remote=%s request_id=%s)", err, r.RemoteAddr, r.Header.Get("X-Request-ID"))
			next.ServeHTTP(w, r)
			return
		}

		// If we read more than maxSize, the body exceeded the limit
		if int64(len(bodyBytes)) > maxSize {
			log.Printf("[SCANNER] Request body too large: read=%d max=%d remote=%s request_id=%s",
				len(bodyBytes), maxSize, r.RemoteAddr, r.Header.Get("X-Request-ID"))
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}

		// Restore the body so the proxy can forward it
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))

		// Scan for prompt injection
		bodyStr := string(bodyBytes)
		risk, triggers := middleware.AssessPromptInjectionRisk(bodyStr)
		if risk > middleware.RiskNone {
			log.Printf("[SCANNER] Prompt injection risk=%s triggers=%v path=%s remote=%s body_len=%d request_id=%s",
				risk, triggers, r.URL.Path, r.RemoteAddr, len(bodyBytes), r.Header.Get("X-Request-ID"))
		}

		if sc := middleware.GetSecurityContext(r); sc != nil {
			sc.InputScan = &middleware.ScanDecision{
				Risk:     risk.String(),
				Triggers: triggers,
			}
		}

		// Attach risk assessment as header (OpenClaw can read this)
		r.Header.Set("X-SafePaw-Risk", risk.String())
		if len(triggers) > 0 {
			r.Header.Set("X-SafePaw-Triggers", strings.Join(triggers, ","))
		}

		// System prompt reinforcement: inject safety preamble on every request
		// so OpenClaw can prepend it to conversation context
		r.Header.Set("X-SafePaw-Context", middleware.SystemPromptReinforcement)

		next.ServeHTTP(w, r)
	})
}

// singleJoiningSlash joins a base path and a request path with exactly one slash.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

// ledgerHandler returns an HTTP handler that serves the receipt ledger.
// Supports query params: request_id, session_id, subject, action, since_seq, limit.
func ledgerHandler(ledger *middleware.Ledger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		query := middleware.LedgerQuery{
			RequestID: q.Get("request_id"),
			SessionID: q.Get("session_id"),
			Subject:   q.Get("subject"),
			Action:    q.Get("action"),
		}

		if v := q.Get("since_seq"); v != "" {
			if seq, err := strconv.ParseUint(v, 10, 64); err == nil {
				query.SinceSeq = seq
			}
		}
		if v := q.Get("limit"); v != "" {
			if lim, err := strconv.Atoi(v); err == nil && lim > 0 {
				query.Limit = lim
			}
		}

		// Default: if no filters, return recent entries
		var receipts []middleware.Receipt
		if query.RequestID == "" && query.SessionID == "" && query.Subject == "" && query.Action == "" && query.SinceSeq == 0 {
			limit := query.Limit
			if limit <= 0 {
				limit = 100
			}
			receipts = ledger.Recent(limit)
		} else {
			receipts = ledger.Query(query)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- best-effort HTTP response
			"total_receipts": ledger.Count(),
			"last_seq":       ledger.LastSeq(),
			"receipts":       receipts,
		})
	})
}
