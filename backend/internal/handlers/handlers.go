package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/github"
	"snorlx/backend/internal/models"
	"snorlx/backend/internal/storage"
	"snorlx/backend/internal/websocket"

	"github.com/go-chi/chi/v5"
	gh "github.com/google/go-github/v60/github"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// Handler contains all HTTP handlers
type Handler struct {
	config   *config.Config
	storage  storage.Storage
	ghClient *github.Client
	wsHub    *websocket.Hub
}

// New creates a new Handler
func New(cfg *config.Config, store storage.Storage, ghClient *github.Client, wsHub *websocket.Hub) *Handler {
	return &Handler{
		config:   cfg,
		storage:  store,
		ghClient: ghClient,
		wsHub:    wsHub,
	}
}

// Context key for user
type contextKey string

const userContextKey contextKey = "user"

// ===== Auth Handlers =====

// Login initiates GitHub OAuth
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	state := generateState()

	// Store state in cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	url := h.ghClient.GetAuthURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Callback handles GitHub OAuth callback
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	// Exchange code for token
	code := r.URL.Query().Get("code")
	token, err := h.ghClient.ExchangeCode(r.Context(), code)
	if err != nil {
		log.Error().Err(err).Msg("Failed to exchange code")
		http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		return
	}

	// Get user info
	client := h.ghClient.GetUserClient(r.Context(), token)
	ghUser, err := h.ghClient.GetUser(r.Context(), client)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user info")
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Save or update user
	ghUser.AccessToken = token.AccessToken
	ghUser.TokenExpiresAt = &token.Expiry
	user, err := h.storage.UpsertUser(r.Context(), ghUser)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save user")
		http.Error(w, "Failed to save user", http.StatusInternalServerError)
		return
	}

	// Create session
	sessionID := generateSessionID()
	expiresAt := time.Now().Add(24 * time.Hour * 7) // 7 days

	session := &models.Session{
		ID:        sessionID,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	}
	if err := h.storage.CreateSession(r.Context(), session); err != nil {
		log.Error().Err(err).Msg("Failed to create session")
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to frontend
	http.Redirect(w, r, h.config.FrontendURL, http.StatusTemporaryRedirect)
}

// Logout logs out the user
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	sessionCookie, err := r.Cookie("session")
	if err == nil {
		h.storage.DeleteSession(r.Context(), sessionCookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	w.WriteHeader(http.StatusOK)
}

// AuthStatus returns the current authentication status
func (h *Handler) AuthStatus(w http.ResponseWriter, r *http.Request) {
	// Check session cookie directly (this endpoint is not behind AuthMiddleware)
	sessionCookie, err := r.Cookie("session")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Get session from storage
	_, user, err := h.storage.GetSession(r.Context(), sessionCookie.Value)
	if err != nil || user == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"user":          user,
	})
}

// AuthMiddleware checks if the user is authenticated
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionCookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Get session from storage
		_, user, err := h.storage.GetSession(r.Context(), sessionCookie.Value)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ===== Webhook Handler =====

// HandleWebhook processes GitHub webhook events
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read payload
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read payload", http.StatusBadRequest)
		return
	}

	// Validate signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if !h.ghClient.ValidateWebhookSignature(payload, signature) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse event
	eventType := r.Header.Get("X-GitHub-Event")
	event, err := h.ghClient.ParseWebhookEvent(eventType, payload)
	if err != nil {
		log.Warn().Str("event_type", eventType).Msg("Unsupported webhook event")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process event
	go h.processWebhookEvent(eventType, event)

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) processWebhookEvent(eventType string, event interface{}) {
	ctx := context.Background()

	switch e := event.(type) {
	case *gh.WorkflowRunEvent:
		log.Info().
			Str("action", e.GetAction()).
			Int64("run_id", e.GetWorkflowRun().GetID()).
			Msg("Processing workflow_run event")

		run := h.convertWorkflowRun(e.GetWorkflowRun(), e.GetRepo())
		if _, err := h.storage.UpsertRun(ctx, run); err != nil {
			log.Error().Err(err).Msg("Failed to save workflow run")
			return
		}

		// Broadcast update via WebSocket
		h.wsHub.BroadcastWorkflowRunUpdate(run)

	case *gh.WorkflowJobEvent:
		log.Info().
			Str("action", e.GetAction()).
			Int64("job_id", e.GetWorkflowJob().GetID()).
			Msg("Processing workflow_job event")

		// Broadcast update via WebSocket
		h.wsHub.BroadcastWorkflowJobUpdate(e.GetWorkflowJob())

	case *gh.DeploymentEvent:
		log.Info().
			Int64("deployment_id", e.GetDeployment().GetID()).
			Msg("Processing deployment event")

		h.wsHub.BroadcastDeploymentUpdate(e.GetDeployment())

	case *gh.DeploymentStatusEvent:
		log.Info().
			Int64("deployment_id", e.GetDeployment().GetID()).
			Str("status", e.GetDeploymentStatus().GetState()).
			Msg("Processing deployment_status event")

		h.wsHub.BroadcastDeploymentUpdate(e.GetDeployment())
	}
}

