package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"snorlx/backend/internal/models"
)

// isRegistered and registered read the guard under its lock: passes end on their own goroutines.
func (g *syncGuard) isRegistered(userID int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running[userID] != nil
}

func (g *syncGuard) registered() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.running)
}

func TestSyncGuard_BackgroundRunsAreSpacedAndExclusive(t *testing.T) {
	g := newSyncGuard()
	ctx := context.Background()

	run, _, ok := g.beginIfDue(ctx, 1, time.Hour)
	if !ok {
		t.Fatal("a user that never synced is due")
	}
	if _, _, ok := g.beginIfDue(ctx, 1, 0); ok {
		t.Fatal("a running sync blocks the background sync")
	}
	g.end(1, run)
	if _, _, ok := g.beginIfDue(ctx, 1, time.Hour); ok {
		t.Fatal("a sync that started moments ago is not due again")
	}
	if run2, _, ok := g.beginIfDue(ctx, 1, 0); !ok {
		t.Fatal("a finished sync is due again once the interval passed")
	} else {
		g.end(1, run2)
	}
}

func TestSyncGuard_ManualSyncTakesOverFromBackgroundSync(t *testing.T) {
	g := newSyncGuard()
	bgRun, bgCtx, ok := g.beginIfDue(context.Background(), 1, 0)
	if !ok {
		t.Fatal("setup: background sync must start")
	}

	manual, wait, ok := g.beginManual(1)
	if !ok || wait == nil {
		t.Fatalf("a manual sync must start and wait for the background pass, ok=%v wait=%v", ok, wait)
	}
	if bgCtx.Err() == nil {
		t.Fatal("the background pass must be cancelled")
	}
	if g.active(1, bgRun) || !g.active(1, manual) {
		t.Fatal("the manual sync must be the active one")
	}
	select {
	case <-wait:
		t.Fatal("the manual sync must wait until the background pass has ended")
	default:
	}

	g.end(1, bgRun) // the interrupted pass unwinds
	select {
	case <-wait:
	default:
		t.Fatal("the manual sync must be released when the background pass ended")
	}
	if !g.active(1, manual) {
		t.Fatal("ending the old pass must not unregister the manual sync")
	}

	if _, _, ok := g.beginManual(1); ok {
		t.Fatal("a second manual sync must be refused while one runs")
	}
	g.end(1, manual)
	if _, _, ok := g.beginManual(1); !ok {
		t.Fatal("a manual sync may start once the previous one ended")
	}
}

func TestSyncGuard_UsersAreIndependent(t *testing.T) {
	g := newSyncGuard()
	if _, _, ok := g.beginManual(1); !ok {
		t.Fatal("setup")
	}
	if _, _, ok := g.beginManual(2); !ok {
		t.Fatal("another user's sync must not be blocked")
	}
}

func TestMayAnnounce(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	bg, bgCtx, _ := h.syncs.beginIfDue(context.Background(), 1, 0)

	if !h.mayAnnounce(bgCtx, 1, bg, true) {
		t.Error("an undisturbed pass that stored something may announce it")
	}
	if h.mayAnnounce(bgCtx, 1, bg, false) {
		t.Error("a pass that stored nothing stays silent")
	}

	manual, _, _ := h.syncs.beginManual(1)
	if h.mayAnnounce(context.Background(), 1, bg, true) {
		t.Error("a pass taken over by a manual sync must not announce completion over it")
	}
	if bgCtx.Err() == nil || h.mayAnnounce(bgCtx, 1, bg, true) {
		t.Error("a cancelled pass must not announce")
	}
	_ = manual
}

func TestSyncRepositories_RefusesASecondManualSync(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	if _, _, ok := h.syncs.beginManual(1); !ok {
		t.Fatal("setup")
	}

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/repositories/sync", nil), &models.User{ID: 1, AccessToken: "tok"})
	rec := httptest.NewRecorder()
	h.SyncRepositories(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 while a manual sync runs, got %d", rec.Code)
	}
}

func TestScoreIsStale(t *testing.T) {
	score := func(age time.Duration) func(context.Context, int) (*models.RepositoryScore, error) {
		return func(context.Context, int) (*models.RepositoryScore, error) {
			return &models.RepositoryScore{ScannedAt: time.Now().Add(-age)}, nil
		}
	}
	cases := []struct {
		name string
		fn   func(context.Context, int) (*models.RepositoryScore, error)
		want bool
	}{
		{name: "never scored", fn: nil, want: true},
		{name: "fresh", fn: score(time.Hour), want: false},
		{name: "just inside the limit", fn: score(scoreMaxAge - time.Hour), want: false},
		{name: "older than the limit", fn: score(scoreMaxAge + time.Hour), want: true},
		{name: "lookup failed", fn: func(context.Context, int) (*models.RepositoryScore, error) { return nil, context.Canceled }, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(&mockStorage{getLatestScoreFunc: tc.fn})
			if got := h.scoreIsStale(context.Background(), 7); got != tc.want {
				t.Errorf("scoreIsStale = %v, want %v", got, tc.want)
			}
		})
	}
}

