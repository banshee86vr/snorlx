package websocket

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
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

// GetUpgraderWithOrigin returns a WebSocket upgrader that only accepts connections from the
// configured frontend origin. An empty allowed origin rejects every upgrade (fail closed).
func GetUpgraderWithOrigin(allowedOrigin string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if allowedOrigin == "" {
				return false
			}
			return r.Header.Get("Origin") == allowedOrigin
		},
	}
}

// Message represents a WebSocket message
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// presenceMessageType is the only message a client sends: whether its page is visible. The server
// polls GitHub only for users with at least one visible page.
const presenceMessageType = "presence"

type presencePayload struct {
	Active bool `json:"active"`
}

// envelope is a serialized message addressed to a set of users.
type envelope struct {
	payload    []byte
	recipients map[int]struct{}
}

// Client represents a connected WebSocket client
type Client struct {
	ID     string
	UserID int
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	// active is true while the client's page is visible. A new connection starts inactive and
	// counts as watching only once the browser reports a visible page.
	active atomic.Bool
	// reported is set after the first presence frame: this client tells the server about its
	// visibility, so its API traffic is not needed as a presence signal.
	reported atomic.Bool
}

// Hub manages WebSocket client connections. Every message is addressed to explicit user IDs so a
// user only receives events for repositories they may see.
type Hub struct {
	clients    map[*Client]bool
	outbox     chan envelope
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex

	activationMu sync.RWMutex
	onActivated  func(userID int)
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		outbox:     make(chan envelope, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// SetActivationListener registers fn, called whenever a page of a user reports that it became
// visible. fn must not block.
func (h *Hub) SetActivationListener(fn func(userID int)) {
	h.activationMu.Lock()
	defer h.activationMu.Unlock()
	h.onActivated = fn
}

func (h *Hub) notifyActivated(userID int) {
	h.activationMu.RLock()
	fn := h.onActivated
	h.activationMu.RUnlock()
	if fn != nil {
		fn(userID)
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

		case env := <-h.outbox:
			h.deliver(env)
		}
	}
}

// deliver fans an envelope out to the recipients. Clients whose buffer is full are disconnected
// after the read lock is released, so the client map is never written under RLock.
func (h *Hub) deliver(env envelope) {
	var slow []*Client

	h.mu.RLock()
	for client := range h.clients {
		if _, ok := env.recipients[client.UserID]; !ok {
			continue
		}
		select {
		case client.send <- env.payload:
		default:
			slow = append(slow, client)
		}
	}
	h.mu.RUnlock()

	if len(slow) == 0 {
		return
	}
	h.mu.Lock()
	for _, client := range slow {
		if _, ok := h.clients[client]; ok {
			delete(h.clients, client)
			close(client.send)
			log.Warn().Str("client_id", client.ID).Msg("WebSocket client buffer full, disconnecting")
		}
	}
	h.mu.Unlock()
}

// Register registers a new client
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister unregisters a client
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// SendToUsers delivers message to every connected client of the given users.
func (h *Hub) SendToUsers(userIDs []int, message Message) {
	if len(userIDs) == 0 {
		return
	}
	payload, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal WebSocket message")
		return
	}
	recipients := make(map[int]struct{}, len(userIDs))
	for _, id := range userIDs {
		recipients[id] = struct{}{}
	}
	h.outbox <- envelope{payload: payload, recipients: recipients}
}

// SendToUser delivers message to one user.
func (h *Hub) SendToUser(userID int, message Message) {
	h.SendToUsers([]int{userID}, message)
}

// SendWorkflowRunUpdate sends a workflow run update event to the users who may see the repository.
func (h *Hub) SendWorkflowRunUpdate(userIDs []int, run interface{}) {
	h.SendToUsers(userIDs, Message{Type: "workflow_run", Data: run})
}

// SendWorkflowJobUpdate sends a workflow job update event to the users who may see the repository.
func (h *Hub) SendWorkflowJobUpdate(userIDs []int, job interface{}) {
	h.SendToUsers(userIDs, Message{Type: "workflow_job", Data: job})
}

// SendDeploymentUpdate sends a deployment update event to the users who may see the repository.
func (h *Hub) SendDeploymentUpdate(userIDs []int, deployment interface{}) {
	h.SendToUsers(userIDs, Message{Type: "deployment", Data: deployment})
}

// SendSyncStart tells the user who started a sync how many repositories it covers.
func (h *Hub) SendSyncStart(userID, total int) {
	h.SendToUser(userID, Message{
		Type: "sync:start",
		Data: map[string]interface{}{"total": total},
	})
}

// SendSyncProgress reports sync progress to the user who started it.
func (h *Hub) SendSyncProgress(userID, synced, total int, current string) {
	progress := 0
	if total > 0 {
		progress = (synced * 100) / total
	}
	h.SendToUser(userID, Message{
		Type: "sync:progress",
		Data: map[string]interface{}{
			"synced":   synced,
			"total":    total,
			"current":  current,
			"progress": progress,
		},
	})
}

// SendSyncComplete reports the sync outcome to the user who started it.
func (h *Hub) SendSyncComplete(userID, repos, workflows, runs int) {
	h.SendToUser(userID, Message{
		Type: "sync:complete",
		Data: map[string]interface{}{
			"repositories": repos,
			"workflows":    workflows,
			"runs":         runs,
		},
	})
}

// SendSyncError reports a sync failure to the user who started it.
func (h *Hub) SendSyncError(userID int, message string) {
	h.SendToUser(userID, Message{
		Type: "sync:error",
		Data: map[string]interface{}{"message": message},
	})
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ActiveUserIDs returns the distinct users with at least one connection whose page is visible.
func (h *Hub) ActiveUserIDs() []int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[int]struct{}, len(h.clients))
	ids := make([]int, 0, len(h.clients))
	for client := range h.clients {
		if !client.active.Load() {
			continue
		}
		if _, ok := seen[client.UserID]; ok {
			continue
		}
		seen[client.UserID] = struct{}{}
		ids = append(ids, client.UserID)
	}
	return ids
}

// ReportsPresence reports whether userID has at least one open connection that has told the
// server about its page visibility. Such a user is tracked through presence frames, visible or
// not; a user without one (older client, API tool) is tracked through API activity instead.
func (h *Hub) ReportsPresence(userID int) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.UserID == userID && client.reported.Load() {
			return true
		}
	}
	return false
}

// NewClient creates a new WebSocket client. It starts inactive until the browser reports a
// visible page, so a background tab never counts as watching.
func NewClient(id string, userID int, hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		ID:     id,
		UserID: userID,
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
	}
}

// Active reports whether the client's page is visible.
func (c *Client) Active() bool {
	return c.active.Load()
}

// handleIncoming applies a message sent by the client. Only presence is understood; anything else
// is logged and ignored.
func (c *Client) handleIncoming(raw []byte) {
	var msg struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if msg.Type != presenceMessageType {
		log.Debug().Str("client_id", c.ID).Str("type", msg.Type).Msg("Ignoring WebSocket message")
		return
	}
	var presence presencePayload
	if err := json.Unmarshal(msg.Data, &presence); err != nil {
		return
	}
	c.ApplyPresence(presence.Active)
}

// ApplyPresence records whether the client's page is visible and tells the hub when it became
// visible (first report included).
func (c *Client) ApplyPresence(active bool) {
	c.reported.Store(true)
	wasActive := c.active.Swap(active)
	if active && !wasActive {
		c.hub.notifyActivated(c.UserID)
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
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
		c.handleIncoming(message)
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send each message as a separate WebSocket frame
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