// ===== WebSocket Handler =====

// WebSocketHandler handles WebSocket connections
func (h *Handler) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := h.getUserFromContext(r.Context())
	userID := 0
	if user != nil {
		userID = user.ID
	}

	// Upgrade HTTP connection to WebSocket
	upgrader := websocket.GetUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade to WebSocket")
		return
	}

	// Create client
	client := websocket.NewClient(generateState(), userID, h.wsHub, conn)

	// Register client
	h.wsHub.Register(client)

	// Start read and write pumps
	go client.WritePump()
	go client.ReadPump()
}

// ===== Organization Handlers =====

// ListOrganizations lists all organizations
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.storage.ListOrganizations(r.Context())
	if err != nil {
		http.Error(w, "Failed to fetch organizations", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(orgs)
}

// GetOrganization gets a single organization
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	org, err := h.storage.GetOrganization(r.Context(), id)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(org)
}

// ===== Repository Handlers =====

// ListRepositories lists all repositories
func (h *Handler) ListRepositories(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 20
	search := r.URL.Query().Get("search")

	repos, total, err := h.storage.ListRepositories(r.Context(), page, pageSize, search)
	if err != nil {
		http.Error(w, "Failed to fetch repositories", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(models.ListResponse[models.Repository]{
		Data: repos,
		Pagination: models.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		},
	})
}

// GetRepository gets a single repository
func (h *Handler) GetRepository(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	repo, err := h.storage.GetRepository(r.Context(), id)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(repo)
}

// filterRepositories filters repositories based on config settings (SYNC_REPOS and SYNC_LIMIT)
func (h *Handler) filterRepositories(ghRepos []*gh.Repository) []*gh.Repository {
	// If specific repos are configured, filter to only those
	if len(h.config.SyncRepos) > 0 {
		repoSet := make(map[string]bool)
		for _, repo := range h.config.SyncRepos {
			repoSet[repo] = true
		}

		var filtered []*gh.Repository
		for _, repo := range ghRepos {
			if repoSet[repo.GetFullName()] {
				filtered = append(filtered, repo)
			}
		}
		ghRepos = filtered
		log.Info().Int("filtered", len(filtered)).Strs("repos", h.config.SyncRepos).Msg("Filtered to specific repositories")
	}

	// Apply limit if configured
	if h.config.SyncLimit > 0 && len(ghRepos) > h.config.SyncLimit {
		log.Info().Int("limit", h.config.SyncLimit).Int("total", len(ghRepos)).Msg("Limiting repositories to sync")
		ghRepos = ghRepos[:h.config.SyncLimit]
	}

	return ghRepos
}

// SyncRepositories starts a background sync of repositories from GitHub
// Returns immediately with 202 Accepted, progress is sent via WebSocket
func (h *Handler) SyncRepositories(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Copy user token for background goroutine
	accessToken := user.AccessToken

	// Return immediately - sync runs in background
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "started",
		"message": "Sync started, progress will be sent via WebSocket",
	})

	// Run sync in background goroutine
	go h.runSync(accessToken)
}

