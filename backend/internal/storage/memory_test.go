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

// seedUser creates a user and returns its ID.
func seedUser(t *testing.T, s *MemoryStorage, githubID int64, login string) int {
	t.Helper()
	user, err := s.UpsertUser(context.Background(), &models.User{GitHubID: githubID, Login: login})
	if err != nil {
		t.Fatalf("UpsertUser(%s): %v", login, err)
	}
	return user.ID
}

// seedRepo creates a repository visible to userID and returns it.
func seedRepo(t *testing.T, s *MemoryStorage, userID int, githubID int64, name string, active bool) *models.Repository {
	t.Helper()
	ctx := context.Background()
	repo, err := s.UpsertRepository(ctx, &models.Repository{GitHubID: githubID, Name: name, FullName: "org/" + name, IsActive: active})
	if err != nil {
		t.Fatalf("UpsertRepository(%s): %v", name, err)
	}
	if err := s.GrantRepositoryAccess(ctx, userID, repo.ID); err != nil {
		t.Fatalf("GrantRepositoryAccess(%d, %d): %v", userID, repo.ID, err)
	}
	return repo
}

// ===== Repository access (tenancy) =====

func TestGrantRepositoryAccess_RequiresExistingUserAndRepo(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	if err := s.GrantRepositoryAccess(ctx, 1, 1); err == nil {
		t.Fatal("expected error when neither user nor repository exist")
	}
	userID := seedUser(t, s, 1, "alice")
	if err := s.GrantRepositoryAccess(ctx, userID, 99); err == nil {
		t.Fatal("expected error for unknown repository")
	}
	repo, _ := s.UpsertRepository(ctx, &models.Repository{GitHubID: 1, Name: "r", FullName: "org/r", IsActive: true})
	if err := s.GrantRepositoryAccess(ctx, 99, repo.ID); err == nil {
		t.Fatal("expected error for unknown user")
	}
	if err := s.GrantRepositoryAccess(ctx, userID, repo.ID); err != nil {
		t.Fatalf("GrantRepositoryAccess: %v", err)
	}
	// Idempotent
	if err := s.GrantRepositoryAccess(ctx, userID, repo.ID); err != nil {
		t.Fatalf("second GrantRepositoryAccess: %v", err)
	}
}

func TestHasRepositoryAccess_AndViewers(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")
	shared := seedRepo(t, s, alice, 10, "shared", true)
	if err := s.GrantRepositoryAccess(ctx, bob, shared.ID); err != nil {
		t.Fatalf("grant bob: %v", err)
	}
	private := seedRepo(t, s, alice, 11, "private", true)

	for _, tc := range []struct {
		user, repo int
		want       bool
	}{
		{alice, shared.ID, true},
		{bob, shared.ID, true},
		{alice, private.ID, true},
		{bob, private.ID, false},
		{bob, 999, false},
	} {
		got, err := s.HasRepositoryAccess(ctx, tc.user, tc.repo)
		if err != nil || got != tc.want {
			t.Errorf("HasRepositoryAccess(%d,%d) = %v,%v want %v", tc.user, tc.repo, got, err, tc.want)
		}
	}

	viewers, err := s.ListUsersWithRepositoryAccess(ctx, shared.ID)
	if err != nil || len(viewers) != 2 || viewers[0] != alice || viewers[1] != bob {
		t.Fatalf("viewers of shared = %v, %v", viewers, err)
	}
	viewers, _ = s.ListUsersWithRepositoryAccess(ctx, private.ID)
	if len(viewers) != 1 || viewers[0] != alice {
		t.Fatalf("viewers of private = %v", viewers)
	}
}

// ===== Organizations =====

