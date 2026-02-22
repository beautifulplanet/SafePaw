// =============================================================
// NOPEnclaw Gateway — WebSocket Handler
// =============================================================
// This is the heart of the Gateway. It:
//
// 1. Upgrades HTTP → WebSocket (the "handshake")
// 2. Manages read/write goroutines per connection
// 3. Receives messages from users and publishes to Redis Streams
// 4. Tracks active connections for outbound delivery
//
// Think of this like the mailroom:
//   - Users knock on the door (HTTP request)
//   - We open a special fast-lane window (WebSocket upgrade)
//   - They pass notes through the window (messages)
//   - We put each note on the conveyor belt (Redis Streams)
// =============================================================

package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"nopenclaw/gateway/config"
	"nopenclaw/gateway/middleware"
	redisStream "nopenclaw/gateway/redis"
)

// ================================================================
// Connection Hub — tracks all active WebSocket connections
// ================================================================

// Hub maintains the set of active connections.
type Hub struct {
	mu          sync.RWMutex
	connections map[string]*Connection // sessionID → connection
	connCount   atomic.Int64
	maxConns    int
}

// NewHub creates an empty connection hub.
func NewHub(maxConns int) *Hub {
	return &Hub{
		connections: make(map[string]*Connection),
		maxConns:    maxConns,
	}
}

// Register adds a new connection. Returns false if at capacity.
// The capacity check and insertion are both inside the lock to prevent
// TOCTOU races where two goroutines both pass the check simultaneously.
func (h *Hub) Register(conn *Connection) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if int(h.connCount.Load()) >= h.maxConns {
		return false
	}

	h.connections[conn.SessionID] = conn
	h.connCount.Add(1)

	log.Printf("[HUB] Connection registered: session=%s (total=%d)", conn.SessionID, h.connCount.Load())
	return true
}

// Unregister removes a connection.
func (h *Hub) Unregister(sessionID string) {
	h.mu.Lock()
	if _, ok := h.connections[sessionID]; ok {
		delete(h.connections, sessionID)
		h.connCount.Add(-1)
	}
	h.mu.Unlock()

	log.Printf("[HUB] Connection removed: session=%s (total=%d)", sessionID, h.connCount.Load())
}

// GetConnection returns a connection by session ID.
func (h *Hub) GetConnection(sessionID string) (*Connection, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.connections[sessionID]
	return conn, ok
}

// Count returns the current connection count.
func (h *Hub) Count() int64 {
	return h.connCount.Load()
}

// CloseAll sends a CloseGoingAway frame to every connected client.
// Used during graceful shutdown to tell clients "reconnect to another instance."
// Returns the number of connections that were closed.
func (h *Hub) CloseAll() int {
	h.mu.RLock()
	conns := make([]*Connection, 0, len(h.connections))
	for _, c := range h.connections {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	closed := 0
	for _, c := range conns {
		msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down")
		deadline := time.Now().Add(2 * time.Second)
		if err := c.ws.WriteControl(websocket.CloseMessage, msg, deadline); err != nil {
			log.Printf("[HUB] CloseAll: failed to send close to session=%s: %v", c.SessionID, err)
		}
		closed++
	}

	return closed
}

// ================================================================
// Connection — a single WebSocket client
// ================================================================

// Connection wraps a single WebSocket connection.
type Connection struct {
	SessionID   string
	AuthSubject string // Authenticated user/service ID (empty if auth disabled)
	AuthScope   string // Token scope ("ws", "admin", etc.)
	ws          *websocket.Conn
	send        chan []byte // Outbound message buffer
	hub         *Hub
	stream      *redisStream.StreamClient
	cfg         *config.Config

	// Per-connection message rate limiter.
	// Tracks messages sent within a sliding window.
	// Without this, a single client can flood Redis with unlimited messages
	// once the WebSocket is established (HTTP rate limiter only covers upgrades).
	msgCount    atomic.Int64
	msgWindowAt time.Time // Start of current rate window
	msgMu       sync.Mutex
}

// ================================================================
// WebSocket Handler — upgrades HTTP to WebSocket
// ================================================================

// Handler creates the HTTP handler for the /ws endpoint.
func Handler(hub *Hub, stream *redisStream.StreamClient, cfg *config.Config) http.HandlerFunc {
	// Configure the WebSocket upgrader
	upgrader := websocket.Upgrader{
		ReadBufferSize:  cfg.WSReadBufferSize,
		WriteBufferSize: cfg.WSWriteBufferSize,
		// CheckOrigin is handled by our middleware — allow all here
		// (the OriginCheck middleware blocks bad origins BEFORE we get here)
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		// Error handler for upgrade failures
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			log.Printf("[WS] Upgrade failed: status=%d reason=%v", status, reason)
			http.Error(w, "WebSocket upgrade failed", status)
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Check capacity BEFORE upgrading
		if hub.Count() >= int64(cfg.WSMaxConnections) {
			log.Printf("[WS] Connection rejected: at capacity (%d/%d)", hub.Count(), cfg.WSMaxConnections)
			http.Error(w, "Service at capacity", http.StatusServiceUnavailable)
			return
		}

		// Upgrade HTTP → WebSocket
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrader.Error already logged it
			return
		}

		// Extract authenticated identity (set by auth middleware if enabled)
		authSubject := r.Header.Get("X-Auth-Subject")
		authScope := r.Header.Get("X-Auth-Scope")

		// Create session
		sessionID := uuid.New().String()
		conn := &Connection{
			SessionID:   sessionID,
			AuthSubject: authSubject,
			AuthScope:   authScope,
			ws:          ws,
			send:        make(chan []byte, 256), // Buffer up to 256 outbound messages
			hub:         hub,
			stream:      stream,
			cfg:         cfg,
		}

		// Register connection
		if !hub.Register(conn) {
			log.Printf("[WS] Registration failed: at capacity")
			ws.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "server at capacity"))
			ws.Close()
			return
		}

		// Send session ID (and auth info if available) to the client
		welcome := map[string]string{
			"type":       "session_init",
			"session_id": sessionID,
		}
		if authSubject != "" {
			welcome["auth_subject"] = authSubject
			welcome["auth_scope"] = authScope
		}
		welcomeJSON, _ := json.Marshal(welcome)
		ws.WriteMessage(websocket.TextMessage, welcomeJSON)

		log.Printf("[WS] Client connected: session=%s auth=%s scope=%s remote=%s",
			sessionID, authSubject, authScope, r.RemoteAddr)

		// Start read and write pumps as separate goroutines
		go conn.writePump()
		go conn.readPump()
	}
}

