package handlers

import (
	"encoding/json"
	"net/http"

	"snorlx/backend/internal/websocket"
)

// SyncStatus reports the state of the caller's current or most recent sync. The browser reads it
// while a sync is shown, so progress and completion still arrive when the WebSocket is down.
func (h *Handler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	status := websocket.SyncStatus{}
	if h.wsHub != nil {
		status = h.wsHub.SyncStatusOf(user.ID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