func TestUpsertAndGetOrganization(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")

	org := &models.Organization{GitHubID: 1001, Login: "test-org"}
	created, err := s.UpsertOrganization(ctx, org)
	if err != nil {
		t.Fatalf("UpsertOrganization failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}

	// Not visible until the user has a repository in that organization
	if _, err := s.GetOrganization(ctx, alice, created.ID); err == nil {
		t.Fatal("expected organization to be hidden without repository access")
	}

	repo, _ := s.UpsertRepository(ctx, &models.Repository{GitHubID: 1, OrgID: &created.ID, Name: "r", FullName: "test-org/r", IsActive: true})
	_ = s.GrantRepositoryAccess(ctx, alice, repo.ID)

	got, err := s.GetOrganization(ctx, alice, created.ID)
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

	created, _ := s.UpsertOrganization(ctx, &models.Organization{GitHubID: 2001, Login: "original"})
	result, err := s.UpsertOrganization(ctx, &models.Organization{GitHubID: 2001, Login: "updated"})
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

	s.UpsertOrganization(ctx, &models.Organization{GitHubID: 3001, Login: "gh-org"})

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
	alice := seedUser(t, s, 1, "alice")

	if _, err := s.GetOrganization(ctx, alice, 9999); err == nil {
		t.Error("expected error for missing organization")
	}
}

func TestListOrganizations_ScopedAndSorted(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")

	zebra, _ := s.UpsertOrganization(ctx, &models.Organization{GitHubID: 1, Login: "zebra-org"})
	alpha, _ := s.UpsertOrganization(ctx, &models.Organization{GitHubID: 2, Login: "alpha-org"})
	middle, _ := s.UpsertOrganization(ctx, &models.Organization{GitHubID: 3, Login: "middle-org"})

	for i, org := range []*models.Organization{zebra, alpha} {
		repo, _ := s.UpsertRepository(ctx, &models.Repository{GitHubID: int64(100 + i), OrgID: &org.ID, Name: "r", FullName: org.Login + "/r", IsActive: true})
		_ = s.GrantRepositoryAccess(ctx, alice, repo.ID)
	}
	repo, _ := s.UpsertRepository(ctx, &models.Repository{GitHubID: 200, OrgID: &middle.ID, Name: "r", FullName: "middle-org/r", IsActive: true})
	_ = s.GrantRepositoryAccess(ctx, bob, repo.ID)

	orgs, err := s.ListOrganizations(ctx, alice)
	if err != nil {
		t.Fatalf("ListOrganizations failed: %v", err)
	}
	if len(orgs) != 2 || orgs[0].Login != "alpha-org" || orgs[1].Login != "zebra-org" {
		t.Errorf("unexpected orgs for alice: %+v", orgs)
	}
	orgs, _ = s.ListOrganizations(ctx, bob)
	if len(orgs) != 1 || orgs[0].Login != "middle-org" {
		t.Errorf("unexpected orgs for bob: %+v", orgs)
	}
}

// ===== Repositories =====

func TestUpsertAndGetRepository(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	created, err := s.UpsertRepository(ctx, &models.Repository{GitHubID: 5001, Name: "my-repo", FullName: "owner/my-repo", IsActive: true})
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
	alice := seedUser(t, s, 1, "alice")

	for i := 0; i < 5; i++ {
		seedRepo(t, s, alice, int64(i+1), "repo-"+string(rune('a'+i)), true)
	}

	repos, total, err := s.ListRepositories(ctx, alice, 1, 2, "")
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
	alice := seedUser(t, s, 1, "alice")

	seedRepo(t, s, alice, 1, "frontend", true)
	seedRepo(t, s, alice, 2, "backend", true)
	seedRepo(t, s, alice, 3, "infra", true)

	repos, total, err := s.ListRepositories(ctx, alice, 1, 10, "back")
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
	alice := seedUser(t, s, 1, "alice")

	seedRepo(t, s, alice, 1, "active", true)
	seedRepo(t, s, alice, 2, "inactive", false)

	repos, total, err := s.ListRepositories(ctx, alice, 1, 10, "")
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

func TestListRepositories_OnlyGrantedRepositories(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")

	seedRepo(t, s, alice, 1, "alice-repo", true)
	seedRepo(t, s, bob, 2, "bob-repo", true)

	repos, total, err := s.ListRepositories(ctx, alice, 1, 10, "")
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if total != 1 || len(repos) != 1 || repos[0].Name != "alice-repo" {
		t.Errorf("alice must only see her repository, got total=%d %v", total, repos)
	}
	repos, total, _ = s.ListRepositories(ctx, 999, 1, 10, "")
	if total != 0 || len(repos) != 0 {
		t.Errorf("unknown user must see nothing, got total=%d %v", total, repos)
	}
}

func TestUpdateRepository(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	created, _ := s.UpsertRepository(ctx, &models.Repository{GitHubID: 8001, Name: "original", FullName: "org/original", IsActive: true})

	updated, err := s.UpdateRepository(ctx, created.ID, &models.Repository{Name: "renamed", FullName: "org/renamed", IsActive: false})
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

	created, err := s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 9001, RepoID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"})
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

func TestListWorkflows_ScopedToUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")
	aliceRepo := seedRepo(t, s, alice, 1, "alice-repo", true)
	bobRepo := seedRepo(t, s, bob, 2, "bob-repo", true)

	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 1, RepoID: aliceRepo.ID, Name: "CI"})
	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 2, RepoID: bobRepo.ID, Name: "Deploy"})

	workflows, err := s.ListWorkflows(ctx, alice, nil)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 1 || workflows[0].Name != "CI" {
		t.Errorf("alice must only see CI, got %+v", workflows)
	}
	// Asking for another user's repository explicitly yields nothing
	workflows, _ = s.ListWorkflows(ctx, alice, &bobRepo.ID)
	if len(workflows) != 0 {
		t.Errorf("alice must not see bob's workflows, got %+v", workflows)
	}
}

