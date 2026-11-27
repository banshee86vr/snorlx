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
	// autoSyncWorkers bounds how many users are synced in the background at once.
	autoSyncWorkers = 4
	// autoSyncPassTimeout bounds one user's background sync; the next one starts after the interval.
	autoSyncPassTimeout = 10 * time.Minute
	// scoreMaxAge is how old a score may get before an incremental sync recomputes it.
	scoreMaxAge = 7 * 24 * time.Hour
)

func workflowsETagKey(repoID int) string { return fmt.Sprintf("workflows:%d", repoID) }

// syncRun is one sync of one user, registered with the syncGuard.
type syncRun struct {
	manual bool
	cancel context.CancelFunc // set for background runs, so a manual sync can interrupt them
	done   chan struct{}      // closed when the run has ended
	once   sync.Once
}

// syncGuard allows one sync per user at a time. The Sync button wins over the background sync: it
// cancels a background pass that is running and starts once that pass has unwound, so the two
// never write at once and a background pass cannot report completion over a manual sync. A second
// manual sync is refused.
type syncGuard struct {
	mu      sync.Mutex
	running map[int]*syncRun
	started map[int]time.Time
}

func newSyncGuard() *syncGuard {
	return &syncGuard{running: make(map[int]*syncRun), started: make(map[int]time.Time)}
}

// beginManual registers a manual sync of userID. It returns false when another manual sync is
// running. A background pass is cancelled instead; wait is then closed once that pass has ended and
// the caller must not start syncing before that. wait is nil when nothing was running.
func (g *syncGuard) beginManual(userID int) (run *syncRun, wait <-chan struct{}, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if prev := g.running[userID]; prev != nil {
		if prev.manual {
			return nil, nil, false
		}
		prev.cancel()
		wait = prev.done
	}
	run = &syncRun{manual: true, done: make(chan struct{})}
	g.running[userID] = run
	g.started[userID] = time.Now()
	return run, wait, true
}

// beginIfDue registers a background sync unless one of userID runs or the last one started less
// than interval ago. The returned context is cancelled when a manual sync takes over.
func (g *syncGuard) beginIfDue(parent context.Context, userID int, interval time.Duration) (*syncRun, context.Context, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running[userID] != nil {
		return nil, nil, false
	}
	if at, ok := g.started[userID]; ok && time.Since(at) < interval {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	run := &syncRun{cancel: cancel, done: make(chan struct{})}
	g.running[userID] = run
	g.started[userID] = time.Now()
	return run, ctx, true
}

// active reports whether run is still the sync of userID, that is, no manual sync took over.
func (g *syncGuard) active(userID int, run *syncRun) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running[userID] == run
}

// withCurrent runs fn only while run is still the sync of userID. The lock is held across the
// check and fn so a beginManual takeover cannot land between them (for example between deciding
// to emit sync:complete and actually sending it).
func (g *syncGuard) withCurrent(userID int, run *syncRun, fn func()) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running[userID] != run {
		return false
	}
	fn()
	return true
}

// end unregisters run and releases whoever waits for it. The goroutine that registered the run
// calls it, once the run is over; calling it again does nothing.
func (g *syncGuard) end(userID int, run *syncRun) {
	g.mu.Lock()
	if g.running[userID] == run {
		delete(g.running, userID)
	}
	g.mu.Unlock()
	run.once.Do(func() {
		if run.cancel != nil {
			run.cancel()
		}
		close(run.done)
	})
}

// RunAutoSync syncs every watching user whose last sync started more than interval ago, until ctx
// is cancelled. Only users who already have synced repositories qualify, so nobody's account is
// imported without pressing Sync once. Each user is synced on its own goroutine under a time
// limit, so one slow GitHub request never holds up the others.
func (h *Handler) RunAutoSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(autoSyncCheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		h.autoSyncDueUsers(ctx, interval)
	}
}

// autoSyncDueUsers starts a background sync for every due watching user and returns a group that
// is done when they have all ended.
func (h *Handler) autoSyncDueUsers(ctx context.Context, interval time.Duration) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	sem := make(chan struct{}, autoSyncWorkers)
	for _, userID := range h.watchingUsers() {
		if ctx.Err() != nil {
			break
		}
		run, runCtx, ok := h.syncs.beginIfDue(ctx, userID, interval)
		if !ok {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer h.syncs.end(userID, run)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				return
			}
			runCtx, cancel := context.WithTimeout(runCtx, autoSyncPassTimeout)
			defer cancel()
			h.autoSyncUser(runCtx, userID, run)
		}()
	}
	return wg
}

// autoSyncUser runs the incremental sync of one user; the caller registered run and ends it.
func (h *Handler) autoSyncUser(ctx context.Context, userID int, run *syncRun) {
	user, err := h.storage.GetUserByID(ctx, userID)
	if err != nil || user == nil || user.AccessToken == "" {
		return
	}
	if _, total, err := h.storage.ListRepositories(ctx, userID, 1, 1, ""); err != nil || total == 0 {
		return
	}
	log.Debug().Int("user_id", userID).Msg("Automatic sync started")
	h.runSync(ctx, userID, user.AccessToken, true, run)
}

// announceComplete tells the browser the sync finished. A manual sync always announces. An
// incremental pass announces only when it stored something, was not cancelled, and is still the
// user's sync: the guard check and the WebSocket send run under the same lock, so a manual sync
// that takes over cannot land between them and have its progress overlay closed by this pass.
func (h *Handler) announceComplete(ctx context.Context, userID int, run *syncRun, incremental, changed bool, repos, workflows, runs int) {
	if !incremental {
		h.wsHub.SendSyncComplete(userID, repos, workflows, runs)
		return
	}
	if !changed || ctx.Err() != nil {
		return
	}
	h.syncs.withCurrent(userID, run, func() {
		h.wsHub.SendSyncComplete(userID, repos, workflows, runs)
	})
}

// scoreIsStale reports whether a repository has no score or an old one.
func (h *Handler) scoreIsStale(ctx context.Context, repoID int) bool {
	score, err := h.storage.GetLatestRepositoryScore(ctx, repoID)
	return err != nil || score == nil || time.Since(score.ScannedAt) > scoreMaxAge
}
