package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"snorlx/backend/internal/models"
	"snorlx/backend/internal/websocket"

	gh "github.com/google/go-github/v92/github"
)

func workflowJobEvent(repoGitHubID, runGitHubID, jobID int64) *gh.WorkflowJobEvent {
	return &gh.WorkflowJobEvent{
		Action: gh.Ptr("completed"),
		Repo:   &gh.Repository{ID: gh.Ptr(repoGitHubID)},
		WorkflowJob: &gh.WorkflowJob{
			ID:         gh.Ptr(jobID),
			RunID:      gh.Ptr(runGitHubID),
			Name:       gh.Ptr("build"),
			Status:     gh.Ptr("completed"),
			Conclusion: gh.Ptr("success"),
			StartedAt:  &gh.Timestamp{Time: time.Now().Add(-time.Minute)},
		},
	}
}

func newWebhookHandler(store *mockStorage) *Handler {
	h := newTestHandler(store)
	h.wsHub = websocket.NewHub()
	go h.wsHub.Run()
	return h
}

func TestProcessWebhookEvent_WorkflowJobIsStoredForItsRun(t *testing.T) {
	var saved *models.WorkflowJob
	viewersFor := 0
	store := &mockStorage{
		getRepoByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.Repository, error) {
			return &models.Repository{ID: 7, GitHubID: githubID, FullName: "acme/app"}, nil
		},
		getRunByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
			return &models.WorkflowRun{ID: 11, GitHubID: githubID, RepoID: 7}, nil
		},
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			saved = job
			return job, nil
		},
		listUsersWithRepoAccessFunc: func(ctx context.Context, repoID int) ([]int, error) {
			viewersFor = repoID
			return []int{1}, nil
		},
	}
	h := newWebhookHandler(store)

	h.processWebhookEvent("workflow_job", workflowJobEvent(700, 500, 9001))

	if saved == nil {
		t.Fatal("expected the job to be stored")
	}
	if saved.RunID != 11 || saved.GitHubID != 9001 || saved.Conclusion == nil || *saved.Conclusion != "success" {
		t.Errorf("stored job = %+v", saved)
	}
	if viewersFor != 7 {
		t.Errorf("expected the jobs event to go to the viewers of repository 7, resolved for %d", viewersFor)
	}
}

func TestProcessWebhookEvent_WorkflowJobOfAnotherRepositoryIsIgnored(t *testing.T) {
	stored := false
	store := &mockStorage{
		getRepoByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.Repository, error) {
			return &models.Repository{ID: 7, GitHubID: githubID, FullName: "acme/app"}, nil
		},
		getRunByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
			// The run exists but belongs to a different repository than the delivery claims.
			return &models.WorkflowRun{ID: 11, GitHubID: githubID, RepoID: 8}, nil
		},
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			stored = true
			return job, nil
		},
	}
	h := newWebhookHandler(store)

	h.processWebhookEvent("workflow_job", workflowJobEvent(700, 500, 9001))

	if stored {
		t.Error("a job must not be attached to a run of another repository")
	}
}

func TestProcessWebhookEvent_WorkflowJobForUnknownRunIsIgnored(t *testing.T) {
	stored := false
	store := &mockStorage{
		getRepoByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.Repository, error) {
			return &models.Repository{ID: 7, GitHubID: githubID, FullName: "acme/app"}, nil
		},
		getRunByGitHubIDFunc: func(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
			return nil, errors.New("run not found")
		},
		upsertJobFunc: func(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
			stored = true
			return job, nil
		},
	}
	h := newWebhookHandler(store)

	h.processWebhookEvent("workflow_job", workflowJobEvent(700, 500, 9001))

	if stored {
		t.Error("a job of a run that was never synced must be ignored")
	}
}
