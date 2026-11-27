package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"snorlx/backend/internal/models"
	"snorlx/backend/internal/websocket"
)

func TestSyncStatus_ReportsTheCallersSync(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.wsHub = websocket.NewHub()
	h.wsHub.SendSyncStart(1, 4)
	h.wsHub.SendSyncProgress(1, 2, 4, "acme/api")
	h.wsHub.SendSyncStart(2, 99)

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/repositories/sync/status", nil), &models.User{ID: 1})
	rec := httptest.NewRecorder()
	h.SyncStatus(rec, req)

	var got websocket.SyncStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got != (websocket.SyncStatus{Running: true, Total: 4, Synced: 2, Current: "acme/api"}) {
		t.Fatalf("expected the caller's own progress, got %+v", got)
	}
}

func TestSyncStatus_RequiresAUser(t *testing.T) {
	h := newTestHandler(&mockStorage{})

	rec := httptest.NewRecorder()
	h.SyncStatus(rec, httptest.NewRequest(http.MethodGet, "/api/repositories/sync/status", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSyncRepositories_RefusedSyncDoesNotMarkTheUserAsSyncing(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.wsHub = websocket.NewHub()
	if _, _, ok := h.syncs.beginManual(1); !ok {
		t.Fatal("setup")
	}

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/repositories/sync", nil), &models.User{ID: 1, AccessToken: "tok"})
	rec := httptest.NewRecorder()
	h.SyncRepositories(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if got := h.wsHub.SyncStatusOf(1); got.Running {
		t.Fatalf("a refused request must not be reported as a running sync, got %+v", got)
	}
}
