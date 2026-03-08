package storage

import (
	"context"
	"testing"
	"time"

	"snorlx/backend/internal/models"
)

func newTestStorage() *MemoryStorage {
	return NewMemoryStorage()
}

// ===== Organizations =====

func TestUpsertAndGetOrganization(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	org := &models.Organization{
		GitHubID: 1001,
		Login:    "test-org",
	}

	created, err := s.UpsertOrganization(ctx, org)
	if err != nil {
		t.Fatalf("UpsertOrganization failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}
	if created.Login != "test-org" {
		t.Errorf("expected login test-org, got %q", created.Login)
	}

	got, err := s.GetOrganization(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetOrganization failed: %v", err)
	}
	if got.Login != "test-org" {
		t.Errorf("expected login test-org, got %q", got.Login)
	}
}

func TestUpsertOrganization_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	org := &models.Organization{GitHubID: 2001, Login: "original"}
	created, _ := s.UpsertOrganization(ctx, org)

	updated := &models.Organization{GitHubID: 2001, Login: "updated"}
	result, err := s.UpsertOrganization(ctx, updated)
	if err != nil {
		t.Fatalf("UpsertOrganization update failed: %v", err)
	}
	if result.ID != created.ID {
		t.Errorf("expected same ID %d, got %d", created.ID, result.ID)
	}
	if result.Login != "updated" {
		t.Errorf("expected login updated, got %q", result.Login)
	}
}

func TestGetOrganizationByGitHubID(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	org := &models.Organization{GitHubID: 3001, Login: "gh-org"}
	s.UpsertOrganization(ctx, org)

	got, err := s.GetOrganizationByGitHubID(ctx, 3001)
	if err != nil {
		t.Fatalf("GetOrganizationByGitHubID failed: %v", err)
	}
	if got.Login != "gh-org" {
		t.Errorf("expected gh-org, got %q", got.Login)
	}
}

func TestGetOrganization_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	_, err := s.GetOrganization(ctx, 9999)
	if err == nil {
		t.Error("expected error for missing organization")
	}
}

func TestListOrganizations_SortedByLogin(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertOrganization(ctx, &models.Organization{GitHubID: 1, Login: "zebra-org"})
	s.UpsertOrganization(ctx, &models.Organization{GitHubID: 2, Login: "alpha-org"})
	s.UpsertOrganization(ctx, &models.Organization{GitHubID: 3, Login: "middle-org"})

	orgs, err := s.ListOrganizations(ctx)
	if err != nil {
		t.Fatalf("ListOrganizations failed: %v", err)
	}
	if len(orgs) != 3 {
		t.Fatalf("expected 3 orgs, got %d", len(orgs))
	}
	if orgs[0].Login != "alpha-org" || orgs[1].Login != "middle-org" || orgs[2].Login != "zebra-org" {
		t.Errorf("orgs not sorted: %v", []string{orgs[0].Login, orgs[1].Login, orgs[2].Login})
	}
}

// ===== Repositories =====

func TestUpsertAndGetRepository(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	repo := &models.Repository{
		GitHubID: 5001,
		Name:     "my-repo",
		FullName: "owner/my-repo",
		IsActive: true,
	}

	created, err := s.UpsertRepository(ctx, repo)
	if err != nil {
		t.Fatalf("UpsertRepository failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}

	got, err := s.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if got.FullName != "owner/my-repo" {
		t.Errorf("expected owner/my-repo, got %q", got.FullName)
	}
}

func TestListRepositories_Pagination(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	for i := 0; i < 5; i++ {
		s.UpsertRepository(ctx, &models.Repository{
			GitHubID: int64(i + 1),
			Name:     "repo",
			FullName: "owner/repo-" + string(rune('a'+i)),
			IsActive: true,
		})
	}

	repos, total, err := s.ListRepositories(ctx, 1, 2, "")
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos on page 1, got %d", len(repos))
	}
}

func TestListRepositories_Search(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertRepository(ctx, &models.Repository{GitHubID: 1, Name: "frontend", FullName: "org/frontend", IsActive: true})
	s.UpsertRepository(ctx, &models.Repository{GitHubID: 2, Name: "backend", FullName: "org/backend", IsActive: true})
	s.UpsertRepository(ctx, &models.Repository{GitHubID: 3, Name: "infra", FullName: "org/infra", IsActive: true})

	repos, total, err := s.ListRepositories(ctx, 1, 10, "back")
	if err != nil {
		t.Fatalf("ListRepositories with search failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 for search 'back', got %d", total)
	}
	if len(repos) != 1 || repos[0].Name != "backend" {
		t.Errorf("expected backend repo in search results, got %v", repos)
	}
}

func TestListRepositories_InactiveExcluded(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertRepository(ctx, &models.Repository{GitHubID: 1, Name: "active", FullName: "org/active", IsActive: true})
	s.UpsertRepository(ctx, &models.Repository{GitHubID: 2, Name: "inactive", FullName: "org/inactive", IsActive: false})

	repos, total, err := s.ListRepositories(ctx, 1, 10, "")
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected only 1 active repo, got %d", total)
	}
	if len(repos) != 1 || repos[0].Name != "active" {
		t.Errorf("expected only active repo, got %v", repos)
	}
}