// ===== Workflow Runs =====

func TestUpsertAndGetRun(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	created, err := s.UpsertRun(ctx, &models.WorkflowRun{
		GitHubID: 1001, WorkflowID: 1, RepoID: 1, RunNumber: 42, Name: "CI Run",
		Status: "completed", Event: "push", Branch: "main", StartedAt: time.Now(),
	})
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
	alice := seedUser(t, s, 1, "alice")
	repo := seedRepo(t, s, alice, 1, "r", true)

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, WorkflowID: 10, RepoID: repo.ID, Status: "completed", Branch: "main", StartedAt: time.Now()})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 2, WorkflowID: 20, RepoID: repo.ID, Status: "in_progress", Branch: "feature", StartedAt: time.Now()})

	runs, total, err := s.ListRuns(ctx, alice, &models.RunFilters{Status: "completed"}, 1, 10)
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

func TestListRuns_ScopedToUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")
	aliceRepo := seedRepo(t, s, alice, 1, "alice-repo", true)
	bobRepo := seedRepo(t, s, bob, 2, "bob-repo", true)

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, RepoID: aliceRepo.ID, Status: "in_progress", StartedAt: time.Now()})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 2, RepoID: bobRepo.ID, Status: "in_progress", StartedAt: time.Now()})

	runs, total, _ := s.ListRuns(ctx, alice, nil, 1, 10)
	if total != 1 || len(runs) != 1 || runs[0].RepoID != aliceRepo.ID {
		t.Errorf("alice must only see her runs, got total=%d %v", total, runs)
	}
	// Filtering on another user's repository leaks nothing
	runs, total, _ = s.ListRuns(ctx, alice, &models.RunFilters{RepoID: bobRepo.ID}, 1, 10)
	if total != 0 || len(runs) != 0 {
		t.Errorf("alice must not see bob's runs via filter, got total=%d %v", total, runs)
	}
	active, _ := s.ListActivePipelines(ctx, bob)
	if len(active) != 1 || active[0].RepoID != bobRepo.ID {
		t.Errorf("bob must only see his active pipelines, got %v", active)
	}
}

func TestListRuns_Pagination_EmptyPage(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	repo := seedRepo(t, s, alice, 1, "r", true)

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, RepoID: repo.ID, StartedAt: time.Now()})

	runs, total, err := s.ListRuns(ctx, alice, nil, 2, 10)
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

	created, err := s.UpsertJob(ctx, &models.WorkflowJob{GitHubID: 2001, RunID: 1, Name: "build", Status: "completed", StartedAt: time.Now()})
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

