package websocket

import "testing"

func TestSyncStatus_FollowsTheSyncEvents(t *testing.T) {
	h := NewHub()

	if got := h.SyncStatusOf(1); got.Running {
		t.Fatalf("an unknown user has no running sync, got %+v", got)
	}

	// The request was accepted but the sync has not announced itself yet.
	h.SyncQueued(1)
	if got := h.SyncStatusOf(1); !got.Running || got.Total != 0 {
		t.Fatalf("a queued sync must read as running, got %+v", got)
	}

	h.SendSyncStart(1, 4)
	h.SendSyncProgress(1, 2, 4, "acme/b")
	if got := h.SyncStatusOf(1); got != (SyncStatus{Running: true, Total: 4, Synced: 2, Current: "acme/b"}) {
		t.Fatalf("unexpected progress status %+v", got)
	}
	if got := h.SyncStatusOf(2); got.Running {
		t.Fatalf("another user's sync must not leak, got %+v", got)
	}

	h.SendSyncComplete(1, 4, 9, 40)
	if got := h.SyncStatusOf(1); got.Running || got.Synced != 4 || got.Error != "" {
		t.Fatalf("a completed sync must read as finished, got %+v", got)
	}
}

func TestSyncStatus_ErrorEndsTheSync(t *testing.T) {
	h := NewHub()
	h.SyncQueued(1)

	h.SendSyncError(1, "Failed to fetch repositories from GitHub")

	got := h.SyncStatusOf(1)
	if got.Running || got.Error != "Failed to fetch repositories from GitHub" {
		t.Fatalf("an error must end the sync and be reported, got %+v", got)
	}
}
