package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"snorlx/backend/internal/config"

	ghLib "github.com/google/go-github/v92/github"
	"golang.org/x/oauth2"
)

// fakeGitHub serves a GitHub Enterprise style REST API (/api/v3 prefix) that honours
// If-None-Match for a single ETag per path.
type fakeGitHub struct {
	t        *testing.T
	server   *httptest.Server
	etags    map[string]string
	bodies   map[string]interface{}
	requests atomic.Int32
	hits304  atomic.Int32
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{t: t, etags: map[string]string{}, bodies: map[string]interface{}{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		etag, ok := f.etags[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("If-None-Match") == etag {
			f.hits304.Add(1)
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.bodies[key])
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) serve(path, etag string, body interface{}) {
	f.etags["/api/v3/"+path] = etag
	f.bodies["/api/v3/"+path] = body
}

func (f *fakeGitHub) client(t *testing.T) (*Client, *ghLib.Client) {
	t.Helper()
	c, err := NewClient(&config.Config{
		GitHubClientID:     "id",
		GitHubClientSecret: "secret",
		GitHubBaseURL:      f.server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	api := c.GetUserClient(context.Background(), &oauth2.Token{AccessToken: "token"})
	return c, api
}

func TestGetWorkflowRunIfChanged_FetchesThenReturnsNotModified(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.serve("repos/acme/app/actions/runs/42", `W/"run-v1"`, map[string]interface{}{
		"id": 42, "status": "in_progress",
	})
	c, api := gh.client(t)

	run, etag, changed, err := c.GetWorkflowRunIfChanged(context.Background(), api, "acme", "app", 42, "")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !changed || run == nil || run.GetID() != 42 || run.GetStatus() != "in_progress" {
		t.Fatalf("first call should return the run, got changed=%v run=%+v", changed, run)
	}
	if etag != `W/"run-v1"` {
		t.Fatalf("etag = %q", etag)
	}

	run, etag, changed, err = c.GetWorkflowRunIfChanged(context.Background(), api, "acme", "app", 42, etag)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if changed || run != nil {
		t.Fatalf("second call should be not modified, got changed=%v run=%+v", changed, run)
	}
	if etag != `W/"run-v1"` {
		t.Errorf("not-modified call must return the etag it sent, got %q", etag)
	}
	if gh.hits304.Load() != 1 {
		t.Errorf("expected one 304, got %d", gh.hits304.Load())
	}
}

func TestGetWorkflowRunIfChanged_StaleETagReturnsFreshRun(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.serve("repos/acme/app/actions/runs/42", `W/"run-v2"`, map[string]interface{}{
		"id": 42, "status": "completed", "conclusion": "success",
	})
	c, api := gh.client(t)

	run, etag, changed, err := c.GetWorkflowRunIfChanged(context.Background(), api, "acme", "app", 42, `W/"run-v1"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || run.GetConclusion() != "success" || etag != `W/"run-v2"` {
		t.Fatalf("expected the new run and etag, got changed=%v etag=%q run=%+v", changed, etag, run)
	}
}

func TestGetWorkflowRunIfChanged_PropagatesErrors(t *testing.T) {
	gh := newFakeGitHub(t)
	c, api := gh.client(t)

	_, _, _, err := c.GetWorkflowRunIfChanged(context.Background(), api, "acme", "app", 7, "")
	if err == nil {
		t.Fatal("expected an error for an unknown run")
	}
}

func TestListWorkflowJobsIfChanged_SinglePage(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.serve("repos/acme/app/actions/runs/42/jobs?per_page=100", `W/"jobs-v1"`, map[string]interface{}{
		"total_count": 2,
		"jobs": []map[string]interface{}{
			{"id": 1, "name": "build", "status": "completed"},
			{"id": 2, "name": "test", "status": "in_progress"},
		},
	})
	c, api := gh.client(t)

	jobs, etag, changed, err := c.ListWorkflowJobsIfChanged(context.Background(), api, "acme", "app", 42, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || len(jobs) != 2 || jobs[1].GetName() != "test" {
		t.Fatalf("expected both jobs, got changed=%v jobs=%d", changed, len(jobs))
	}

	jobs, _, changed, err = c.ListWorkflowJobsIfChanged(context.Background(), api, "acme", "app", 42, etag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || jobs != nil {
		t.Fatalf("expected not modified, got changed=%v jobs=%v", changed, jobs)
	}
	if gh.requests.Load() != 2 {
		t.Errorf("expected exactly two requests, got %d", gh.requests.Load())
	}
}

func TestListWorkflowJobsIfChanged_MultiPageReturnsNoETag(t *testing.T) {
	gh := newFakeGitHub(t)
	// The first page claims more jobs than it carries: the run spans several pages.
	gh.serve("repos/acme/app/actions/runs/42/jobs?per_page=100", `W/"jobs-p1"`, map[string]interface{}{
		"total_count": 3,
		"jobs": []map[string]interface{}{
			{"id": 1, "name": "build", "status": "completed"},
			{"id": 2, "name": "test", "status": "in_progress"},
		},
	})
	c, api := gh.client(t)

	jobs, etag, changed, err := c.ListWorkflowJobsIfChanged(context.Background(), api, "acme", "app", 42, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || len(jobs) == 0 {
		t.Fatalf("expected the full listing, got changed=%v jobs=%d", changed, len(jobs))
	}
	if etag != "" {
		t.Errorf("a multi-page listing must not hand out a first-page ETag, got %q", etag)
	}
}

func TestListRecentWorkflowRunsIfChanged(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.serve("repos/acme/app/actions/runs?per_page=20", `W/"list-v1"`, map[string]interface{}{
		"total_count": 1,
		"workflow_runs": []map[string]interface{}{
			{"id": 100, "status": "queued", "workflow_id": 9},
		},
	})
	c, api := gh.client(t)

	runs, etag, changed, err := c.ListRecentWorkflowRunsIfChanged(context.Background(), api, "acme", "app", 20, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || len(runs) != 1 || runs[0].GetID() != 100 {
		t.Fatalf("expected the queued run, got changed=%v runs=%d", changed, len(runs))
	}

	runs, _, changed, err = c.ListRecentWorkflowRunsIfChanged(context.Background(), api, "acme", "app", 20, etag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || runs != nil {
		t.Fatalf("expected not modified, got changed=%v runs=%v", changed, runs)
	}
}