func TestUpsertJob_MovedStartKeepsOneRowAndID(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	queuedAt := time.Date(2026, 10, 5, 15, 13, 35, 0, time.UTC)
	queued, err := s.UpsertJob(ctx, &models.WorkflowJob{GitHubID: 3001, RunID: 1, Name: "Resolve release tag", Status: "queued", StartedAt: queuedAt})
	if err != nil {
		t.Fatalf("UpsertJob (queued) failed: %v", err)
	}

	startedAt := queuedAt.Add(4 * time.Second)
	completedAt := startedAt.Add(5 * time.Second)
	runner := "GitHub Actions 42"
	conclusion := "success"
	done, err := s.UpsertJob(ctx, &models.WorkflowJob{
		GitHubID: 3001, RunID: 1, Name: "Resolve release tag", Status: "completed", Conclusion: &conclusion,
		RunnerName: &runner, StartedAt: startedAt, CompletedAt: &completedAt,
	})
	if err != nil {
		t.Fatalf("UpsertJob (completed) failed: %v", err)
	}
	if done.ID != queued.ID {
		t.Errorf("expected the same internal id %d, got %d", queued.ID, done.ID)
	}

	jobs, err := s.ListJobsForRun(ctx, 1)
	if err != nil {
		t.Fatalf("ListJobsForRun failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected a single row for the job, got %d", len(jobs))
	}
	got := jobs[0]
	if got.Status != "completed" || !got.StartedAt.Equal(startedAt) || got.RunnerName == nil || *got.RunnerName != runner {
		t.Errorf("expected the stored job to carry the latest status, started_at and runner, got %+v", got)
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

	created, err := s.UpsertUser(ctx, &models.User{GitHubID: 9999, Login: "octocat"})
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

	if _, err := s.GetUserByGitHubID(ctx, 9999999); err == nil {
		t.Error("expected error for missing user")
	}
}

func TestCreateAndGetSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")

	if err := s.CreateSession(ctx, &models.Session{ID: "test-session-id", UserID: alice, ExpiresAt: time.Now().Add(24 * time.Hour)}); err != nil {
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
	bob := seedUser(t, s, 2, "bob")

	s.CreateSession(ctx, &models.Session{ID: "expired-session", UserID: bob, ExpiresAt: time.Now().Add(-1 * time.Hour)})

	if _, _, err := s.GetSession(ctx, "expired-session"); err == nil {
		t.Error("expected error for expired session")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()

	if _, _, err := s.GetSession(ctx, "nonexistent-session"); err == nil {
		t.Error("expected error for missing session")
	}
}

func TestDeleteSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	carol := seedUser(t, s, 3, "carol")

	s.CreateSession(ctx, &models.Session{ID: "delete-me", UserID: carol, ExpiresAt: time.Now().Add(time.Hour)})

	if err := s.DeleteSession(ctx, "delete-me"); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
	if _, _, err := s.GetSession(ctx, "delete-me"); err == nil {
		t.Error("expected error after deleting session")
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	dave := seedUser(t, s, 4, "dave")

	s.CreateSession(ctx, &models.Session{ID: "valid-session", UserID: dave, ExpiresAt: time.Now().Add(time.Hour)})
	s.CreateSession(ctx, &models.Session{ID: "expired-session-1", UserID: dave, ExpiresAt: time.Now().Add(-time.Hour)})
	s.CreateSession(ctx, &models.Session{ID: "expired-session-2", UserID: dave, ExpiresAt: time.Now().Add(-2 * time.Hour)})

	if err := s.CleanExpiredSessions(ctx); err != nil {
		t.Fatalf("CleanExpiredSessions failed: %v", err)
	}

	if _, _, err := s.GetSession(ctx, "valid-session"); err != nil {
		t.Errorf("valid session should still exist: %v", err)
	}
	for _, id := range []string{"expired-session-1", "expired-session-2"} {
		if _, _, err := s.GetSession(ctx, id); err == nil {
			t.Errorf("expired session %q should have been cleaned", id)
		}
	}
}

// ===== Dashboard & Metrics =====

func TestGetDashboardSummary_ScopedToUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")

	r1 := seedRepo(t, s, alice, 1, "r1", true)
	r2 := seedRepo(t, s, alice, 2, "r2", false)
	bobRepo := seedRepo(t, s, bob, 3, "bob-repo", true)

	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 1, RepoID: r1.ID, Name: "CI", State: "active"})
	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 2, RepoID: r2.ID, Name: "Deploy", State: "disabled"})
	s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 3, RepoID: bobRepo.ID, Name: "Bob CI", State: "active"})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, RepoID: r1.ID, Status: "completed", StartedAt: time.Now()})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 2, RepoID: bobRepo.ID, Status: "completed", StartedAt: time.Now()})

	summary, err := s.GetDashboardSummary(ctx, alice)
	if err != nil {
		t.Fatalf("GetDashboardSummary failed: %v", err)
	}
	if summary.Repositories.Total != 2 || summary.Repositories.Active != 1 {
		t.Errorf("unexpected repository summary: %+v", summary.Repositories)
	}
	if summary.Workflows.Total != 2 || summary.Workflows.Active != 1 {
		t.Errorf("unexpected workflow summary: %+v", summary.Workflows)
	}
	if summary.Runs.Total != 1 || len(summary.RecentRuns) != 1 || summary.RecentRuns[0].RepoID != r1.ID {
		t.Errorf("alice must only count her runs: %+v recent=%v", summary.Runs, summary.RecentRuns)
	}
}

func TestGetTrends_ScopedToUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")
	repo := seedRepo(t, s, alice, 1, "r", true)
	bobRepo := seedRepo(t, s, bob, 2, "bob-repo", true)

	success := "success"
	failure := "failure"
	now := time.Now()

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 1, RepoID: repo.ID, StartedAt: now, Conclusion: &success})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 2, RepoID: repo.ID, StartedAt: now.Add(-time.Hour), Conclusion: &failure})
	// Older than 7 days: must not appear
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 3, RepoID: repo.ID, StartedAt: now.Add(-8 * 24 * time.Hour), Conclusion: &success})
	// Another user's run: must not appear
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 4, RepoID: bobRepo.ID, StartedAt: now, Conclusion: &success})

	trends, err := s.GetTrends(ctx, alice, 7)
	if err != nil {
		t.Fatalf("GetTrends failed: %v", err)
	}
	total := 0
	for _, t2 := range trends {
		total += t2.TotalRuns
	}
	if total != 2 {
		t.Errorf("expected 2 runs in trends (last 7 days, alice only), got %d", total)
	}
}

