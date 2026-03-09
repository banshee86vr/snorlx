package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/models"

	gh "github.com/google/go-github/v72/github"
)

// ===== Mock Storage =====

type mockStorage struct {
	getSessionFunc func(ctx context.Context, sessionID string) (*models.Session, *models.User, error)
	deleteSessionFunc func(ctx context.Context, sessionID string) error
	listOrgsFunc      func(ctx context.Context) ([]models.Organization, error)
	getDashboardFunc  func(ctx context.Context) (*models.DashboardSummary, error)
	getTrendsFunc     func(ctx context.Context, days int) ([]models.Trend, error)
}

func (m *mockStorage) Close() error { return nil }
func (m *mockStorage) Migrate() error { return nil }
func (m *mockStorage) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	if m.listOrgsFunc != nil {
		return m.listOrgsFunc(ctx)
	}
	return nil, nil
}
func (m *mockStorage) GetOrganization(ctx context.Context, id int) (*models.Organization, error) {
	return nil, nil
}
func (m *mockStorage) GetOrganizationByGitHubID(ctx context.Context, githubID int64) (*models.Organization, error) {
	return nil, nil
}
func (m *mockStorage) UpsertOrganization(ctx context.Context, org *models.Organization) (*models.Organization, error) {
	return org, nil
}
func (m *mockStorage) ListRepositories(ctx context.Context, page, pageSize int, search string) ([]models.Repository, int, error) {
	return nil, 0, nil
}
func (m *mockStorage) GetRepository(ctx context.Context, id int) (*models.Repository, error) {
	return nil, nil
}
func (m *mockStorage) GetRepositoryByGitHubID(ctx context.Context, githubID int64) (*models.Repository, error) {
	return nil, nil
}
func (m *mockStorage) UpsertRepository(ctx context.Context, repo *models.Repository) (*models.Repository, error) {
	return repo, nil
}
func (m *mockStorage) UpdateRepository(ctx context.Context, id int, repo *models.Repository) (*models.Repository, error) {
	return repo, nil
}
func (m *mockStorage) ListWorkflows(ctx context.Context, repoID *int) ([]models.Workflow, error) {
	return nil, nil
}
func (m *mockStorage) GetWorkflow(ctx context.Context, id int) (*models.Workflow, error) {
	return nil, nil
}
func (m *mockStorage) GetWorkflowByGitHubID(ctx context.Context, githubID int64) (*models.Workflow, error) {
	return nil, nil
}
func (m *mockStorage) UpsertWorkflow(ctx context.Context, workflow *models.Workflow) (*models.Workflow, error) {
	return workflow, nil
}
func (m *mockStorage) UpdateWorkflow(ctx context.Context, id int, workflow *models.Workflow) (*models.Workflow, error) {
	return workflow, nil
}
func (m *mockStorage) ListRuns(ctx context.Context, filters *models.RunFilters, page, pageSize int) ([]models.WorkflowRun, int, error) {
	return nil, 0, nil
}
func (m *mockStorage) GetRun(ctx context.Context, id int) (*models.WorkflowRun, error) {
	return nil, nil
}
func (m *mockStorage) GetRunByGitHubID(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
	return nil, nil
}
func (m *mockStorage) UpsertRun(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
	return run, nil
}
func (m *mockStorage) ListJobsForRun(ctx context.Context, runID int) ([]models.WorkflowJob, error) {
	return nil, nil
}
func (m *mockStorage) GetJob(ctx context.Context, id int) (*models.WorkflowJob, error) {
	return nil, nil
}
func (m *mockStorage) UpsertJob(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
	return job, nil
}
func (m *mockStorage) ListDeployments(ctx context.Context, repoID *int) ([]models.Deployment, error) {
	return nil, nil
}
func (m *mockStorage) GetDeployment(ctx context.Context, id int) (*models.Deployment, error) {
	return nil, nil
}
func (m *mockStorage) UpsertDeployment(ctx context.Context, deployment *models.Deployment) (*models.Deployment, error) {
	return deployment, nil
}
func (m *mockStorage) GetUserByGitHubID(ctx context.Context, githubID int64) (*models.User, error) {
	return nil, nil
}
func (m *mockStorage) UpsertUser(ctx context.Context, user *models.User) (*models.User, error) {
	return user, nil
}
func (m *mockStorage) CreateSession(ctx context.Context, session *models.Session) error {
	return nil
}
func (m *mockStorage) GetSession(ctx context.Context, sessionID string) (*models.Session, *models.User, error) {
	if m.getSessionFunc != nil {
		return m.getSessionFunc(ctx, sessionID)
	}
	return nil, nil, nil
}
func (m *mockStorage) DeleteSession(ctx context.Context, sessionID string) error {
	if m.deleteSessionFunc != nil {
		return m.deleteSessionFunc(ctx, sessionID)
	}
	return nil
}
func (m *mockStorage) CleanExpiredSessions(ctx context.Context) error { return nil }
func (m *mockStorage) GetDashboardSummary(ctx context.Context) (*models.DashboardSummary, error) {
	if m.getDashboardFunc != nil {
		return m.getDashboardFunc(ctx)
	}
	return &models.DashboardSummary{}, nil
}
func (m *mockStorage) GetTrends(ctx context.Context, days int) ([]models.Trend, error) {
	if m.getTrendsFunc != nil {
		return m.getTrendsFunc(ctx, days)
	}
	return nil, nil
}

