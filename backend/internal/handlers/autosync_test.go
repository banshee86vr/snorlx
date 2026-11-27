package handlers

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"snorlx/backend/internal/models"
)

func TestSyncGuard(t *testing.T) {
	g := newSyncGuard()

	if !g.startIfDue(1, time.Hour) {
		t.Fatal("a user that never synced is due")
	}
	if g.startIfDue(1, 0) {
		t.Fatal("a running sync blocks the background sync")
	}
	g.done(1)
	if g.startIfDue(1, time.Hour) {
		t.Fatal("a sync that started moments ago is not due again")
	}
	if !g.startIfDue(1, 0) {
		t.Fatal("a finished sync is due again once the interval passed")
	}
	g.done(1)

	g.start(2) // the Sync button
	if g.startIfDue(2, 0) {
		t.Fatal("the background sync must not overlap a manual sync")
	}
	g.done(2)
}

// autoSyncFixture serves one repository (acme/app) with one workflow and one run.
type autoSyncFixture struct {
	api           *fakeGitHubAPI
	h             *Handler
	known         atomic.Bool
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
		getUserByIDFunc: tokenUser(1),
		getRepoByGitHubIDFunc: func(context.Context, int64) (*models.Repository, error) {
			if f.known.Load() {
				return &models.Repository{ID: 7, GitHubID: 1}, nil
			}
			return nil, nil
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
	f.known.Store(true)

	// First pass: nothing cached yet, so the workflows are stored. Runs are the poller's job.
	f.h.syncs.start(1)
	f.h.runSync(context.Background(), 1, "tok", true)
	if f.workflowSaves.Load() != 1 {
		t.Fatalf("expected the workflow to be stored, got %d (requests %v)", f.workflowSaves.Load(), f.api.requestedPaths())
	}

	// Second pass: GitHub answers 304 for the workflow list, so nothing is written.
	f.h.syncs.start(1)
	f.h.runSync(context.Background(), 1, "tok", true)
	if f.workflowSaves.Load() != 1 {
		t.Fatalf("unchanged workflows must not be written again, got %d saves", f.workflowSaves.Load())
	}
	if f.runSaves.Load() != 0 || f.requestsTo("/actions/runs") != 0 {
		t.Fatalf("an incremental sync must leave runs to the live poller (saved %d, requests %v)",
			f.runSaves.Load(), f.api.requestedPaths())
	}
}

func TestRunSync_IncrementalSyncsNewRepositoriesInFull(t *testing.T) {
	f := newAutoSyncFixture(t) // known stays false: the repository is new to the instance

	f.h.syncs.start(1)
	f.h.runSync(context.Background(), 1, "tok", true)

	if f.workflowSaves.Load() != 1 || f.runSaves.Load() != 1 {
		t.Fatalf("a new repository gets its workflows and runs, stored %d and %d", f.workflowSaves.Load(), f.runSaves.Load())
	}
}

func TestAutoSync_OnlyForUsersWhoSyncedBefore(t *testing.T) {
	f := newAutoSyncFixture(t)
	f.known.Store(true)
	store := f.h.storage.(*mockStorage)
	ctx := context.Background()

	store.listRepositoriesFunc = func(context.Context, int, int, int, string) ([]models.Repository, int, error) {
		return nil, 0, nil
	}
	f.h.autoSyncUser(ctx, 1, time.Hour)
	if f.api.requests.Load() != 0 {
		t.Fatalf("a user without synced repositories must not be imported, saw %v", f.api.requestedPaths())
	}

	store.listRepositoriesFunc = func(context.Context, int, int, int, string) ([]models.Repository, int, error) {
		return []models.Repository{{ID: 7}}, 1, nil
	}
	f.h.autoSyncUser(ctx, 1, 0) // the check above already used up this interval
	synced := f.api.requests.Load()
	if synced == 0 {
		t.Fatal("a user who synced before must be synced")
	}
	f.h.autoSyncUser(ctx, 1, time.Hour)
	if f.api.requests.Load() != synced {
		t.Fatal("a user synced moments ago must not be synced again")
	}
}