// runSync performs the actual sync operation in the background
func (h *Handler) runSync(accessToken string) {
	ctx := context.Background()

	// Create GitHub client with user's token
	token := &oauth2.Token{AccessToken: accessToken}
	client := h.ghClient.GetUserClient(ctx, token)

	// Fetch user's repositories from GitHub
	ghRepos, err := h.ghClient.ListUserRepositories(ctx, client)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch repositories from GitHub")
		h.wsHub.BroadcastSyncError("Failed to fetch repositories from GitHub")
		return
	}

	// Apply filters from config
	ghRepos = h.filterRepositories(ghRepos)

	total := len(ghRepos)
	syncedRepos := 0
	syncedWorkflows := 0
	syncedRuns := 0

	// Broadcast sync start
	h.wsHub.BroadcastSyncStart(total)

	for i, ghRepo := range ghRepos {
		// Broadcast progress
		h.wsHub.BroadcastSyncProgress(i, total, ghRepo.GetFullName())

		// Convert and save repository
		repo := &models.Repository{
			GitHubID:      ghRepo.GetID(),
			Name:          ghRepo.GetName(),
			FullName:      ghRepo.GetFullName(),
			DefaultBranch: ghRepo.GetDefaultBranch(),
			HTMLURL:       ghRepo.GetHTMLURL(),
			IsPrivate:     ghRepo.GetPrivate(),
			IsActive:      true,
		}
		if ghRepo.Description != nil {
			repo.Description = ghRepo.Description
		}

		savedRepo, err := h.storage.UpsertRepository(ctx, repo)
		if err != nil {
			log.Error().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to save repository")
			continue
		}
		syncedRepos++

		// Fetch and save workflows for this repository
		owner := ghRepo.GetOwner().GetLogin()
		repoName := ghRepo.GetName()

		ghWorkflows, err := h.ghClient.ListWorkflows(ctx, client, owner, repoName)
		if err != nil {
			log.Warn().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to fetch workflows")
			continue
		}

		// Build a map of GitHub workflow ID to internal workflow ID
		workflowIDMap := make(map[int64]int)

		for _, ghWorkflow := range ghWorkflows {
			workflow := &models.Workflow{
				GitHubID: ghWorkflow.GetID(),
				RepoID:   savedRepo.ID,
				Name:     ghWorkflow.GetName(),
				Path:     ghWorkflow.GetPath(),
				State:    ghWorkflow.GetState(),
			}
			if ghWorkflow.BadgeURL != nil {
				workflow.BadgeURL = ghWorkflow.BadgeURL
			}
			if ghWorkflow.HTMLURL != nil {
				workflow.HTMLURL = ghWorkflow.HTMLURL
			}

			savedWorkflow, err := h.storage.UpsertWorkflow(ctx, workflow)
			if err != nil {
				log.Error().Err(err).Str("workflow", ghWorkflow.GetName()).Msg("Failed to save workflow")
				continue
			}
			syncedWorkflows++
			workflowIDMap[ghWorkflow.GetID()] = savedWorkflow.ID
		}

		// Fetch and save workflow runs for this repository
		// Use 200 runs to ensure we have enough history for the "Previous 30 days" comparison
		ghRuns, err := h.ghClient.ListWorkflowRuns(ctx, client, owner, repoName, nil, 200)
		if err != nil {
			log.Warn().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to fetch workflow runs")
			continue
		}

		for _, ghRun := range ghRuns {
			// Look up the internal workflow ID
			workflowID, ok := workflowIDMap[ghRun.GetWorkflowID()]
			if !ok {
				log.Debug().Int64("github_workflow_id", ghRun.GetWorkflowID()).Msg("Skipping run - workflow not found in map")
				continue
			}

			startedAt := ghRun.GetRunStartedAt().Time
			log.Debug().
				Int64("run_id", ghRun.GetID()).
				Time("started_at", startedAt).
				Str("status", ghRun.GetStatus()).
				Msg("Syncing run")

			run := &models.WorkflowRun{
				GitHubID:   ghRun.GetID(),
				WorkflowID: workflowID,
				RepoID:     savedRepo.ID,
				RunNumber:  ghRun.GetRunNumber(),
				Name:       ghRun.GetName(),
				Status:     ghRun.GetStatus(),
				Event:      ghRun.GetEvent(),
				Branch:     ghRun.GetHeadBranch(),
				CommitSHA:  ghRun.GetHeadSHA(),
				ActorLogin: ghRun.GetActor().GetLogin(),
				HTMLURL:    ghRun.GetHTMLURL(),
				StartedAt:  startedAt,
			}

			if ghRun.Conclusion != nil {
				run.Conclusion = ghRun.Conclusion
			}
			if ghRun.GetActor() != nil {
				avatar := ghRun.GetActor().GetAvatarURL()
				run.ActorAvatar = &avatar
			}
			if !ghRun.GetUpdatedAt().IsZero() && ghRun.GetStatus() == "completed" {
				completedAt := ghRun.GetUpdatedAt().Time
				run.CompletedAt = &completedAt
				duration := int(completedAt.Sub(run.StartedAt).Seconds())
				run.DurationSeconds = &duration
			}

			if _, err := h.storage.UpsertRun(ctx, run); err != nil {
				log.Error().Err(err).Int64("run_id", ghRun.GetID()).Msg("Failed to save workflow run")
				continue
			}
			syncedRuns++
		}
	}

	log.Info().
		Int("repositories", syncedRepos).
		Int("workflows", syncedWorkflows).
		Int("runs", syncedRuns).
		Msg("Sync completed")

	// Broadcast sync complete
	h.wsHub.BroadcastSyncComplete(syncedRepos, syncedWorkflows, syncedRuns)
}

