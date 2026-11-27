package websocket

import "sync"

// SyncStatus is the latest known state of a user's repository sync. It mirrors the sync:* events
// the hub pushes, so a browser that missed them (socket down, blocked by a proxy) can still read
// the progress with a plain request.
type SyncStatus struct {
	Running bool   `json:"running"`
	Total   int    `json:"total"`
	Synced  int    `json:"synced"`
	Current string `json:"current"`
	// Error is set when the sync ended with sync:error.
	Error string `json:"error,omitempty"`
}

// syncStatuses keeps the status per user. The zero value is ready to use.
type syncStatuses struct {
	mu     sync.Mutex
	status map[int]SyncStatus
}

func (s *syncStatuses) set(userID int, status SyncStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == nil {
		s.status = make(map[int]SyncStatus)
	}
	s.status[userID] = status
}

func (s *syncStatuses) update(userID int, change func(*SyncStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == nil {
		s.status = make(map[int]SyncStatus)
	}
	st := s.status[userID]
	change(&st)
	s.status[userID] = st
}

func (s *syncStatuses) get(userID int) SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status[userID]
}

// SyncQueued records that a sync was requested for userID but has not announced itself yet: it
// first lists the user's repositories, which takes a moment. Without this a status read right
// after the request would look like "finished".
func (h *Hub) SyncQueued(userID int) {
	h.syncs.set(userID, SyncStatus{Running: true})
}

// SyncStatusOf returns the latest sync status of userID; the zero value means no sync is known.
func (h *Hub) SyncStatusOf(userID int) SyncStatus {
	return h.syncs.get(userID)
}