// ================================================================
// Read Pump — reads messages FROM the client
// ================================================================

// readPump reads messages from the WebSocket and publishes them to Redis.
// Runs in its own goroutine. One per connection.
func (c *Connection) readPump() {
	defer func() {
		c.hub.Unregister(c.SessionID)
		c.ws.Close()
	}()

	// Set read limits
	c.ws.SetReadLimit(c.cfg.WSMaxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(c.cfg.WSPongWait))

	// Reset deadline on every pong received
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(c.cfg.WSPongWait))
		return nil
	})

	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
			) {
				log.Printf("[WS] Unexpected close: session=%s err=%v", c.SessionID, err)
			} else {
				log.Printf("[WS] Connection closed normally: session=%s", c.SessionID)
			}
			break
		}

		log.Printf("[WS] Received %d bytes from session=%s", len(message), c.SessionID)

		// ---- Per-connection message rate limit ----
		// Limits messages per minute per WebSocket connection.
		// This is the main defense against a single client flooding Redis.
		// Configurable via WS_MSG_RATE_LIMIT (default: 60 msgs/min).
		if !c.checkMessageRate() {
			log.Printf("[WS] Message rate exceeded for session=%s (limit=%d/min)",
				c.SessionID, c.cfg.WSMsgRateLimit)
			c.sendError("rate_limited", "Too many messages. Slow down.")
			continue
		}

		// Parse the incoming message
		var incoming ClientMessage
		if err := json.Unmarshal(message, &incoming); err != nil {
			log.Printf("[WS] Invalid JSON from session=%s: %v", c.SessionID, err)
			c.sendError("invalid_json", "Message must be valid JSON")
			continue
		}

		// Validate required fields
		if incoming.Content == "" {
			c.sendError("empty_content", "Message content cannot be empty")
			continue
		}

		// ---- AI Defense Layer: Input Sanitization ----
		// Sanitize BEFORE anything enters the Redis pipeline.
		// This is the Gate — the first of two defense layers.
		// The Agent is the Brain — the second layer.

		// 1. Validate + sanitize content type (block content_type:"system" attacks)
		contentType := middleware.ValidateContentType(incoming.ContentType)

		// 2. Validate channel (block path traversal: "../admin")
		channel, channelOK := middleware.ValidateChannel(incoming.Channel)
		if !channelOK {
			c.sendError("invalid_channel", "Channel name contains invalid characters")
			continue
		}

		// 3. Validate sender fields (prevent log injection)
		senderID := middleware.ValidateSenderID(incoming.SenderID)
		senderPlatform := middleware.ValidateSenderPlatform(incoming.SenderPlatform)

		// 4. Sanitize content (strip dangerous HTML/JS for XSS prevention)
		sanitizedContent := middleware.SanitizeContent(incoming.Content)

		// 5. Sanitize metadata (limit keys, strip control chars, block reserved prefixes)
		sanitizedMeta := middleware.SanitizeMetadata(incoming.Metadata)

		// 6. Assess prompt injection risk (heuristic scanner)
		risk, triggers := middleware.AssessPromptInjectionRisk(sanitizedContent)
		if risk >= middleware.RiskHigh {
			log.Printf("[WS] HIGH prompt injection risk from session=%s: triggers=%v",
				c.SessionID, triggers)
		}

		// Embed risk assessment in metadata so the Agent can use it
		// for defense-in-depth decisions (reinforce system prompt, reject, etc.)
		if risk > middleware.RiskNone {
			if sanitizedMeta == nil {
				sanitizedMeta = make(map[string]string)
			}
			sanitizedMeta["_injection_risk"] = risk.String()
			sanitizedMeta["_injection_triggers"] = strings.Join(triggers, ",")
		}

		// Build the inbound message for Redis
		// Use authenticated identity if available, fall back to validated sender
		if c.AuthSubject != "" {
			senderID = c.AuthSubject // Authenticated identity takes priority
		}

		inbound := &redisStream.InboundMessage{
			MessageID:      uuid.New().String(),
			SessionID:      c.SessionID,
			Channel:        channel,
			SenderID:       senderID,
			SenderPlatform: senderPlatform,
			ContentType:    contentType,
			Content:        sanitizedContent,
			Metadata:       sanitizedMeta,
			Timestamp:      time.Now().Unix(),
		}

		// Publish to Redis Streams
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		streamID, err := c.stream.PublishInbound(ctx, inbound)
		cancel()

		if err != nil {
			log.Printf("[WS] Redis publish failed: session=%s err=%v", c.SessionID, err)
			c.sendError("publish_failed", "Message could not be queued")
			continue
		}

		log.Printf("[WS] Message published: session=%s msgID=%s streamID=%s",
			c.SessionID, inbound.MessageID, streamID)

		// Acknowledge to the client
		c.sendAck(inbound.MessageID, streamID)
	}
}