// ===== Workflow Handlers =====

// ListWorkflows lists all workflows
func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	var repoID *int
	if repoIDStr := r.URL.Query().Get("repo_id"); repoIDStr != "" {
		id, _ := strconv.Atoi(repoIDStr)
		repoID = &id
	}

	workflows, err := h.storage.ListWorkflows(r.Context(), repoID)
	if err != nil {
		http.Error(w, "Failed to fetch workflows", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(workflows)
}

// GetWorkflow gets a single workflow
func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	wf, err := h.storage.GetWorkflow(r.Context(), id)
	if err != nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(wf)
}

// GetWorkflowRuns gets runs for a workflow
func (h *Handler) GetWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	h.listRunsWithFilter(w, r, &models.RunFilters{WorkflowID: id})
}

// ===== Run Handlers =====

// ListRuns lists all workflow runs
func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	h.listRunsWithFilter(w, r, nil)
}

func (h *Handler) listRunsWithFilter(w http.ResponseWriter, r *http.Request, baseFilters *models.RunFilters) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 50

	// Build filters
	filters := &models.RunFilters{}
	if baseFilters != nil {
		*filters = *baseFilters
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}
	if conclusion := r.URL.Query().Get("conclusion"); conclusion != "" {
		filters.Conclusion = conclusion
	}
	if branch := r.URL.Query().Get("branch"); branch != "" {
		filters.Branch = branch
	}

	runs, total, err := h.storage.ListRuns(r.Context(), filters, page, pageSize)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch runs")
		http.Error(w, "Failed to fetch runs", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(models.ListResponse[models.WorkflowRun]{
		Data: runs,
		Pagination: models.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		},
	})
}

// GetRun gets a single run
func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(run)
}

// GetRunJobs gets jobs for a run - fetches from GitHub on-demand
func (h *Handler) GetRunJobs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// First, try to get cached jobs from storage
	jobs, err := h.storage.ListJobsForRun(r.Context(), id)
	if err == nil && len(jobs) > 0 {
		json.NewEncoder(w).Encode(jobs)
		return
	}

	// No cached jobs, fetch from GitHub
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the run to find GitHub ID and repo
	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	// Get the repository to find owner/repo name
	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Parse owner and repo name from full_name (e.g., "owner/repo")
	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	// Create GitHub client with user's token
	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	// Fetch jobs from GitHub
	ghJobs, err := h.ghClient.ListWorkflowJobs(r.Context(), client, owner, repoName, run.GitHubID)
	if err != nil {
		log.Error().Err(err).Int64("run_github_id", run.GitHubID).Msg("Failed to fetch jobs from GitHub")
		// Return empty array instead of error if GitHub fetch fails
		json.NewEncoder(w).Encode([]models.WorkflowJob{})
		return
	}

	// Convert and save jobs
	var savedJobs []models.WorkflowJob
	for _, ghJob := range ghJobs {
		job := h.convertWorkflowJob(ghJob, id)
		if _, err := h.storage.UpsertJob(r.Context(), job); err != nil {
			log.Error().Err(err).Int64("job_id", ghJob.GetID()).Msg("Failed to save job")
			continue
		}
		savedJobs = append(savedJobs, *job)
	}

	json.NewEncoder(w).Encode(savedJobs)
}

