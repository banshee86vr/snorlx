package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/github"
	"snorlx/backend/internal/models"
	"snorlx/backend/internal/websocket"
)

// ===== Fake GitHub =====

// fakeGitHubAPI serves GitHub Enterprise style paths (/api/v3/...) with one ETag per path and
// honours If-None-Match. Paths without an entry answer with the configured status (404 by default).
type fakeGitHubAPI struct {
	server   *httptest.Server
	mu       sync.Mutex
	etags    map[string]string
	bodies   map[string]interface{}
	failWith map[string]int
	// deniedTokens answer 404 to every request, like GitHub does for a private repository the
	// token can no longer see.
	deniedTokens map[string]bool
	requests     atomic.Int32
	paths        []string
	tokens       []string
}

func newFakeGitHubAPI(t *testing.T) *fakeGitHubAPI {
	t.Helper()
	f := &fakeGitHubAPI{
		etags:        map[string]string{},
		bodies:       map[string]interface{}{},
		failWith:     map[string]int{},
		deniedTokens: map[string]bool{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		f.mu.Lock()
		f.paths = append(f.paths, key)
		f.tokens = append(f.tokens, token)
		denied := f.deniedTokens[token]
		status, failing := f.failWith[key]
		etag, ok := f.etags[key]
		body := f.bodies[key]
		f.mu.Unlock()

		if denied {
			http.NotFound(w, r)
			return
		}
		if failing {
			if status == http.StatusForbidden {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Limit", "5000")
				w.Header().Set("X-RateLimit-Reset", "9999999999")
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"failed"}`))
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHubAPI) serve(path, etag string, body interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.etags["/api/v3/"+path] = etag
	f.bodies["/api/v3/"+path] = body
}

func (f *fakeGitHubAPI) fail(path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failWith["/api/v3/"+path] = status
}

func (f *fakeGitHubAPI) requestedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

func (f *fakeGitHubAPI) usedTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tokens...)
}

func (f *fakeGitHubAPI) denyToken(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deniedTokens[token] = true
}

// newPollerHandler wires a handler to the fake GitHub API and a running WebSocket hub.
func newPollerHandler(t *testing.T, store *mockStorage, api *fakeGitHubAPI) *Handler {
	t.Helper()
	h := newTestHandler(store)
	ghClient, err := github.NewClient(&config.Config{
		GitHubClientID:     "test-id",
		GitHubClientSecret: "test-secret",
		GitHubBaseURL:      api.server.URL,
	})
	if err != nil {
		t.Fatalf("github.NewClient: %v", err)
	}
	h.ghClient = ghClient
	h.wsHub = websocket.NewHub()
	go h.wsHub.Run()
	return h
}

func tokenUser(id int) func(ctx context.Context, userID int) (*models.User, error) {
	return func(ctx context.Context, userID int) (*models.User, error) {
		if userID != id {
			return nil, context.Canceled
		}
		return &models.User{ID: id, Login: "octocat", AccessToken: "gho_token"}, nil
	}
}

// tokenUsers gives every listed user its own token, "gho_<id>".
func tokenUsers(ids ...int) func(ctx context.Context, userID int) (*models.User, error) {
	return func(ctx context.Context, userID int) (*models.User, error) {
		for _, id := range ids {
			if id == userID {
				return &models.User{ID: id, AccessToken: fmt.Sprintf("gho_%d", id)}, nil
			}
		}
		return nil, context.Canceled
	}
}

func activeRun(id int, githubID int64) models.WorkflowRun {
	return models.WorkflowRun{
		ID:           id,
		GitHubID:     githubID,
		RepoID:       7,
		WorkflowID:   3,
		Status:       "in_progress",
		IsDeployment: true,
		StartedAt:    time.Now().Add(-time.Minute),
		Repository:   &models.Repository{FullName: "acme/app"},
	}
}

// ===== Active pass =====

func TestRefreshActiveRuns_StoresCompletionAndJobs(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs/500", `W/"run-1"`, map[string]interface{}{
		"id": 500, "workflow_id": 30, "status": "completed", "conclusion": "success",
		"run_started_at": "2026-10-05T10:00:00Z", "updated_at": "2026-10-05T10:05:00Z",
		"name": "CI", "head_branch": "main",
	})
	api.serve("repos/acme/app/actions/runs/500/jobs?per_page=100", `W/"jobs-1"`, map[string]interface{}{
		"total_count": 1,
		"jobs":        []map[string]interface{}{{"id": 9001, "run_id": 500, "name": "build", "status": "completed", "conclusion": "success"}},
	})

	var savedRun *models.WorkflowRun
	var savedJobs []models.WorkflowJob
	viewersAsked := atomic.Int32{}
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: tokenUser(1),
		upsertRunFunc: func(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			savedRun = run
			run.ID = 11
			return run, nil
		},
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			savedJobs = append(savedJobs, *job)
			return job, nil
		},
		listUsersWithRepoAccessFunc: func(ctx context.Context, repoID int) ([]int, error) {
			viewersAsked.Add(1)
			return []int{1}, nil
		},
	}
	h := newPollerHandler(t, store, api)

	h.refreshActiveRuns(context.Background(), []int{1})

	if savedRun == nil {
		t.Fatal("expected the completed run to be stored")
	}
	if savedRun.Status != "completed" || savedRun.Conclusion == nil || *savedRun.Conclusion != "success" {
		t.Errorf("stored run state = %s/%v", savedRun.Status, savedRun.Conclusion)
	}
	if savedRun.RepoID != 7 || savedRun.WorkflowID != 3 || !savedRun.IsDeployment {
		t.Errorf("stored run must keep internal ids and classification, got repo=%d workflow=%d deployment=%v", savedRun.RepoID, savedRun.WorkflowID, savedRun.IsDeployment)
	}
	if savedRun.CompletedAt == nil || savedRun.DurationSeconds == nil || *savedRun.DurationSeconds != 300 {
		t.Errorf("completed run must carry completion fields, got %v / %v", savedRun.CompletedAt, savedRun.DurationSeconds)
	}
	if len(savedJobs) != 1 || savedJobs[0].RunID != 11 || savedJobs[0].GitHubID != 9001 {
		t.Fatalf("expected one job bound to run 11, got %+v", savedJobs)
	}
	if viewersAsked.Load() != 2 {
		t.Errorf("expected viewers to be resolved for the run and the jobs event, got %d", viewersAsked.Load())
	}
	if h.etags.get(runETagKey(500)) != `W/"run-1"` || h.etags.get(jobsETagKey(500)) != `W/"jobs-1"` {
		t.Errorf("etags were not remembered: run=%q jobs=%q", h.etags.get(runETagKey(500)), h.etags.get(jobsETagKey(500)))
	}
}

func TestRefreshActiveRuns_NotModifiedLeavesStorageUntouched(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs/500", `W/"run-1"`, map[string]interface{}{"id": 500, "status": "in_progress"})
	api.serve("repos/acme/app/actions/runs/500/jobs?per_page=100", `W/"jobs-1"`, map[string]interface{}{"total_count": 0, "jobs": []interface{}{}})

	upserts := atomic.Int32{}
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: tokenUser(1),
		upsertRunFunc: func(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			upserts.Add(1)
			return run, nil
		},
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			upserts.Add(1)
			return job, nil
		},
	}
	h := newPollerHandler(t, store, api)
	h.etags.set(runETagKey(500), `W/"run-1"`)
	h.etags.set(jobsETagKey(500), `W/"jobs-1"`)

	h.refreshActiveRuns(context.Background(), []int{1})

	if upserts.Load() != 0 {
		t.Errorf("a 304 must not write to storage, got %d upserts", upserts.Load())
	}
	if api.requests.Load() != 2 {
		t.Errorf("expected one conditional request for the run and one for the jobs, got %d", api.requests.Load())
	}
}

func TestRefreshActiveRuns_UnchangedStateSendsNoEvent(t *testing.T) {
	api := newFakeGitHubAPI(t)
	// GitHub answers 200 (new ETag, e.g. updated_at moved) but the run is still in progress.
	api.serve("repos/acme/app/actions/runs/500", `W/"run-2"`, map[string]interface{}{"id": 500, "status": "in_progress"})
	api.serve("repos/acme/app/actions/runs/500/jobs?per_page=100", `W/"jobs-1"`, map[string]interface{}{"total_count": 0, "jobs": []interface{}{}})

	viewersAsked := atomic.Int32{}
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: tokenUser(1),
		listUsersWithRepoAccessFunc: func(ctx context.Context, repoID int) ([]int, error) {
			viewersAsked.Add(1)
			return []int{1}, nil
		},
	}
	h := newPollerHandler(t, store, api)
	h.etags.set(runETagKey(500), `W/"run-1"`)

	h.refreshActiveRuns(context.Background(), []int{1})

	if viewersAsked.Load() != 0 {
		t.Errorf("no WebSocket event expected when status and conclusion did not change, viewers resolved %d times", viewersAsked.Load())
	}
	if h.etags.get(runETagKey(500)) != `W/"run-2"` {
		t.Errorf("new etag must replace the stale one, got %q", h.etags.get(runETagKey(500)))
	}
}

func TestRefreshActiveRuns_FailedSaveDoesNotCacheETag(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs/500", `W/"run-1"`, map[string]interface{}{"id": 500, "status": "completed", "conclusion": "success"})
	api.serve("repos/acme/app/actions/runs/500/jobs?per_page=100", `W/"jobs-1"`, map[string]interface{}{
		"total_count": 1,
		"jobs":        []map[string]interface{}{{"id": 9001, "run_id": 500, "name": "build", "status": "completed"}},
	})

	failRun := true
	failJob := true
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: tokenUser(1),
		upsertRunFunc: func(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			if failRun {
				return nil, context.DeadlineExceeded
			}
			return run, nil
		},
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			if failJob {
				return nil, context.DeadlineExceeded
			}
			return job, nil
		},
	}
	h := newPollerHandler(t, store, api)

	// Run save fails: nothing is remembered, the jobs are not even attempted.
	h.refreshActiveRuns(context.Background(), []int{1})
	if h.etags.get(runETagKey(500)) != "" || h.etags.get(jobsETagKey(500)) != "" {
		t.Fatalf("a failed save must not remember an ETag, got run=%q jobs=%q", h.etags.get(runETagKey(500)), h.etags.get(jobsETagKey(500)))
	}

	// Run save works, job save fails: only the run ETag is remembered.
	failRun = false
	h.refreshActiveRuns(context.Background(), []int{1})
	if h.etags.get(runETagKey(500)) != `W/"run-1"` {
		t.Errorf("run ETag must be remembered after a successful save, got %q", h.etags.get(runETagKey(500)))
	}
	if h.etags.get(jobsETagKey(500)) != "" {
		t.Errorf("jobs ETag must not be remembered while a job save fails, got %q", h.etags.get(jobsETagKey(500)))
	}

	// Everything works: the next pass re-reads the jobs (no 304) and remembers their ETag.
	failJob = false
	h.refreshActiveRuns(context.Background(), []int{1})
	if h.etags.get(jobsETagKey(500)) != `W/"jobs-1"` {
		t.Errorf("jobs ETag must be remembered once the jobs are stored, got %q", h.etags.get(jobsETagKey(500)))
	}
}

func TestRefreshActiveRuns_MultiPageJobsStayUnconditional(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs/500", `W/"run-1"`, map[string]interface{}{"id": 500, "status": "in_progress"})
	// total_count exceeds the page: later pages may change while the first one does not.
	api.serve("repos/acme/app/actions/runs/500/jobs?per_page=100", `W/"jobs-p1"`, map[string]interface{}{
		"total_count": 2,
		"jobs":        []map[string]interface{}{{"id": 9001, "run_id": 500, "name": "build", "status": "in_progress"}},
	})

	jobSaves := atomic.Int32{}
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: tokenUser(1),
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			jobSaves.Add(1)
			return job, nil
		},
	}
	h := newPollerHandler(t, store, api)
	h.etags.set(jobsETagKey(500), `W/"stale-single-page"`)

	h.refreshActiveRuns(context.Background(), []int{1})
	h.refreshActiveRuns(context.Background(), []int{1})

	if h.etags.get(jobsETagKey(500)) != "" {
		t.Errorf("a multi-page job list must not keep an ETag, got %q", h.etags.get(jobsETagKey(500)))
	}
	if jobSaves.Load() != 2 {
		t.Errorf("jobs of a multi-page run must be re-read on every pass, got %d saves over two passes", jobSaves.Load())
	}
}

func TestRefreshActiveRuns_PrunesETagsOfFinishedRuns(t *testing.T) {
	api := newFakeGitHubAPI(t)
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return nil, nil
		},
	}
	h := newPollerHandler(t, store, api)
	h.etags.set(runETagKey(1), `W/"old"`)
	h.etags.set(jobsETagKey(1), `W/"old"`)
	h.etags.set(repoETagKey(7), `W/"repo"`)

	h.refreshActiveRuns(context.Background(), []int{1})

	if h.etags.get(runETagKey(1)) != "" || h.etags.get(jobsETagKey(1)) != "" {
		t.Error("etags of runs that are no longer active must be dropped")
	}
	if h.etags.get(repoETagKey(7)) != `W/"repo"` {
		t.Error("repository etags must survive the active pass")
	}
	if api.requests.Load() != 0 {
		t.Errorf("no active runs means no GitHub requests, got %d", api.requests.Load())
	}
}

func TestRefreshActiveRuns_SkipsUsersWithoutToken(t *testing.T) {
	api := newFakeGitHubAPI(t)
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: func(ctx context.Context, id int) (*models.User, error) {
			return &models.User{ID: id, AccessToken: ""}, nil
		},
	}
	h := newPollerHandler(t, store, api)

	h.refreshActiveRuns(context.Background(), []int{1})

	if api.requests.Load() != 0 {
		t.Errorf("a user without a stored token must not reach GitHub, got %d requests", api.requests.Load())
	}
}

func TestRefreshActiveRuns_FallsBackToAnotherWatcherWhenAccessWasRevoked(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs/500", `W/"run-1"`, map[string]interface{}{"id": 500, "status": "completed", "conclusion": "success"})
	api.serve("repos/acme/app/actions/runs/500/jobs?per_page=100", `W/"jobs-1"`, map[string]interface{}{"total_count": 0, "jobs": []interface{}{}})
	// User 1 synced the repository once but GitHub no longer lets that token see it.
	api.denyToken("gho_1")

	var savedRun *models.WorkflowRun
	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500)}, nil
		},
		getUserByIDFunc: tokenUsers(1, 2),
		upsertRunFunc: func(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			savedRun = run
			return run, nil
		},
		listUsersWithRepoAccessFunc: func(ctx context.Context, repoID int) ([]int, error) {
			return []int{1, 2}, nil
		},
	}
	h := newPollerHandler(t, store, api)

	h.refreshActiveRuns(context.Background(), []int{1, 2})

	if savedRun == nil || savedRun.Status != "completed" {
		t.Fatalf("the second watcher's token must complete the refresh, got %+v", savedRun)
	}
	tokens := api.usedTokens()
	if len(tokens) < 3 || tokens[0] != "gho_1" || tokens[1] != "gho_2" {
		t.Fatalf("expected user 1 to be tried, then user 2, got %v", tokens)
	}
	for _, token := range tokens[2:] {
		if token != "gho_2" {
			t.Errorf("after a 404 the resource must stay with the working token, got %v", tokens)
		}
	}
}

func TestRefreshActiveRuns_ForbiddenIsScopedToTheResource(t *testing.T) {
	api := newFakeGitHubAPI(t)
	// Run 500 was deleted on GitHub; run 501 is fine with the same token.
	api.fail("repos/acme/app/actions/runs/500", http.StatusNotFound)
	api.serve("repos/acme/app/actions/runs/501", `W/"run-501"`, map[string]interface{}{"id": 501, "status": "in_progress"})
	api.serve("repos/acme/app/actions/runs/501/jobs?per_page=100", `W/"jobs-501"`, map[string]interface{}{"total_count": 0, "jobs": []interface{}{}})

	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500), activeRun(12, 501)}, nil
		},
		getUserByIDFunc: tokenUser(1),
	}
	h := newPollerHandler(t, store, api)

	h.refreshActiveRuns(context.Background(), []int{1})

	paths := api.requestedPaths()
	if len(paths) != 3 {
		t.Fatalf("a 404 on one run must not stop the token for the other run, got %v", paths)
	}
}

func TestRefreshActiveRuns_RateLimitPausesTokenForThePass(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.fail("repos/acme/app/actions/runs/500", http.StatusForbidden)
	api.serve("repos/acme/app/actions/runs/501", `W/"run-501"`, map[string]interface{}{"id": 501, "status": "in_progress"})

	store := &mockStorage{
		listActivePipelinesFunc: func(ctx context.Context, userID int) ([]models.WorkflowRun, error) {
			return []models.WorkflowRun{activeRun(11, 500), activeRun(12, 501)}, nil
		},
		getUserByIDFunc: tokenUser(1),
	}
	h := newPollerHandler(t, store, api)

	h.refreshActiveRuns(context.Background(), []int{1})

	if api.requests.Load() != 1 {
		t.Errorf("after a rate-limit answer the token must rest until the next pass, got %d requests: %v", api.requests.Load(), api.requestedPaths())
	}
}

// ===== Discovery pass =====

func TestDiscoverNewRuns_StoresUnknownRunsAndThenHonoursETag(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs?per_page=20", `W/"list-1"`, map[string]interface{}{
		"total_count": 2,
		"workflow_runs": []map[string]interface{}{
			{"id": 600, "workflow_id": 30, "status": "queued", "name": "CI", "run_started_at": "2026-10-05T11:00:00Z"},
			{"id": 601, "workflow_id": 99, "status": "queued", "name": "Unknown workflow", "run_started_at": "2026-10-05T11:00:00Z"},
		},
	})

	var saved []models.WorkflowRun
	viewersAsked := atomic.Int32{}
	store := &mockStorage{
		listRepositoriesFunc: func(ctx context.Context, userID, page, pageSize int, search string) ([]models.Repository, int, error) {
			return []models.Repository{{ID: 7, FullName: "acme/app", IsActive: true}}, 1, nil
		},
		listWorkflowsFunc: func(ctx context.Context, userID int, repoID *int) ([]models.Workflow, error) {
			return []models.Workflow{{ID: 3, GitHubID: 30, RepoID: 7, Name: "CI", Path: ".github/workflows/ci.yml"}}, nil
		},
		getUserByIDFunc: tokenUser(1),
		getRunByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
			return nil, context.Canceled // not stored yet
		},
		upsertRunFunc: func(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			saved = append(saved, *run)
			return run, nil
		},
		listUsersWithRepoAccessFunc: func(ctx context.Context, repoID int) ([]int, error) {
			viewersAsked.Add(1)
			return []int{1}, nil
		},
	}
	h := newPollerHandler(t, store, api)

	h.discoverNewRuns(context.Background(), []int{1})

	if len(saved) != 1 || saved[0].GitHubID != 600 || saved[0].WorkflowID != 3 || saved[0].RepoID != 7 {
		t.Fatalf("expected only the run of a known workflow to be stored, got %+v", saved)
	}
	if viewersAsked.Load() != 1 {
		t.Errorf("expected exactly one workflow_run event per changed repository, viewers resolved %d times", viewersAsked.Load())
	}

	// Second pass: nothing changed on GitHub, so storage stays untouched.
	h.discoverNewRuns(context.Background(), []int{1})
	if len(saved) != 1 {
		t.Errorf("a 304 must not store runs again, got %d upserts", len(saved))
	}
	if api.requests.Load() != 2 {
		t.Errorf("expected two list requests, got %d", api.requests.Load())
	}
}

func TestDiscoverNewRuns_FailedSaveOrMissingWorkflowsDoesNotCacheETag(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs?per_page=20", `W/"list-1"`, map[string]interface{}{
		"total_count":   1,
		"workflow_runs": []map[string]interface{}{{"id": 600, "workflow_id": 30, "status": "queued", "name": "CI"}},
	})

	workflowsErr := context.DeadlineExceeded
	saveErr := context.DeadlineExceeded
	saves := 0
	store := &mockStorage{
		listRepositoriesFunc: func(ctx context.Context, userID, page, pageSize int, search string) ([]models.Repository, int, error) {
			return []models.Repository{{ID: 7, FullName: "acme/app", IsActive: true}}, 1, nil
		},
		listWorkflowsFunc: func(ctx context.Context, userID int, repoID *int) ([]models.Workflow, error) {
			if workflowsErr != nil {
				return nil, workflowsErr
			}
			return []models.Workflow{{ID: 3, GitHubID: 30, RepoID: 7, Name: "CI"}}, nil
		},
		getUserByIDFunc: tokenUser(1),
		getRunByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
			return nil, context.Canceled
		},
		upsertRunFunc: func(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
			saves++
			if saveErr != nil {
				return nil, saveErr
			}
			return run, nil
		},
	}
	h := newPollerHandler(t, store, api)

	h.discoverNewRuns(context.Background(), []int{1})
	if h.etags.get(repoETagKey(7)) != "" {
		t.Fatal("the list ETag must not be remembered when the workflow index cannot be loaded")
	}

	workflowsErr = nil
	h.discoverNewRuns(context.Background(), []int{1})
	if saves != 1 {
		t.Fatalf("expected the run to be attempted once the workflows load, got %d saves", saves)
	}
	if h.etags.get(repoETagKey(7)) != "" {
		t.Fatal("the list ETag must not be remembered when a run save failed")
	}

	saveErr = nil
	h.discoverNewRuns(context.Background(), []int{1})
	if saves != 2 {
		t.Fatalf("expected the list to be read again and the run saved, got %d saves", saves)
	}
	if h.etags.get(repoETagKey(7)) != `W/"list-1"` {
		t.Errorf("the list ETag must be remembered once every run is stored, got %q", h.etags.get(repoETagKey(7)))
	}
	if api.requests.Load() != 3 {
		t.Errorf("expected three list requests (none answered 304), got %d", api.requests.Load())
	}
}

func TestDiscoverNewRuns_KnownUnchangedRunsSendNoEvent(t *testing.T) {
	api := newFakeGitHubAPI(t)
	api.serve("repos/acme/app/actions/runs?per_page=20", `W/"list-1"`, map[string]interface{}{
		"total_count":   1,
		"workflow_runs": []map[string]interface{}{{"id": 600, "workflow_id": 30, "status": "completed", "conclusion": "success"}},
	})
	conclusion := "success"
	viewersAsked := atomic.Int32{}
	store := &mockStorage{
		listRepositoriesFunc: func(ctx context.Context, userID, page, pageSize int, search string) ([]models.Repository, int, error) {
			return []models.Repository{{ID: 7, FullName: "acme/app", IsActive: true}}, 1, nil
		},
		listWorkflowsFunc: func(ctx context.Context, userID int, repoID *int) ([]models.Workflow, error) {
			return []models.Workflow{{ID: 3, GitHubID: 30, RepoID: 7, Name: "CI"}}, nil
		},
		getUserByIDFunc: tokenUser(1),
		getRunByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
			return &models.WorkflowRun{ID: 11, GitHubID: 600, Status: "completed", Conclusion: &conclusion}, nil
		},
		listUsersWithRepoAccessFunc: func(ctx context.Context, repoID int) ([]int, error) {
			viewersAsked.Add(1)
			return []int{1}, nil
		},
	}
	h := newPollerHandler(t, store, api)

	h.discoverNewRuns(context.Background(), []int{1})

	if viewersAsked.Load() != 0 {
		t.Errorf("a run that is already stored in the same state must not produce an event, viewers resolved %d times", viewersAsked.Load())
	}
}

// ===== Watchers =====

func TestWatchingUsers_RecentAPIActivityCounts(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	if users := h.watchingUsers(); len(users) != 0 {
		t.Fatalf("expected nobody watching, got %v", users)
	}

	h.watchers.touch(5)
	h.watchers.touch(2)
	if users := h.watchingUsers(); len(users) != 2 || users[0] != 2 || users[1] != 5 {
		t.Fatalf("expected users 2 and 5 sorted, got %v", users)
	}

	h.watchers.mu.Lock()
	h.watchers.seen[5] = time.Now().Add(-watcherWindow - time.Second)
	h.watchers.mu.Unlock()
	if users := h.watchingUsers(); len(users) != 1 || users[0] != 2 {
		t.Fatalf("expected the idle user to drop out, got %v", users)
	}
}

func TestWatchingUsers_VisibleSocketsCountHiddenOnesDoNot(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.wsHub = websocket.NewHub()
	go h.wsHub.Run()

	visible := websocket.NewClient("visible", 3, h.wsHub, nil)
	hidden := websocket.NewClient("hidden", 4, h.wsHub, nil)
	silent := websocket.NewClient("silent", 5, h.wsHub, nil)
	h.wsHub.Register(visible)
	h.wsHub.Register(hidden)
	h.wsHub.Register(silent)
	time.Sleep(20 * time.Millisecond)
	visible.ApplyPresence(true)
	hidePage(t, hidden)

	if users := h.watchingUsers(); len(users) != 1 || users[0] != 3 {
		t.Fatalf("expected only the user with a visible page, got %v", users)
	}
}

func TestNoteAPIActivity_BrowserReportingPresenceDoesNotExtendIt(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.wsHub = websocket.NewHub()
	go h.wsHub.Run()

	tab := websocket.NewClient("tab", 3, h.wsHub, nil)
	h.wsHub.Register(tab)
	time.Sleep(20 * time.Millisecond)
	hidePage(t, tab)

	// A session request from a browser that reports presence over its socket is not presence.
	h.noteAPIActivity(3, false)
	if users := h.watchingUsers(); len(users) != 0 {
		t.Fatalf("hidden page with API traffic must not be watching, got %v", users)
	}

	// An API token client (MCP) has no socket: its requests are presence.
	h.noteAPIActivity(3, true)
	if users := h.watchingUsers(); len(users) != 1 || users[0] != 3 {
		t.Fatalf("bearer requests must count as presence, got %v", users)
	}

	// A browser whose socket is down polls the API: its requests are presence.
	h.noteAPIActivity(9, false)
	if users := h.watchingUsers(); len(users) != 2 {
		t.Fatalf("session requests without a socket must count as presence, got %v", users)
	}
}

func TestNoteAPIActivity_SocketWithoutPresenceFramesStillCounts(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.wsHub = websocket.NewHub()
	go h.wsHub.Run()

	// A client that never sends presence (for example a frontend built before presence frames)
	// keeps working through the API presence window.
	legacy := websocket.NewClient("legacy", 6, h.wsHub, nil)
	h.wsHub.Register(legacy)
	time.Sleep(20 * time.Millisecond)

	h.noteAPIActivity(6, false)
	if users := h.watchingUsers(); len(users) != 1 || users[0] != 6 {
		t.Fatalf("a socket that reports nothing must not suppress API presence, got %v", users)
	}
}

func TestPollLoop_NoWatchersNoPasses(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	passes := atomic.Int32{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	h.pollLoop(ctx, 10*time.Millisecond, h.wakeActive, func(context.Context, []int) { passes.Add(1) })

	if passes.Load() != 0 {
		t.Errorf("the loop must stay idle while nobody is watching, ran %d passes", passes.Load())
	}
}

func TestPollLoop_RunsPassesForWatchers(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.watchers.touch(1)
	passes := atomic.Int32{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	h.pollLoop(ctx, 10*time.Millisecond, h.wakeActive, func(_ context.Context, users []int) {
		if len(users) != 1 || users[0] != 1 {
			t.Errorf("unexpected watchers %v", users)
		}
		passes.Add(1)
	})

	if passes.Load() < 2 {
		t.Errorf("expected repeated passes while a user is watching, got %d", passes.Load())
	}
}

func TestPollLoop_WakeRunsPassBeforeTheTimer(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	passes := make(chan time.Time, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	go h.pollLoop(ctx, time.Hour, h.wakeActive, func(context.Context, []int) { passes <- time.Now() })

	// Nobody watching: the immediate first tick does nothing. Then a page becomes visible.
	time.Sleep(20 * time.Millisecond)
	h.watchers.touch(1)
	h.UserActivated(1)

	select {
	case at := <-passes:
		if at.Sub(start) > time.Second {
			t.Fatalf("wake must run a pass right away, took %s", at.Sub(start))
		}
	case <-time.After(time.Second):
		t.Fatal("expected a pass after UserActivated instead of waiting for the hourly timer")
	}

	// A second wake right after is absorbed by wakeSpacing.
	h.UserActivated(1)
	select {
	case <-passes:
		t.Fatal("a wake within wakeSpacing must not run another pass")
	case <-time.After(100 * time.Millisecond):
	}
}

// hidePage reports a hidden page for client the way the browser does.
func hidePage(t *testing.T, client *websocket.Client) {
	t.Helper()
	client.ApplyPresence(false)
	if client.Active() {
		t.Fatal("client should be inactive after hiding the page")
	}
}