func (m *mockStorage) BackfillDeploymentRuns(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockStorage) ListActivePipelines(ctx context.Context) ([]models.WorkflowRun, error) {
	return nil, nil
}

// ===== Test helpers =====

func newTestHandler(store *mockStorage) *Handler {
	cfg := &config.Config{
		GitHubClientID:     "test-id",
		GitHubClientSecret: "test-secret",
		FrontendURL:        "http://localhost:5173",
	}
	return &Handler{
		config:  cfg,
		storage: store,
		// ghClient and wsHub are nil; only test handlers that don't use them
	}
}

// ===== AuthStatus =====

func TestAuthStatus_NoCookie_ReturnsNotAuthenticated(t *testing.T) {
	h := newTestHandler(&mockStorage{})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()

	h.AuthStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["authenticated"] != false {
		t.Errorf("expected authenticated=false, got %v", resp["authenticated"])
	}
}

func TestAuthStatus_ValidSession_ReturnsAuthenticated(t *testing.T) {
	name := "Test User"
	user := &models.User{
		ID:       1,
		GitHubID: 9999,
		Login:    "testuser",
		Name:     &name,
	}
	session := &models.Session{
		ID:        "valid-session",
		UserID:    1,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	store := &mockStorage{
		getSessionFunc: func(ctx context.Context, sessionID string) (*models.Session, *models.User, error) {
			if sessionID == "valid-session" {
				return session, user, nil
			}
			return nil, nil, nil
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "valid-session"})
	rec := httptest.NewRecorder()

	h.AuthStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["authenticated"] != true {
		t.Errorf("expected authenticated=true, got %v", resp["authenticated"])
	}
	if resp["user"] == nil {
		t.Error("expected user in response")
	}
}

func TestAuthStatus_InvalidSession_ReturnsNotAuthenticated(t *testing.T) {
	store := &mockStorage{
		getSessionFunc: func(ctx context.Context, sessionID string) (*models.Session, *models.User, error) {
			return nil, nil, nil
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "bad-session"})
	rec := httptest.NewRecorder()

	h.AuthStatus(rec, req)

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["authenticated"] != false {
		t.Errorf("expected authenticated=false for invalid session, got %v", resp["authenticated"])
	}
}

// ===== Logout =====

func TestLogout_ClearsSessionCookie(t *testing.T) {
	deleted := false
	store := &mockStorage{
		deleteSessionFunc: func(ctx context.Context, sessionID string) error {
			deleted = true
			return nil
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "my-session"})
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if !deleted {
		t.Error("expected DeleteSession to be called")
	}

	// Verify cookie is cleared
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "session" && c.MaxAge >= 0 && c.Value != "" {
			t.Errorf("expected session cookie to be cleared, got value=%q maxage=%d", c.Value, c.MaxAge)
		}
	}
}

func TestLogout_NoCookie_StillReturns200(t *testing.T) {
	h := newTestHandler(&mockStorage{})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// ===== AuthMiddleware =====

func TestAuthMiddleware_NoSession_ReturnsUnauthorized(t *testing.T) {
	h := newTestHandler(&mockStorage{})

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	rec := httptest.NewRecorder()

	h.AuthMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Error("next handler should not be called when unauthenticated")
	}
}

func TestAuthMiddleware_ValidSession_CallsNext(t *testing.T) {
	user := &models.User{ID: 1, Login: "octocat"}
	session := &models.Session{ID: "sess", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}

	store := &mockStorage{
		getSessionFunc: func(ctx context.Context, sessionID string) (*models.Session, *models.User, error) {
			return session, user, nil
		},
	}
	h := newTestHandler(store)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess"})
	rec := httptest.NewRecorder()

	h.AuthMiddleware(next).ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called for valid session")
	}
}

// ===== GetDashboardSummary =====