// ================================================================
// Write Pump — sends messages TO the client
// ================================================================

// writePump sends messages from the send channel to the WebSocket.
// Also sends periodic pings to keep the connection alive.
// Runs in its own goroutine. One per connection.
func (c *Connection) writePump() {
	ticker := time.NewTicker(c.cfg.WSPingInterval)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(c.cfg.WSWriteWait))
			if !ok {
				// Hub closed the channel
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.ws.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[WS] Write failed: session=%s err=%v", c.SessionID, err)
				return
			}

		case <-ticker.C:
			// Send ping to keep connection alive
			c.ws.SetWriteDeadline(time.Now().Add(c.cfg.WSWriteWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ================================================================
// Client Message Types
// ================================================================

// ClientMessage is the JSON structure clients send to the gateway.
type ClientMessage struct {
	Channel        string            `json:"channel"`
	SenderID       string            `json:"sender_id"`
	SenderPlatform string            `json:"sender_platform"`
	ContentType    string            `json:"content_type,omitempty"`
	Content        string            `json:"content"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// sendError sends a structured error to the client.
func (c *Connection) sendError(code, message string) {
	resp := map[string]string{
		"type":    "error",
		"code":    code,
		"message": message,
	}
	data, _ := json.Marshal(resp)
	select {
	case c.send <- data:
	default:
		log.Printf("[WS] Send buffer full for session=%s, dropping error", c.SessionID)
	}
}

// sendAck acknowledges a successfully queued message.
func (c *Connection) sendAck(messageID, streamID string) {
	resp := map[string]string{
		"type":       "ack",
		"message_id": messageID,
		"stream_id":  streamID,
	}
	data, _ := json.Marshal(resp)
	select {
	case c.send <- data:
	default:
		log.Printf("[WS] Send buffer full for session=%s, dropping ack", c.SessionID)
	}
}

// Send queues a message to be sent to the client.
// Used by the outbound reader to deliver messages from the Router/Agent.
func (c *Connection) Send(data []byte) bool {
	select {
	case c.send <- data:
		log.Printf("[WS] Queued %d bytes for session=%s (buffer=%d/256)",
			len(data), c.SessionID, len(c.send))
		return true
	default:
		log.Printf("[WS] Send buffer FULL for session=%s — dropping %d bytes",
			c.SessionID, len(data))
		return false
	}
}

// checkMessageRate enforces per-connection message rate limiting.
// Returns true if the message is allowed, false if rate exceeded.
// Uses a simple sliding window: resets count every minute.
func (c *Connection) checkMessageRate() bool {
	limit := c.cfg.WSMsgRateLimit
	if limit <= 0 {
		return true // Rate limiting disabled
	}

	c.msgMu.Lock()
	defer c.msgMu.Unlock()

	now := time.Now()
	if now.Sub(c.msgWindowAt) > 1*time.Minute {
		// Window expired — reset
		c.msgWindowAt = now
		c.msgCount.Store(1)
		return true
	}

	count := c.msgCount.Add(1)
	return count <= int64(limit)
}