func TestBackfillDeploymentRuns_ScopedToUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")
	repo := seedRepo(t, s, alice, 1, "test", true)
	bobRepo := seedRepo(t, s, bob, 2, "bob-test", true)

	wf, _ := s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 1, RepoID: repo.ID, Name: "Release workflow", Path: ".github/workflows/release.yml"})
	bobWf, _ := s.UpsertWorkflow(ctx, &models.Workflow{GitHubID: 2, RepoID: bobRepo.ID, Name: "Release workflow", Path: ".github/workflows/release.yml"})

	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 100, WorkflowID: wf.ID, RepoID: repo.ID, StartedAt: time.Now(), Event: "push"})
	s.UpsertRun(ctx, &models.WorkflowRun{GitHubID: 200, WorkflowID: bobWf.ID, RepoID: bobRepo.ID, StartedAt: time.Now(), Event: "push"})

	updated, err := s.BackfillDeploymentRuns(ctx, alice)
	if err != nil {
		t.Fatalf("BackfillDeploymentRuns failed: %v", err)
	}
	if updated != 1 {
		t.Errorf("expected 1 run updated for alice, got %d", updated)
	}

	stored, _ := s.GetRunByGitHubID(ctx, 100)
	if stored == nil || !stored.IsDeployment {
		t.Error("expected alice's run to have IsDeployment true after backfill")
	}
	bobRun, _ := s.GetRunByGitHubID(ctx, 200)
	if bobRun == nil || bobRun.IsDeployment {
		t.Error("bob's run must be untouched by alice's backfill")
	}
}

func TestListLatestRepositoryScores_ScopedToUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStorage()
	alice := seedUser(t, s, 1, "alice")
	bob := seedUser(t, s, 2, "bob")
	repo := seedRepo(t, s, alice, 1, "r", true)
	bobRepo := seedRepo(t, s, bob, 2, "bob-repo", true)

	s.UpsertRepositoryScore(ctx, &models.RepositoryScore{RepoID: repo.ID, OverallScore: 50, ScannedAt: time.Now().Add(-time.Hour)})
	s.UpsertRepositoryScore(ctx, &models.RepositoryScore{RepoID: repo.ID, OverallScore: 80, ScannedAt: time.Now()})
	s.UpsertRepositoryScore(ctx, &models.RepositoryScore{RepoID: bobRepo.ID, OverallScore: 10, ScannedAt: time.Now()})

	scores, err := s.ListLatestRepositoryScores(ctx, alice)
	if err != nil {
		t.Fatalf("ListLatestRepositoryScores: %v", err)
	}
	if len(scores) != 1 || scores[0].RepoID != repo.ID || scores[0].OverallScore != 80 {
		t.Errorf("alice must see only her latest score, got %+v", scores)
	}
}

// ===== Storage Lifecycle =====

func TestClose(t *testing.T) {
	s := newTestStorage()
	if err := s.Close(); err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

func TestMigrateAndPing(t *testing.T) {
	s := newTestStorage()
	if err := s.Migrate(); err != nil {
		t.Errorf("Migrate should not return error for memory storage: %v", err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Errorf("Ping should not return error for memory storage: %v", err)
	}
}

func TestNewStorage_DatabaseModeFailsClosed(t *testing.T) {
	if _, err := NewStorage(StorageModeDatabase, "", nil); err == nil {
		t.Fatal("database mode without DATABASE_URL must fail instead of falling back to memory")
	}
	if _, err := NewStorage(StorageModeDatabase, "postgresql://localhost:1/db", nil); err == nil {
		t.Fatal("database mode without a cipher must fail")
	}
	if _, err := NewStorage("bogus", "", nil); err == nil {
		t.Fatal("unknown mode must fail")
	}
	store, err := NewStorage(StorageModeMemory, "", nil)
	if err != nil || store == nil {
		t.Fatalf("memory mode must work without a database: %v", err)
	}
}

func TestEscapeLike(t *testing.T) {
	for in, want := range map[string]string{
		"plain":   "plain",
		"50%":     `50\%`,
		"a_b":     `a\_b`,
		`back\sl`: `back\\sl`,
	} {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