func TestUpdateRepository(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	repo := &models.Repository{GitHubID: 8001, Name: "original", FullName: "org/original", IsActive: true}
	created, _ := s.UpsertRepository(ctx, repo)

	update := &models.Repository{Name: "renamed", FullName: "org/renamed", IsActive: false}
	updated, err := s.UpdateRepository(ctx, created.ID, update)
	if err != nil {
		t.Fatalf("UpdateRepository failed: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("expected renamed, got %q", updated.Name)
	}
	if updated.IsActive {
		t.Error("expected IsActive to be false after update")
	}
}

// ===== Workflows =====

func TestUpsertAndGetWorkflow(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	wf := &models.Workflow{
		GitHubID: 9001,
		RepoID:   1,
		Name:     "CI",
		Path:     ".github/workflows/ci.yml",
		State:    "active",
	}

	created, err := s.UpsertWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("UpsertWorkflow failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}

	got, err := s.GetWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if got.Name != "CI" {
		t.Errorf("expected CI, got %q", got.Name)
	}
}

func TestGetWorkflowByGitHubID(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 7777, RepoID: 1, Name: "Deploy"})

	got, err := s.GetWorkflowByGitHubID(ctx, 7777)
	if err != nil {
		t.Fatalf("GetWorkflowByGitHubID failed: %v", err)
	}
	if got.Name != "Deploy" {
		t.Errorf("expected Deploy, got %q", got.Name)
	}
}

// ===== Workflow Runs =====

func TestUpsertAndGetRun(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	run := &models.WorkflowRun{
		GitHubID:   1001,
		WorkflowID: 1,
		RepoID:     1,
		RunNumber:  42,
		Name:       "CI Run",
		Status:     "completed",
		Event:      "push",
		Branch:     "main",
		StartedAt:  time.Now(),
	}

	created, err := s.UpsertRun(ctx, run)
	if err != nil {
		t.Fatalf("UpsertRun failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}

	got, err := s.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if got.RunNumber != 42 {
		t.Errorf("expected run number 42, got %d", got.RunNumber)
	}
}

func TestListRuns_Filters(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID: 1, WorkflowID: 10, RepoID: 1,
		Status: "completed", Branch: "main", StartedAt: time.Now(),
	})
	s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID: 2, WorkflowID: 20, RepoID: 2,
		Status: "in_progress", Branch: "feature", StartedAt: time.Now(),
	})

	runs, total, err := s.ListRuns(ctx, &models.RunFilters{Status: "completed"}, 1, 10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 completed run, got %d", total)
	}
	if len(runs) != 1 || runs[0].Status != "completed" {
		t.Errorf("unexpected runs: %v", runs)
	}
}

func TestListRuns_Pagination_EmptyPage(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, StartedAt: time.Now()})

	runs, total, err := s.ListRuns(ctx, nil, 2, 10)
	if err != nil {
		t.Fatalf("ListRuns page 2 failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(runs) != 0 {
		t.Errorf("expected empty page 2, got %d runs", len(runs))
	}
}

func TestGetRunByGitHubID(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 5555, RunNumber: 99, StartedAt: time.Now()})

	got, err := s.GetRunByGitHubID(ctx, 5555)
	if err != nil {
		t.Fatalf("GetRunByGitHubID failed: %v", err)
	}
	if got.RunNumber != 99 {
		t.Errorf("expected run number 99, got %d", got.RunNumber)
	}
}

// ===== Jobs =====

