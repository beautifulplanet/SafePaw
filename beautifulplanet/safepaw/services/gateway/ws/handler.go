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
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"nopenclaw/gateway/config"
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
func (h *Hub) Register(conn *Connection) bool {
	if int(h.connCount.Load()) >= h.maxConns {
		return false
	}

	h.mu.Lock()
	h.connections[conn.SessionID] = conn
	h.mu.Unlock()
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

// ================================================================
// Connection — a single WebSocket client
// ================================================================

// Connection wraps a single WebSocket connection.
type Connection struct {
	SessionID string
	ws        *websocket.Conn
	send      chan []byte // Outbound message buffer
	hub       *Hub
	stream    *redisStream.StreamClient
	cfg       *config.Config
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

		// Create session
		sessionID := uuid.New().String()
		conn := &Connection{
			SessionID: sessionID,
			ws:        ws,
			send:      make(chan []byte, 256), // Buffer up to 256 outbound messages
			hub:       hub,
			stream:    stream,
			cfg:       cfg,
		}

		// Register connection
		if !hub.Register(conn) {
			log.Printf("[WS] Registration failed: at capacity")
			ws.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "server at capacity"))
			ws.Close()
			return
		}

		// Send session ID to the client so they can identify themselves
		welcome := map[string]string{
			"type":       "session_init",
			"session_id": sessionID,
		}
		welcomeJSON, _ := json.Marshal(welcome)
		ws.WriteMessage(websocket.TextMessage, welcomeJSON)

		log.Printf("[WS] Client connected: session=%s remote=%s", sessionID, r.RemoteAddr)

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
			}
			break
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

		// Sanitize content type
		contentType := incoming.ContentType
		if contentType == "" {
			contentType = "TEXT"
		}

		// Build the inbound message for Redis
		inbound := &redisStream.InboundMessage{
			MessageID:      uuid.New().String(),
			SessionID:      c.SessionID,
			Channel:        incoming.Channel,
			SenderID:       incoming.SenderID,
			SenderPlatform: incoming.SenderPlatform,
			ContentType:    contentType,
			Content:        incoming.Content,
			Metadata:       incoming.Metadata,
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
		return true
	default:
		return false
	}
}
