package websocket

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// GetUpgraderWithOrigin returns a WebSocket upgrader that only accepts connections
// from the specified allowed origin. Pass an empty string to allow all origins (insecure).
func GetUpgraderWithOrigin(allowedOrigin string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if allowedOrigin == "" {
				return true
			}
			origin := r.Header.Get("Origin")
			return origin == allowedOrigin
		},
	}
}

// Message represents a WebSocket message
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Client represents a connected WebSocket client
type Client struct {
	ID     string
	UserID int
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
}

// Hub manages WebSocket client connections
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Debug().Str("client_id", client.ID).Msg("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Debug().Str("client_id", client.ID).Msg("WebSocket client disconnected")

		case message := <-h.broadcast:
			h.mu.RLock()
			data, err := json.Marshal(message)
			if err != nil {
				log.Error().Err(err).Msg("Failed to marshal WebSocket message")
				h.mu.RUnlock()
				continue
			}

			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(h.clients, client)
					log.Warn().Str("client_id", client.ID).Msg("WebSocket client buffer full, disconnecting")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Register registers a new client
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister unregisters a client
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(message Message) {
	h.broadcast <- message
}

// BroadcastWorkflowRunUpdate sends a workflow run update event
func (h *Hub) BroadcastWorkflowRunUpdate(run interface{}) {
	h.Broadcast(Message{
		Type: "workflow_run",
		Data: run,
	})
}

// BroadcastWorkflowJobUpdate sends a workflow job update event
func (h *Hub) BroadcastWorkflowJobUpdate(job interface{}) {
	h.Broadcast(Message{
		Type: "workflow_job",
		Data: job,
	})
}

// BroadcastDeploymentUpdate sends a deployment update event
func (h *Hub) BroadcastDeploymentUpdate(deployment interface{}) {
	h.Broadcast(Message{
		Type: "deployment",
		Data: deployment,
	})
}

// BroadcastSyncStart sends a sync start event
func (h *Hub) BroadcastSyncStart(total int) {
	h.Broadcast(Message{
		Type: "sync:start",
		Data: map[string]interface{}{
			"total": total,
		},
	})
}

// BroadcastSyncProgress sends a sync progress event
func (h *Hub) BroadcastSyncProgress(synced, total int, current string) {
	progress := 0
	if total > 0 {
		progress = (synced * 100) / total
	}
	h.Broadcast(Message{
		Type: "sync:progress",
		Data: map[string]interface{}{
			"synced":   synced,
			"total":    total,
			"current":  current,
			"progress": progress,
		},
	})
}

// BroadcastSyncComplete sends a sync complete event
func (h *Hub) BroadcastSyncComplete(repos, workflows, runs int) {
	h.Broadcast(Message{
		Type: "sync:complete",
		Data: map[string]interface{}{
			"repositories": repos,
			"workflows":    workflows,
			"runs":         runs,
		},
	})
}

// BroadcastSyncError sends a sync error event
func (h *Hub) BroadcastSyncError(message string) {
	h.Broadcast(Message{
		Type: "sync:error",
		Data: map[string]interface{}{
			"message": message,
		},
	})
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// NewClient creates a new WebSocket client
func NewClient(id string, userID int, hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		ID:     id,
		UserID: userID,
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("client_id", c.ID).Msg("WebSocket read error")
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			log.Debug().Str("client_id", c.ID).Str("type", msg.Type).Msg("Received WebSocket message")
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send each message as a separate WebSocket frame
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// GetUpgrader returns the WebSocket upgrader
func GetUpgrader() *websocket.Upgrader {
	return &upgrader
}