func TestUpsertAndGetJob(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	job := &models.WorkflowJob{
		GitHubID:  2001,
		RunID:     1,
		Name:      "build",
		Status:    "completed",
		StartedAt: time.Now(),
	}

	created, err := s.UpsertJob(ctx, job)
	if err != nil {
		t.Fatalf("UpsertJob failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}

	got, err := s.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.Name != "build" {
		t.Errorf("expected build, got %q", got.Name)
	}
}

func TestListJobsForRun(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertJob(ctx, &models.WorkflowJob{GitHubID: 1, RunID: 10, Name: "build", StartedAt: time.Now()})
	s.UpsertJob(ctx, &models.WorkflowJob{GitHubID: 2, RunID: 10, Name: "test", StartedAt: time.Now().Add(time.Second)})
	s.UpsertJob(ctx, &models.WorkflowJob{GitHubID: 3, RunID: 20, Name: "other", StartedAt: time.Now()})

	jobs, err := s.ListJobsForRun(ctx, 10)
	if err != nil {
		t.Fatalf("ListJobsForRun failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs for run 10, got %d", len(jobs))
	}
}

// ===== Users & Sessions =====

func TestUpsertAndGetUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	user := &models.User{
		GitHubID: 9999,
		Login:    "octocat",
	}

	created, err := s.UpsertUser(ctx, user)
	if err != nil {
		t.Fatalf("UpsertUser failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}

	got, err := s.GetUserByGitHubID(ctx, 9999)
	if err != nil {
		t.Fatalf("GetUserByGitHubID failed: %v", err)
	}
	if got.Login != "octocat" {
		t.Errorf("expected octocat, got %q", got.Login)
	}
}

func TestGetUserByGitHubID_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	_, err := s.GetUserByGitHubID(ctx, 9999999)
	if err == nil {
		t.Error("expected error for missing user")
	}
}

func TestCreateAndGetSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	user := &models.User{GitHubID: 1, Login: "alice"}
	createdUser, _ := s.UpsertUser(ctx, user)

	session := &models.Session{
		ID:        "test-session-id",
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	gotSession, gotUser, err := s.GetSession(ctx, "test-session-id")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if gotSession.ID != "test-session-id" {
		t.Errorf("expected session ID test-session-id, got %q", gotSession.ID)
	}
	if gotUser.Login != "alice" {
		t.Errorf("expected user alice, got %q", gotUser.Login)
	}
}

func TestGetSession_Expired(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	user := &models.User{GitHubID: 2, Login: "bob"}
	createdUser, _ := s.UpsertUser(ctx, user)

	session := &models.Session{
		ID:        "expired-session",
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	}
	s.CreateSession(ctx, session)

	_, _, err := s.GetSession(ctx, "expired-session")
	if err == nil {
		t.Error("expected error for expired session")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	_, _, err := s.GetSession(ctx, "nonexistent-session")
	if err == nil {
		t.Error("expected error for missing session")
	}
}

func TestDeleteSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	user := &models.User{GitHubID: 3, Login: "carol"}
	createdUser, _ := s.UpsertUser(ctx, user)

	session := &models.Session{
		ID:        "delete-me",
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	s.CreateSession(ctx, session)

	if err := s.DeleteSession(ctx, "delete-me"); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	_, _, err := s.GetSession(ctx, "delete-me")
	if err == nil {
		t.Error("expected error after deleting session")
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	user := &models.User{GitHubID: 4, Login: "dave"}
	createdUser, _ := s.UpsertUser(ctx, user)

	s.CreateSession(ctx, &models.Session{
		ID:        "valid-session",
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	s.CreateSession(ctx, &models.Session{
		ID:        "expired-session-1",
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(-time.Hour),
	})
	s.CreateSession(ctx, &models.Session{
		ID:        "expired-session-2",
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(-2 * time.Hour),
	})

	if err := s.CleanExpiredSessions(ctx); err != nil {
		t.Fatalf("CleanExpiredSessions failed: %v", err)
	}

	// Valid session should still be accessible
	_, _, err := s.GetSession(ctx, "valid-session")
	if err != nil {
		t.Errorf("valid session should still exist: %v", err)
	}

	// Expired sessions should be gone
	for _, id := range []string{"expired-session-1", "expired-session-2"} {
		_, _, err := s.GetSession(ctx, id)
		if err == nil {
			t.Errorf("expired session %q should have been cleaned", id)
		}
	}
}

// ===== Dashboard & Metrics =====

func TestGetDashboardSummary(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	s.UpsertRepository(ctx, &models.Repository{GitHubID: 1, Name: "r1", FullName: "org/r1", IsActive: true})
	s.UpsertRepository(ctx, &models.Repository{GitHubID: 2, Name: "r2", FullName: "org/r2", IsActive: false})

	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 1, RepoID: 1, Name: "CI", State: "active"})
	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 2, RepoID: 1, Name: "Deploy", State: "disabled"})

	summary, err := s.GetDashboardSummary(ctx)
	if err != nil {
		t.Fatalf("GetDashboardSummary failed: %v", err)
	}
	if summary.Repositories.Total != 2 {
		t.Errorf("expected 2 total repos, got %d", summary.Repositories.Total)
	}
	if summary.Repositories.Active != 1 {
		t.Errorf("expected 1 active repo, got %d", summary.Repositories.Active)
	}
	if summary.Workflows.Total != 2 {
		t.Errorf("expected 2 total workflows, got %d", summary.Workflows.Total)
	}
	if summary.Workflows.Active != 1 {
		t.Errorf("expected 1 active workflow, got %d", summary.Workflows.Active)
	}
}

func TestGetTrends(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	success := "success"
	failure := "failure"
	now := time.Now()

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, StartedAt: now, Conclusion: &success})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 2, StartedAt: now.Add(-time.Hour), Conclusion: &failure})
	// Older than 7 days — should not appear
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 3, StartedAt: now.Add(-8 * 24 * time.Hour), Conclusion: &success})

	trends, err := s.GetTrends(ctx, 7)
	if err != nil {
		t.Fatalf("GetTrends failed: %v", err)
	}
	total := 0
	for _, t2 := range trends {
		total += t2.TotalRuns
	}
	if total != 2 {
		t.Errorf("expected 2 runs in trends (last 7 days), got %d", total)
	}
}