// autoSyncFixture serves one repository (acme/app) with one workflow and one run.
type autoSyncFixture struct {
	api           *fakeGitHubAPI
	h             *Handler
	stored        atomic.Bool // the repository exists in storage (synced by anybody)
	access        atomic.Bool // user 1 holds it
	workflowSaves atomic.Int32
	runSaves      atomic.Int32
}

const userReposPath = "user/repos?affiliation=owner%2Ccollaborator%2Corganization_member&per_page=100&visibility=all"

func newAutoSyncFixture(t *testing.T) *autoSyncFixture {
	t.Helper()
	f := &autoSyncFixture{api: newFakeGitHubAPI(t)}
	f.api.serve(userReposPath, `W/"repos-1"`, []map[string]interface{}{
		{"id": 1, "name": "app", "full_name": "acme/app", "owner": map[string]interface{}{"login": "acme"}},
	})
	f.api.serve("repos/acme/app/actions/workflows?per_page=100", `W/"wf-1"`, map[string]interface{}{
		"total_count": 1,
		"workflows":   []map[string]interface{}{{"id": 30, "name": "CI", "path": ".github/workflows/ci.yml"}},
	})
	f.api.serve("repos/acme/app/actions/runs?per_page=50", `W/"runs-1"`, map[string]interface{}{
		"total_count":   1,
		"workflow_runs": []map[string]interface{}{{"id": 600, "workflow_id": 30, "status": "completed", "name": "CI"}},
	})

	store := &mockStorage{
		getUserByIDFunc: tokenUsers(1, 2),
		getRepoByGitHubIDFunc: func(context.Context, int64) (*models.Repository, error) {
			if f.stored.Load() {
				return &models.Repository{ID: 7, GitHubID: 1}, nil
			}
			return nil, nil
		},
		hasRepoAccessFunc: func(_ context.Context, userID, _ int) (bool, error) {
			return userID == 1 && f.access.Load(), nil
		},
		upsertWorkflowFunc: func(_ context.Context, w *models.Workflow) (*models.Workflow, error) {
			f.workflowSaves.Add(1)
			return w, nil
		},
		upsertRunFunc: func(_ context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			f.runSaves.Add(1)
			return run, nil
		},
		listRepositoriesFunc: func(context.Context, int, int, int, string) ([]models.Repository, int, error) {
			return []models.Repository{{ID: 7, FullName: "acme/app"}}, 1, nil
		},
	}
	f.h = newPollerHandler(t, store, f.api)
	return f
}

// sync runs one sync of user 1 to completion, registered with the guard like the real callers do.
func (f *autoSyncFixture) sync(incremental bool) {
	if incremental {
		run, ctx, ok := f.h.syncs.beginIfDue(context.Background(), 1, 0)
		if !ok {
			panic("background sync not due")
		}
		defer f.h.syncs.end(1, run)
		f.h.runSync(ctx, 1, "tok", true, run)
		return
	}
	run, _, ok := f.h.syncs.beginManual(1)
	if !ok {
		panic("manual sync refused")
	}
	defer f.h.syncs.end(1, run)
	f.h.runSync(context.Background(), 1, "tok", false, run)
}

func (f *autoSyncFixture) requestsTo(substr string) int {
	n := 0
	for _, p := range f.api.requestedPaths() {
		if strings.Contains(p, substr) {
			n++
		}
	}
	return n
}

func TestRunSync_IncrementalSkipsUnchangedRepositories(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.stored.Store(true)
	f.access.Store(true)

	// First pass: nothing cached yet, so the workflows are stored. Runs are the poller's job.
	f.sync(true)
	if f.workflowSaves.Load() != 1 {
		t.Fatalf("expected the workflow to be stored, got %d (requests %v)", f.workflowSaves.Load(), f.api.requestedPaths())
	}

	// Second pass: GitHub answers 304 for the workflow list, so nothing is written.
	f.sync(true)
	if f.workflowSaves.Load() != 1 {
		t.Fatalf("unchanged workflows must not be written again, got %d saves", f.workflowSaves.Load())
	}
	if f.runSaves.Load() != 0 || f.requestsTo("/actions/runs") != 0 {
		t.Fatalf("an incremental sync must leave runs to the live poller (saved %d, requests %v)",
			f.runSaves.Load(), f.api.requestedPaths())
	}
}