// GetRunLogs gets logs URL for a run
func (h *Handler) GetRunLogs(w http.ResponseWriter, r *http.Request) {
	// For now, return a placeholder
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Logs retrieval requires GitHub App installation token",
	})
}

// GetRunAnnotations fetches annotations for a run from GitHub
func (h *Handler) GetRunAnnotations(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the run to find GitHub ID and repo
	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	// Get the repository to find owner/repo name
	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Parse owner and repo name from full_name
	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	// Create GitHub client with user's token
	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	// Fetch annotations from GitHub
	annotations, err := h.ghClient.GetWorkflowRunAnnotations(r.Context(), client, owner, repoName, run.GitHubID)
	if err != nil {
		log.Error().Err(err).Int64("run_github_id", run.GitHubID).Msg("Failed to fetch run annotations from GitHub")
		http.Error(w, "Failed to fetch annotations", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(annotations)
}

// GetJobLogs fetches logs for a specific job from GitHub
func (h *Handler) GetJobLogs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the job from storage
	job, err := h.storage.GetJob(r.Context(), id)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Get the run to find the repo
	run, err := h.storage.GetRun(r.Context(), job.RunID)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	// Get the repository
	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Parse owner and repo name
	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	// Create GitHub client with user's token
	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	// Fetch job logs URL from GitHub
	logsURL, err := h.ghClient.GetWorkflowJobLogs(r.Context(), client, owner, repoName, job.GitHubID)
	if err != nil {
		log.Error().Err(err).Int64("job_github_id", job.GitHubID).Msg("Failed to fetch job logs from GitHub")
		http.Error(w, "Failed to fetch job logs", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"url": logsURL,
	})
}

// WorkflowDefinition represents the parsed workflow YAML structure
type WorkflowDefinition struct {
	Name string                            `yaml:"name" json:"name"`
	Jobs map[string]WorkflowJobDefinition `yaml:"jobs" json:"jobs"`
}

// WorkflowJobDefinition represents a job in the workflow YAML
type WorkflowJobDefinition struct {
	Name     string                   `yaml:"name,omitempty" json:"name,omitempty"`
	Needs    interface{}              `yaml:"needs,omitempty" json:"needs,omitempty"` // Can be string or []string
	Uses     string                   `yaml:"uses,omitempty" json:"uses,omitempty"`   // For reusable workflows
	Strategy *WorkflowStrategyDefinition `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

// WorkflowStrategyDefinition represents the strategy section of a job
type WorkflowStrategyDefinition struct {
	Matrix map[string]interface{} `yaml:"matrix,omitempty" json:"matrix,omitempty"`
}

// JobDependency represents a job and its dependencies for the frontend
type JobDependency struct {
	JobID    string   `json:"job_id"`    // The job key in the YAML (e.g., "build", "test")
	Name     string   `json:"name"`      // The display name (from 'name' field or job_id)
	Needs    []string `json:"needs"`     // List of job IDs this job depends on
	IsMatrix bool     `json:"is_matrix"` // Whether this job uses a matrix strategy
	Prefix   string   `json:"prefix"`    // Prefix for job names (e.g., calling job name for reusable workflows)
}

// GetRunWorkflowDefinition fetches and parses the workflow YAML to extract job dependencies
func (h *Handler) GetRunWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the run to find workflow info
	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	// Get the workflow to find the file path
	workflow, err := h.storage.GetWorkflow(r.Context(), run.WorkflowID)
	if err != nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	// Get the repository
	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Parse owner and repo name
	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	// Create GitHub client with user's token
	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	// Fetch the workflow file content using the commit SHA from the run
	content, err := h.ghClient.GetWorkflowContent(r.Context(), client, owner, repoName, workflow.Path, run.CommitSHA)
	if err != nil {
		log.Error().Err(err).Str("path", workflow.Path).Str("sha", run.CommitSHA).Msg("Failed to fetch workflow content")
		// Return empty dependencies on error
		json.NewEncoder(w).Encode([]JobDependency{})
		return
	}

	// Parse the YAML
	var workflowDef WorkflowDefinition
	if err := yaml.Unmarshal(content, &workflowDef); err != nil {
		log.Error().Err(err).Msg("Failed to parse workflow YAML")
		json.NewEncoder(w).Encode([]JobDependency{})
		return
	}

	log.Debug().
		Str("workflow_path", workflow.Path).
		Int("job_count", len(workflowDef.Jobs)).
		Str("workflow_name", workflowDef.Name).
		Msg("Parsed workflow definition")

	// Extract job dependencies, handling reusable workflows
	dependencies := make([]JobDependency, 0, len(workflowDef.Jobs))
	
	for jobID, jobDef := range workflowDef.Jobs {
		// Check if this job uses a reusable workflow
		if jobDef.Uses != "" {
			log.Debug().
				Str("job_id", jobID).
				Str("uses", jobDef.Uses).
				Msg("Job uses reusable workflow")
			
			// Parse the reusable workflow reference
			ref := parseReusableWorkflowRef(jobDef.Uses)
			if ref == nil {
				log.Warn().Str("uses", jobDef.Uses).Msg("Could not parse reusable workflow reference")
			} else {
				log.Debug().
					Str("uses", jobDef.Uses).
					Str("owner", ref.Owner).
					Str("repo", ref.Repo).
					Str("path", ref.Path).
					Str("ref", ref.Ref).
					Bool("is_local", ref.IsLocal).
					Msg("Parsed reusable workflow reference")
				
				var reusableContent []byte
				var fetchErr error
				
				if ref.IsLocal {
					// Local workflow - use current repo
					reusableContent, fetchErr = h.ghClient.GetWorkflowContent(r.Context(), client, owner, repoName, ref.Path, run.CommitSHA)
				} else {
					// External workflow - use referenced repo
					// Use the ref from the uses clause, or fall back to default branch
					fetchRef := ref.Ref
					if fetchRef == "" {
						fetchRef = "main" // Default fallback
					}
					reusableContent, fetchErr = h.ghClient.GetWorkflowContent(r.Context(), client, ref.Owner, ref.Repo, ref.Path, fetchRef)
				}
				
				if fetchErr != nil {
					log.Warn().Err(fetchErr).
						Str("path", ref.Path).
						Str("owner", ref.Owner).
						Str("repo", ref.Repo).
						Msg("Failed to fetch reusable workflow content")
				} else {
					var reusableDef WorkflowDefinition
					if err := yaml.Unmarshal(reusableContent, &reusableDef); err != nil {
						log.Warn().Err(err).Str("path", ref.Path).Msg("Failed to parse reusable workflow YAML")
					} else {
						log.Debug().
							Str("reusable_path", ref.Path).
							Int("job_count", len(reusableDef.Jobs)).
							Msg("Parsed reusable workflow definition")
						
						// Add jobs from the reusable workflow with the calling job as prefix
						for reusableJobID, reusableJobDef := range reusableDef.Jobs {
							dep := JobDependency{
								JobID:    reusableJobID,
								Name:     reusableJobDef.Name,
								Needs:    []string{},
								IsMatrix: reusableJobDef.Strategy != nil && reusableJobDef.Strategy.Matrix != nil,
								Prefix:   jobID, // The calling job ID becomes the prefix
							}

							if dep.Name == "" {
								dep.Name = reusableJobID
							}

							// Parse needs
							switch needs := reusableJobDef.Needs.(type) {
							case string:
								if needs != "" {
									dep.Needs = []string{needs}
								}
							case []interface{}:
								for _, n := range needs {
									if s, ok := n.(string); ok {
										dep.Needs = append(dep.Needs, s)
									}
								}
							}
							
							dependencies = append(dependencies, dep)
						}
						continue // Skip adding the calling job itself
					}
				}
			}
		}

		// Regular job (not a reusable workflow or failed to fetch reusable)
		dep := JobDependency{
			JobID:    jobID,
			Name:     jobDef.Name,
			Needs:    []string{},
			IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
			Prefix:   "", // No prefix for regular jobs
		}

		// Use jobID as name if name is not set
		if dep.Name == "" {
			dep.Name = jobID
		}

		// Parse needs - can be a single string or an array of strings
		switch needs := jobDef.Needs.(type) {
		case string:
			if needs != "" {
				dep.Needs = []string{needs}
			}
		case []interface{}:
			for _, n := range needs {
				if s, ok := n.(string); ok {
					dep.Needs = append(dep.Needs, s)
				}
			}
		}

		dependencies = append(dependencies, dep)
	}

	log.Debug().Int("total_dependencies", len(dependencies)).Msg("Returning job dependencies")
	json.NewEncoder(w).Encode(dependencies)
}

// ReusableWorkflowRef represents a parsed reusable workflow reference
type ReusableWorkflowRef struct {
	Owner    string
	Repo     string
	Path     string
	Ref      string
	IsLocal  bool
}

// parseReusableWorkflowRef parses a 'uses' reference into its components
func parseReusableWorkflowRef(uses string) *ReusableWorkflowRef {
	// Handle local workflow references: ./.github/workflows/foo.yaml
	if strings.HasPrefix(uses, "./") {
		return &ReusableWorkflowRef{
			Path:    strings.TrimPrefix(uses, "./"),
			IsLocal: true,
		}
	}
	
	// Handle same-repo references without ./ prefix
	if strings.HasPrefix(uses, ".github/workflows/") {
		path := uses
		ref := ""
		if idx := strings.Index(path, "@"); idx != -1 {
			ref = path[idx+1:]
			path = path[:idx]
		}
		return &ReusableWorkflowRef{
			Path:    path,
			Ref:     ref,
			IsLocal: true,
		}
	}
	
	// External workflows: owner/repo/.github/workflows/file.yaml@ref
	// or owner/repo/path/to/workflow.yml@ref
	ref := ""
	pathWithRef := uses
	if idx := strings.Index(uses, "@"); idx != -1 {
		ref = uses[idx+1:]
		pathWithRef = uses[:idx]
	}
	
	// Split into owner/repo and path
	parts := strings.SplitN(pathWithRef, "/", 3)
	if len(parts) >= 3 {
		return &ReusableWorkflowRef{
			Owner:   parts[0],
			Repo:    parts[1],
			Path:    parts[2],
			Ref:     ref,
			IsLocal: false,
		}
	}
	
	return nil
}

// resolveReusableWorkflowPath converts a 'uses' reference to a file path (for local refs only)
func (h *Handler) resolveReusableWorkflowPath(uses string, callerPath string) string {
	ref := parseReusableWorkflowRef(uses)
	if ref == nil {
		return ""
	}
	if ref.IsLocal {
		return ref.Path
	}
	// External workflows need special handling with owner/repo
	return ""
}

// RerunWorkflow reruns a workflow
func (h *Handler) RerunWorkflow(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// CancelRun cancels a workflow run
func (h *Handler) CancelRun(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ===== DevOps Metrics Handlers =====

// GetDevOpsMetrics returns all DevOps performance metrics
func (h *Handler) GetDevOpsMetrics(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	days := 30
	switch period {
	case "7d":
		days = 7
	case "30d":
		days = 30
	case "90d":
		days = 90
	}

	startDate := time.Now().AddDate(0, 0, -days)
	endDate := time.Now()

	metrics, err := h.storage.GetDevOpsMetrics(r.Context(), startDate, endDate)
	if err != nil {
		http.Error(w, "Failed to fetch DevOps metrics", http.StatusInternalServerError)
		return
	}

	metrics.Period = period
	json.NewEncoder(w).Encode(metrics)
}

// GetDeploymentFrequency returns deployment frequency metric
func (h *Handler) GetDeploymentFrequency(w http.ResponseWriter, r *http.Request) {
	h.GetDevOpsMetrics(w, r)
}

// GetLeadTime returns lead time metric
func (h *Handler) GetLeadTime(w http.ResponseWriter, r *http.Request) {
	h.GetDevOpsMetrics(w, r)
}

// GetChangeFailureRate returns change failure rate metric
func (h *Handler) GetChangeFailureRate(w http.ResponseWriter, r *http.Request) {
	h.GetDevOpsMetrics(w, r)
}

// GetMTTR returns MTTR metric
func (h *Handler) GetMTTR(w http.ResponseWriter, r *http.Request) {
	h.GetDevOpsMetrics(w, r)
}

// ===== Dashboard Handlers =====

// GetDashboardSummary returns dashboard summary
func (h *Handler) GetDashboardSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.storage.GetDashboardSummary(r.Context())
	if err != nil {
		http.Error(w, "Failed to fetch dashboard summary", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(summary)
}

// GetTrends returns trend data
func (h *Handler) GetTrends(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}

	trends, err := h.storage.GetTrends(r.Context(), days)
	if err != nil {
		http.Error(w, "Failed to fetch trends", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"trends": trends,
	})
}

// ===== Helper Functions =====

func (h *Handler) getUserFromContext(ctx context.Context) *models.User {
	user, _ := ctx.Value(userContextKey).(*models.User)
	return user
}

func (h *Handler) convertWorkflowRun(run *gh.WorkflowRun, repo *gh.Repository) *models.WorkflowRun {
	result := &models.WorkflowRun{
		GitHubID:    run.GetID(),
		RunNumber:   run.GetRunNumber(),
		Name:        run.GetName(),
		Status:      run.GetStatus(),
		Event:       run.GetEvent(),
		Branch:      run.GetHeadBranch(),
		CommitSHA:   run.GetHeadSHA(),
		ActorLogin:  run.GetActor().GetLogin(),
		HTMLURL:     run.GetHTMLURL(),
		StartedAt:   run.GetRunStartedAt().Time,
	}

	if run.Conclusion != nil {
		result.Conclusion = run.Conclusion
	}
	if run.GetActor() != nil {
		avatar := run.GetActor().GetAvatarURL()
		result.ActorAvatar = &avatar
	}
	if !run.GetUpdatedAt().IsZero() {
		completedAt := run.GetUpdatedAt().Time
		result.CompletedAt = &completedAt
		duration := int(completedAt.Sub(result.StartedAt).Seconds())
		result.DurationSeconds = &duration
	}

	return result
}

func (h *Handler) convertWorkflowJob(job *gh.WorkflowJob, runID int) *models.WorkflowJob {
	result := &models.WorkflowJob{
		GitHubID:  job.GetID(),
		RunID:     runID,
		Name:      job.GetName(),
		Status:    job.GetStatus(),
		StartedAt: job.GetStartedAt().Time,
	}

	if job.Conclusion != nil {
		result.Conclusion = job.Conclusion
	}
	if job.RunnerName != nil {
		result.RunnerName = job.RunnerName
	}
	if job.RunnerGroupName != nil {
		result.RunnerGroup = job.RunnerGroupName
	}
	if job.CompletedAt != nil && !job.GetCompletedAt().IsZero() {
		completedAt := job.GetCompletedAt().Time
		result.CompletedAt = &completedAt
		duration := int(completedAt.Sub(result.StartedAt).Seconds())
		result.DurationSeconds = &duration
	}

	// Convert labels
	if len(job.Labels) > 0 {
		labels := make([]interface{}, len(job.Labels))
		for i, label := range job.Labels {
			labels[i] = label
		}
		result.Labels = labels
	}

	// Convert steps
	if len(job.Steps) > 0 {
		steps := make([]interface{}, len(job.Steps))
		for i, step := range job.Steps {
			stepMap := map[string]interface{}{
				"name":   step.GetName(),
				"number": step.GetNumber(),
				"status": step.GetStatus(),
			}
			if step.Conclusion != nil {
				stepMap["conclusion"] = *step.Conclusion
			}
			steps[i] = stepMap
		}
		result.Steps = steps
	}

	return result
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