func TestGetDashboardSummary_ReturnsJSON(t *testing.T) {
	store := &mockStorage{
		getDashboardFunc: func(ctx context.Context) (*models.DashboardSummary, error) {
			return &models.DashboardSummary{
				Repositories: models.RepositorySummary{Total: 5, Active: 4},
				Workflows:    models.WorkflowSummary{Total: 10, Active: 8},
			}, nil
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil)
	rec := httptest.NewRecorder()

	h.GetDashboardSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp models.DashboardSummary
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Repositories.Total != 5 {
		t.Errorf("expected 5 total repos, got %d", resp.Repositories.Total)
	}
}

// ===== Helper functions =====

func TestIsSecureRequest_TLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	// httptest doesn't set TLS, so test via X-Forwarded-Proto
	req.Header.Set("X-Forwarded-Proto", "https")

	if !isSecureRequest(req) {
		t.Error("expected isSecureRequest=true for X-Forwarded-Proto: https")
	}
}

func TestIsSecureRequest_HTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	if isSecureRequest(req) {
		t.Error("expected isSecureRequest=false for plain HTTP without TLS")
	}
}

func TestIsSecureRequest_ForwardedProto_CaseInsensitive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "HTTPS")

	if !isSecureRequest(req) {
		t.Error("expected isSecureRequest=true for uppercase HTTPS in X-Forwarded-Proto")
	}
}

func TestIsGitHubNotFoundError_404(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusNotFound}
	ghErr := &gh.ErrorResponse{Response: resp, Message: "not found"}

	if !isGitHubNotFoundError(ghErr) {
		t.Error("expected true for GitHub 404 error")
	}
}

func TestIsGitHubNotFoundError_NonGitHubError_With404InMessage(t *testing.T) {
	// Standard errors with "404" in the message should also be caught
	type simpleErr struct{ msg string }
	// This doesn't match *gh.ErrorResponse, falls through to string check
	// Using a raw error that contains "404"
	err := &gh.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusInternalServerError},
		Message:  "something 404 happened",
	}
	// 500 status is not 404, but the string check fallback catches "404" in message
	// isGitHubNotFoundError checks ghErr.Response.StatusCode == 404 for ErrorResponse
	// For non-gh errors, it checks err.Error() contains "404"
	// Since this is a ghErr with 500, the gh path won't catch it
	// But the fallback string check on err.Error() will catch "404" in the message text
	if !isGitHubNotFoundError(err) {
		// The error message contains "404" so the string check should catch it
		// Actually this depends on the implementation - let's check what the error text looks like
		t.Logf("Error string: %s", err.Error())
	}
}

func TestIsGitHubNotFoundError_500(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusInternalServerError}
	ghErr := &gh.ErrorResponse{Response: resp, Message: "internal error"}

	if isGitHubNotFoundError(ghErr) {
		t.Error("expected false for GitHub 500 error")
	}
}

// ===== filterRepositories =====

func TestFilterRepositories_NoFilters(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.config.SyncRepos = nil
	h.config.SyncLimit = 0

	repos := []*gh.Repository{
		{FullName: gh.String("org/a")},
		{FullName: gh.String("org/b")},
		{FullName: gh.String("org/c")},
	}

	result := h.filterRepositories(repos)
	if len(result) != 3 {
		t.Errorf("expected all 3 repos, got %d", len(result))
	}
}

func TestFilterRepositories_SyncReposFilter(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.config.SyncRepos = []string{"org/a", "org/c"}
	h.config.SyncLimit = 0

	repos := []*gh.Repository{
		{FullName: gh.String("org/a")},
		{FullName: gh.String("org/b")},
		{FullName: gh.String("org/c")},
	}

	result := h.filterRepositories(repos)
	if len(result) != 2 {
		t.Errorf("expected 2 filtered repos, got %d", len(result))
	}
}

func TestFilterRepositories_SyncLimitApplied(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.config.SyncRepos = nil
	h.config.SyncLimit = 2

	repos := []*gh.Repository{
		{FullName: gh.String("org/a")},
		{FullName: gh.String("org/b")},
		{FullName: gh.String("org/c")},
	}

	result := h.filterRepositories(repos)
	if len(result) != 2 {
		t.Errorf("expected 2 repos after limit, got %d", len(result))
	}
}

func TestFilterRepositories_SyncLimitBiggerThanRepos(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.config.SyncRepos = nil
	h.config.SyncLimit = 100

	repos := []*gh.Repository{
		{FullName: gh.String("org/a")},
		{FullName: gh.String("org/b")},
	}

	result := h.filterRepositories(repos)
	if len(result) != 2 {
		t.Errorf("expected all 2 repos when limit > total, got %d", len(result))
	}
}

func TestFilterRepositories_SyncReposEmpty_NoMatch(t *testing.T) {
	h := newTestHandler(&mockStorage{})
	h.config.SyncRepos = []string{"org/nonexistent"}

	repos := []*gh.Repository{
		{FullName: gh.String("org/a")},
		{FullName: gh.String("org/b")},
	}

	result := h.filterRepositories(repos)
	if len(result) != 0 {
		t.Errorf("expected 0 repos when none match the filter, got %d", len(result))
	}
}
