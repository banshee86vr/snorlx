package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// The live poller keeps the runs of synced repositories current, but new repositories, new
// workflows and scores only changed when a user pressed Sync. RunAutoSync does that part in the
// background: for users who are watching and synced before, it runs an incremental sync (see
// runSync) every AUTO_SYNC_INTERVAL.

const (
	// autoSyncCheckEvery is how often the loop looks for users whose sync is due.
	autoSyncCheckEvery = time.Minute
	// scoreMaxAge is how old a score may get before an incremental sync recomputes it.
	scoreMaxAge = 7 * 24 * time.Hour
)

func workflowsETagKey(repoID int) string { return fmt.Sprintf("workflows:%d", repoID) }

// syncGuard knows which users have a sync running and when each last started, so the background
// sync neither overlaps a sync of the same user nor repeats one that just ran.
type syncGuard struct {
	mu      sync.Mutex
	running map[int]int
	started map[int]time.Time
}

func newSyncGuard() *syncGuard {
	return &syncGuard{running: make(map[int]int), started: make(map[int]time.Time)}
}

// start registers a sync of userID. The Sync button always starts one: it never waits for the
// background sync.
func (g *syncGuard) start(userID int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running[userID]++
	g.started[userID] = time.Now()
}

// startIfDue registers a background sync unless one of userID runs or the last one started less
// than interval ago.
func (g *syncGuard) startIfDue(userID int, interval time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running[userID] > 0 {
		return false
	}
	if at, ok := g.started[userID]; ok && time.Since(at) < interval {
		return false
	}
	g.running[userID]++
	g.started[userID] = time.Now()
	return true
}

func (g *syncGuard) done(userID int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running[userID] > 0 {
		g.running[userID]--
	}
}

// RunAutoSync syncs every watching user whose last sync started more than interval ago, until ctx
// is cancelled. Only users who already have synced repositories qualify, so nobody's account is
// imported without pressing Sync once.
func (h *Handler) RunAutoSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(autoSyncCheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for _, userID := range h.watchingUsers() {
			if ctx.Err() != nil {
				return
			}
			h.autoSyncUser(ctx, userID, interval)
		}
	}
}

func (h *Handler) autoSyncUser(ctx context.Context, userID int, interval time.Duration) {
	if !h.syncs.startIfDue(userID, interval) {
		return
	}
	user, err := h.storage.GetUserByID(ctx, userID)
	if err != nil || user == nil || user.AccessToken == "" {
		h.syncs.done(userID)
		return
	}
	if _, total, err := h.storage.ListRepositories(ctx, userID, 1, 1, ""); err != nil || total == 0 {
		h.syncs.done(userID)
		return
	}
	log.Debug().Int("user_id", userID).Msg("Automatic sync started")
	h.runSync(ctx, userID, user.AccessToken, true)
}

// scoreIsStale reports whether a repository has no score or an old one.
func (h *Handler) scoreIsStale(ctx context.Context, repoID int) bool {
	score, err := h.storage.GetLatestRepositoryScore(ctx, repoID)
	return err != nil || score == nil || time.Since(score.ScannedAt) > scoreMaxAge
}
