package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ===== Hub Creation =====

func TestNewHub_CreatesEmptyHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("expected non-nil hub")
	}
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients on new hub, got %d", hub.ClientCount())
	}
}

// ===== Client Registration =====

func TestRegisterAndUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Give the hub goroutine time to start
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "test-client",
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	// Wait for registration to be processed
	time.Sleep(10 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client after register, got %d", hub.ClientCount())
	}

	hub.Unregister(client)
	// Wait for unregistration to be processed
	time.Sleep(10 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.ClientCount())
	}
}

// ===== Broadcast =====

func TestBroadcast_DeliversToClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	msg := Message{Type: "test", Data: "hello"}
	hub.Broadcast(msg)
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var received Message
		if err := json.Unmarshal(data, &received); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}
		if received.Type != "test" {
			t.Errorf("expected type 'test', got %q", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected message to be received within 100ms")
	}

	hub.Unregister(client)
}

// ===== Broadcast helpers =====

func TestBroadcastWorkflowRunUpdate_SendsCorrectType(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "wf-receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastWorkflowRunUpdate(map[string]string{"id": "123"})
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var msg Message
		json.Unmarshal(data, &msg)
		if msg.Type != "workflow_run" {
			t.Errorf("expected type 'workflow_run', got %q", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected message to be delivered")
	}

	hub.Unregister(client)
}

func TestBroadcastSyncStart_SendsCorrectType(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "sync-receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastSyncStart(10)
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var msg Message
		json.Unmarshal(data, &msg)
		if msg.Type != "sync:start" {
			t.Errorf("expected type 'sync:start', got %q", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected sync:start message to be delivered")
	}

	hub.Unregister(client)
}

func TestBroadcastSyncProgress_ComputesPercentage(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "progress-receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastSyncProgress(5, 10, "my-repo")
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var msg struct {
			Type string `json:"type"`
			Data struct {
				Synced   float64 `json:"synced"`
				Total    float64 `json:"total"`
				Progress float64 `json:"progress"`
				Current  string  `json:"current"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if msg.Type != "sync:progress" {
			t.Errorf("expected type 'sync:progress', got %q", msg.Type)
		}
		if msg.Data.Progress != 50 {
			t.Errorf("expected progress 50%%, got %v", msg.Data.Progress)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected sync:progress message to be delivered")
	}

	hub.Unregister(client)
}

func TestBroadcastSyncComplete_SendsCorrectType(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "complete-receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastSyncComplete(5, 20, 100)
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var msg Message
		json.Unmarshal(data, &msg)
		if msg.Type != "sync:complete" {
			t.Errorf("expected type 'sync:complete', got %q", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected sync:complete message to be delivered")
	}

	hub.Unregister(client)
}

func TestBroadcastSyncError_SendsCorrectType(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "error-receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastSyncError("something went wrong")
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var msg Message
		json.Unmarshal(data, &msg)
		if msg.Type != "sync:error" {
			t.Errorf("expected type 'sync:error', got %q", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected sync:error message to be delivered")
	}

	hub.Unregister(client)
}

// ===== GetUpgraderWithOrigin =====

func TestGetUpgraderWithOrigin_AllowsSpecificOrigin(t *testing.T) {
	upgrader := GetUpgraderWithOrigin("https://myapp.example.com")

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://myapp.example.com")

	if !upgrader.CheckOrigin(req) {
		t.Error("expected specific origin to be allowed")
	}
}

func TestGetUpgraderWithOrigin_BlocksDifferentOrigin(t *testing.T) {
	upgrader := GetUpgraderWithOrigin("https://myapp.example.com")

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	if upgrader.CheckOrigin(req) {
		t.Error("expected different origin to be blocked")
	}
}

func TestGetUpgraderWithOrigin_EmptyAllowsAll(t *testing.T) {
	upgrader := GetUpgraderWithOrigin("")

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://any-origin.com")

	if !upgrader.CheckOrigin(req) {
		t.Error("expected any origin when allowed origin is empty")
	}
}

// ===== ClientCount =====

func TestClientCount_MultipleClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = &Client{
			ID:   "client-" + string(rune('A'+i)),
			hub:  hub,
			send: make(chan []byte, 10),
		}
		hub.Register(clients[i])
	}
	time.Sleep(20 * time.Millisecond)

	if hub.ClientCount() != 3 {
		t.Errorf("expected 3 clients, got %d", hub.ClientCount())
	}

	for _, c := range clients {
		hub.Unregister(c)
	}
	time.Sleep(20 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after all unregistered, got %d", hub.ClientCount())
	}
}

// ===== BroadcastSyncProgress edge cases =====

func TestBroadcastSyncProgress_ZeroTotal(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		ID:   "zero-total-receiver",
		hub:  hub,
		send: make(chan []byte, 10),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// Zero total should not cause a divide-by-zero
	hub.BroadcastSyncProgress(0, 0, "")
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var msg struct {
			Type string `json:"type"`
			Data struct {
				Progress float64 `json:"progress"`
			} `json:"data"`
		}
		json.Unmarshal(data, &msg)
		if msg.Data.Progress != 0 {
			t.Errorf("expected 0%% progress for zero total, got %v", msg.Data.Progress)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected message to be delivered")
	}

	hub.Unregister(client)
}