func TestRunSync_IncrementalSyncsNewRepositoriesInFull(t *testing.T) {
	f := newAutoSyncFixture(t) // the repository is new to the instance

	f.sync(true)

	if f.workflowSaves.Load() != 1 || f.runSaves.Load() != 1 {
		t.Fatalf("a new repository gets its workflows and runs, stored %d and %d", f.workflowSaves.Load(), f.runSaves.Load())
	}
}

func TestRunSync_RepositoryKnownToOthersIsNewToThisUser(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.stored.Store(true) // another user synced it; user 1 holds no grant yet

	// Even with warm ETags from the other user's passes, this user's first pass imports it in full
	// instead of answering 304 and leaving their views without it.
	f.h.etags.set(workflowsETagKey(0), `W/"wf-1"`)
	f.sync(true)

	if f.workflowSaves.Load() != 1 || f.runSaves.Load() != 1 {
		t.Fatalf("a repository new to the user must be synced in full, stored %d workflows and %d runs",
			f.workflowSaves.Load(), f.runSaves.Load())
	}
}

func TestRunSync_ManualSyncAlwaysRefetches(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.stored.Store(true)
	f.access.Store(true)

	f.sync(false)
	f.sync(false)

	if f.workflowSaves.Load() != 2 || f.runSaves.Load() != 2 {
		t.Fatalf("a manual sync is a full pass every time, stored %d workflows and %d runs",
			f.workflowSaves.Load(), f.runSaves.Load())
	}
}

func TestAutoSync_OnlyForUsersWhoSyncedBefore(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.stored.Store(true)
	f.access.Store(true)
	store := f.h.storage.(*mockStorage)
	ctx := context.Background()

	store.listRepositoriesFunc = func(context.Context, int, int, int, string) ([]models.Repository, int, error) {
		return nil, 0, nil
	}
	f.h.watchers.touch(1)
	f.h.autoSyncDueUsers(ctx, 0).Wait()
	if f.api.requests.Load() != 0 {
		t.Fatalf("a user without synced repositories must not be imported, saw %v", f.api.requestedPaths())
	}

	store.listRepositoriesFunc = func(context.Context, int, int, int, string) ([]models.Repository, int, error) {
		return []models.Repository{{ID: 7}}, 1, nil
	}
	f.h.autoSyncDueUsers(ctx, 0).Wait()
	synced := f.api.requests.Load()
	if synced == 0 {
		t.Fatal("a user who synced before must be synced")
	}
	f.h.autoSyncDueUsers(ctx, time.Hour).Wait()
	if f.api.requests.Load() != synced {
		t.Fatal("a user synced moments ago must not be synced again")
	}
}

func TestAutoSync_ASlowUserDoesNotHoldUpTheOthers(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.stored.Store(true)
	store := f.h.storage.(*mockStorage)
	release := make(chan struct{})
	lookup := tokenUsers(1, 2)
	store.getUserByIDFunc = func(ctx context.Context, id int) (*models.User, error) {
		if id == 1 {
			select { // user 1's pass hangs until released
			case <-release:
			case <-ctx.Done():
			}
		}
		return lookup(ctx, id)
	}
	f.h.watchers.touch(1)
	f.h.watchers.touch(2)

	wg := f.h.autoSyncDueUsers(context.Background(), 0)

	deadline := time.Now().Add(5 * time.Second)
	for f.api.requests.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("user 2 was not synced while user 1's pass hangs")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !f.h.syncs.isRegistered(1) {
		t.Fatal("the slow user's pass should still be registered")
	}
	close(release)
	wg.Wait()
	if n := f.h.syncs.registered(); n != 0 {
		t.Fatalf("every pass must unregister itself, %d left", n)
	}
}

func TestAutoSync_PassIsCancelledByAManualSync(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.stored.Store(true)
	store := f.h.storage.(*mockStorage)
	inPass := make(chan struct{})
	lookup := tokenUsers(1, 2)
	store.getUserByIDFunc = func(ctx context.Context, id int) (*models.User, error) {
		close(inPass)
		<-ctx.Done() // hangs until the pass is cancelled
		return lookup(ctx, id)
	}
	f.h.watchers.touch(1)

	wg := f.h.autoSyncDueUsers(context.Background(), 0)
	<-inPass
	_, wait, ok := f.h.syncs.beginManual(1)
	if !ok || wait == nil {
		t.Fatalf("a manual sync must be accepted and told to wait, ok=%v wait=%v", ok, wait)
	}
	select {
	case <-wait:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled background pass never unwound")
	}
	wg.Wait()
}