func TestGetDevOpsMetrics(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	now := time.Now()
	dur := 3600 // 1 hour
	failure := "failure"
	success := "success"

	s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID:        1,
		StartedAt:       now.Add(-time.Hour),
		IsDeployment:    true,
		Conclusion:      &success,
		DurationSeconds: &dur,
	})
	s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID:     2,
		StartedAt:    now.Add(-2 * time.Hour),
		IsDeployment: true,
		Conclusion:   &failure,
	})

	metrics, err := s.GetDevOpsMetrics(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("GetDevOpsMetrics failed: %v", err)
	}
	if metrics.DeploymentFrequency.TotalDeployments != 2 {
		t.Errorf("expected 2 total deployments, got %d", metrics.DeploymentFrequency.TotalDeployments)
	}
	if metrics.ChangeFailureRate.FailedDeployments != 1 {
		t.Errorf("expected 1 failed deployment, got %d", metrics.ChangeFailureRate.FailedDeployments)
	}
}

func TestGetDevOpsMetrics_MTTR(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	now := time.Now()
	failure := "failure"
	success := "success"
	// Failed at T-2h, recovered (next success) at T-1h -> recovery time 1h = 60 min
	failedAt := now.Add(-2 * time.Hour)
	successAt := now.Add(-1 * time.Hour)

	s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID:     1,
		StartedAt:   failedAt.Add(-30 * time.Minute),
		IsDeployment: true,
		Conclusion:  &failure,
		CompletedAt: &failedAt,
	})
	s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID:     2,
		StartedAt:   successAt.Add(-10 * time.Minute),
		IsDeployment: true,
		Conclusion:  &success,
		CompletedAt: &successAt,
	})

	metrics, err := s.GetDevOpsMetrics(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("GetDevOpsMetrics failed: %v", err)
	}
	// MTTR should be ~60 min (recovery from failure to next success)
	if metrics.MTTR.MedianMinutes < 50 || metrics.MTTR.MedianMinutes > 70 {
		t.Errorf("expected MTTR median ~60 min, got %d", metrics.MTTR.MedianMinutes)
	}
}

func TestBackfillDeploymentRuns(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	// Create repo and workflow with "release" in name so heuristic matches
	repo := &models.Repository{GitHubID: 1, Name: "test", FullName: "owner/test"}
	_, _ = s.UpsertRepository(ctx, repo)
	wf := &models.Workflow{GitHubID: 1, RepoID: repo.ID, Name: "Release workflow", Path: ".github/workflows/release.yml"}
	_, _ = s.UpsertWorkflow(ctx, wf)

	// Run without IsDeployment set
	run := &models.WorkflowRun{
		GitHubID:    100,
		WorkflowID:  wf.ID,
		RepoID:     repo.ID,
		StartedAt:  time.Now(),
		Event:      "push",
		IsDeployment: false,
	}
	_, err := s.UpsertRun(ctx, run)
	if err != nil {
		t.Fatalf("UpsertRun failed: %v", err)
	}

	updated, err := s.BackfillDeploymentRuns(ctx)
	if err != nil {
		t.Fatalf("BackfillDeploymentRuns failed: %v", err)
	}
	if updated != 1 {
		t.Errorf("expected 1 run updated, got %d", updated)
	}

	// Verify the run is now marked as deployment
	stored, _ := s.GetRunByGitHubID(ctx, 100)
	if stored == nil || !stored.IsDeployment {
		t.Error("expected run to have IsDeployment true after backfill")
	}
}

// ===== Storage Lifecycle =====

func TestClose(t *testing.T) {
	s := newTestStorage()
	if err := s.Close(); err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

func TestMigrate(t *testing.T) {
	s := newTestStorage()
	if err := s.Migrate(); err != nil {
		t.Errorf("Migrate should not return error for memory storage: %v", err)
	}
}
