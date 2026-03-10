package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/github"
	"snorlx/backend/internal/models"
	"snorlx/backend/internal/scorer"
	"snorlx/backend/internal/storage"
	"snorlx/backend/internal/websocket"

	"github.com/go-chi/chi/v5"
	gh "github.com/google/go-github/v84/github"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// isGitHubNotFoundError checks if the error is a GitHub 404 Not Found error
func isGitHubNotFoundError(err error) bool {
	var ghErr *gh.ErrorResponse
	if errors.As(err, &ghErr) {
		return ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound
	}
	// Also check for 404 in error message as fallback
	return strings.Contains(err.Error(), "404")
}

// Handler contains all HTTP handlers
type Handler struct {
	config   *config.Config
	storage  storage.Storage
	ghClient *github.Client
	wsHub    *websocket.Hub
	scorer   *scorer.Scorer
}

// New creates a new Handler
func New(cfg *config.Config, store storage.Storage, ghClient *github.Client, wsHub *websocket.Hub, sc *scorer.Scorer) *Handler {
	return &Handler{
		config:   cfg,
		storage:  store,
		ghClient: ghClient,
		wsHub:    wsHub,
		scorer:   sc,
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
		Secure:   isSecureRequest(r),
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
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to frontend
	http.Redirect(w, r, h.config.FrontendURL, http.StatusTemporaryRedirect)
}

// Logout logs out the user
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	sessionCookie, err := r.Cookie("session")
	if err == nil {
		_ = h.storage.DeleteSession(r.Context(), sessionCookie.Value)
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Get session from storage
	_, user, err := h.storage.GetSession(r.Context(), sessionCookie.Value)
	if err != nil || user == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
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

const maxWebhookPayloadBytes = 10 * 1024 * 1024 // 10 MB, matches GitHub's max webhook payload

// HandleWebhook processes GitHub webhook events
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read payload with a size limit to prevent memory exhaustion
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookPayloadBytes))
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

		if dep := h.convertAndPersistDeployment(ctx, e.GetRepo(), e.GetDeployment(), nil); dep != nil {
			h.wsHub.BroadcastDeploymentUpdate(e.GetDeployment())
		}

	case *gh.DeploymentStatusEvent:
		log.Info().
			Int64("deployment_id", e.GetDeployment().GetID()).
			Str("status", e.GetDeploymentStatus().GetState()).
			Msg("Processing deployment_status event")

		dep := h.convertAndPersistDeploymentStatus(ctx, e.GetRepo(), e.GetDeployment(), e.GetDeploymentStatus())
		if dep != nil {
			h.wsHub.BroadcastDeploymentUpdate(e.GetDeployment())
		}
	}
}

// ===== WebSocket Handler =====

// WebSocketHandler handles WebSocket connections
func (h *Handler) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate via session cookie before upgrading; WebSocket connections
	// bypass standard HTTP middleware once upgraded, so we must check here.
	sessionCookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	_, user, err := h.storage.GetSession(r.Context(), sessionCookie.Value)
	if err != nil || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP connection to WebSocket (only from the configured frontend origin)
	upgrader := websocket.GetUpgraderWithOrigin(h.config.FrontendURL)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade to WebSocket")
		return
	}

	// Create client
	client := websocket.NewClient(generateState(), user.ID, h.wsHub, conn)

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

	_ = json.NewEncoder(w).Encode(orgs)
}

// GetOrganization gets a single organization
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	org, err := h.storage.GetOrganization(r.Context(), id)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(org)
}

// ===== Repository Handlers =====

// ListRepositories lists all repositories
func (h *Handler) ListRepositories(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 500 {
		pageSize = 500
	}
	search := r.URL.Query().Get("search")

	repos, total, err := h.storage.ListRepositories(r.Context(), page, pageSize, search)
	if err != nil {
		http.Error(w, "Failed to fetch repositories", http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(models.ListResponse[models.Repository]{
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

	_ = json.NewEncoder(w).Encode(repo)
}

// GetRepositoryScore returns the latest score for a repository
func (h *Handler) GetRepositoryScore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	score, err := h.storage.GetLatestRepositoryScore(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to get score", http.StatusInternalServerError)
		return
	}
	if score == nil {
		http.Error(w, "No score found for this repository", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(score)
}

// ListRepositoryScores returns the latest score for every repository
func (h *Handler) ListRepositoryScores(w http.ResponseWriter, r *http.Request) {
	scores, err := h.storage.ListLatestRepositoryScores(r.Context())
	if err != nil {
		http.Error(w, "Failed to list scores", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": scores,
	})
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "started",
		"message": "Sync started, progress will be sent via WebSocket",
	})

	// Run sync in background; do not use r.Context() — it is cancelled when the handler returns (after 202),
	// which would immediately cancel the sync and trigger sync:error. Use Background so sync runs to completion.
	go h.runSync(context.Background(), accessToken) // #nosec G118 -- intentional: sync must outlive the HTTP request
}

// runSync performs the actual sync operation in the background
func (h *Handler) runSync(ctx context.Context, accessToken string) {

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

		// Build maps of GitHub workflow ID to internal ID, path, name, and deployment flag (for deployment classification)
		workflowIDMap := make(map[int64]int)
		workflowPathMap := make(map[int64]string)
		workflowNameMap := make(map[int64]string)
		workflowDeploymentMap := make(map[int64]bool)

		for _, ghWorkflow := range ghWorkflows {
			workflow := &models.Workflow{
				GitHubID:             ghWorkflow.GetID(),
				RepoID:               savedRepo.ID,
				Name:                 ghWorkflow.GetName(),
				Path:                 ghWorkflow.GetPath(),
				State:                ghWorkflow.GetState(),
				IsDeploymentWorkflow: false, // preserve DB value via UpsertWorkflow RETURNING
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
			workflowPathMap[ghWorkflow.GetID()] = ghWorkflow.GetPath()
			workflowNameMap[ghWorkflow.GetID()] = ghWorkflow.GetName()
			workflowDeploymentMap[ghWorkflow.GetID()] = savedWorkflow.IsDeploymentWorkflow
		}

		// Fetch and save workflow runs for this repository (limit to 50 recent runs for faster sync)
		ghRuns, err := h.ghClient.ListWorkflowRuns(ctx, client, owner, repoName, nil, 50)
		if err != nil {
			log.Warn().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to fetch workflow runs")
			continue
		}

		for _, ghRun := range ghRuns {
			// Look up the internal workflow ID
			workflowID, ok := workflowIDMap[ghRun.GetWorkflowID()]
			if !ok {
				continue
			}

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
				StartedAt:  ghRun.GetRunStartedAt().Time,
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

			run.IsDeployment = workflowDeploymentMap[ghRun.GetWorkflowID()] || isDeploymentRun(workflowNameMap[ghRun.GetWorkflowID()], workflowPathMap[ghRun.GetWorkflowID()], ghRun.GetEvent())

			if _, err := h.storage.UpsertRun(ctx, run); err != nil {
				log.Error().Err(err).Int64("run_id", ghRun.GetID()).Msg("Failed to save workflow run")
				continue
			}
			syncedRuns++
		}

		// Score repository (documentation, security, CI/CD, etc.)
		if h.scorer != nil {
			meta := &scorer.RepoMeta{
				HasWorkflows:  len(ghWorkflows) > 0,
				WorkflowNames: make([]string, 0, len(ghWorkflows)),
				Topics:        ghRepo.Topics,
			}
			if ghRepo.PushedAt != nil {
				t := ghRepo.PushedAt.Time
				meta.PushedAt = &t
			}
			meta.Archived = ghRepo.GetArchived()
			for _, w := range ghWorkflows {
				meta.WorkflowNames = append(meta.WorkflowNames, w.GetName())
			}
			score, err := h.scorer.ScoreRepository(ctx, client, owner, repoName, savedRepo, meta)
			if err != nil {
				log.Warn().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to score repository")
			} else {
				if _, err := h.storage.UpsertRepositoryScore(ctx, score); err != nil {
					log.Error().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to save repository score")
				}
			}
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

// runSyncOneRepo syncs workflows and runs for a single repository (light sync).
// repo must exist in storage; owner/name are derived from repo.FullName.
func (h *Handler) runSyncOneRepo(ctx context.Context, client *gh.Client, repo *models.Repository) (syncedWorkflows, syncedRuns int, err error) {
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid repository full_name")
	}
	owner, repoName := parts[0], parts[1]

	ghWorkflows, err := h.ghClient.ListWorkflows(ctx, client, owner, repoName)
	if err != nil {
		return 0, 0, err
	}

	workflowIDMap := make(map[int64]int)
	workflowPathMap := make(map[int64]string)
	workflowNameMap := make(map[int64]string)
	workflowDeploymentMap := make(map[int64]bool)

	for _, ghWorkflow := range ghWorkflows {
		workflow := &models.Workflow{
			GitHubID:             ghWorkflow.GetID(),
			RepoID:               repo.ID,
			Name:                 ghWorkflow.GetName(),
			Path:                 ghWorkflow.GetPath(),
			State:                ghWorkflow.GetState(),
			IsDeploymentWorkflow: false,
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
		workflowPathMap[ghWorkflow.GetID()] = ghWorkflow.GetPath()
		workflowNameMap[ghWorkflow.GetID()] = ghWorkflow.GetName()
		workflowDeploymentMap[ghWorkflow.GetID()] = savedWorkflow.IsDeploymentWorkflow
	}

	ghRuns, err := h.ghClient.ListWorkflowRuns(ctx, client, owner, repoName, nil, 50)
	if err != nil {
		return syncedWorkflows, syncedRuns, err
	}

	for _, ghRun := range ghRuns {
		workflowID, ok := workflowIDMap[ghRun.GetWorkflowID()]
		if !ok {
			continue
		}

		run := &models.WorkflowRun{
			GitHubID:   ghRun.GetID(),
			WorkflowID: workflowID,
			RepoID:     repo.ID,
			RunNumber:  ghRun.GetRunNumber(),
			Name:       ghRun.GetName(),
			Status:     ghRun.GetStatus(),
			Event:      ghRun.GetEvent(),
			Branch:     ghRun.GetHeadBranch(),
			CommitSHA:  ghRun.GetHeadSHA(),
			ActorLogin: ghRun.GetActor().GetLogin(),
			HTMLURL:    ghRun.GetHTMLURL(),
			StartedAt:  ghRun.GetRunStartedAt().Time,
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

		run.IsDeployment = workflowDeploymentMap[ghRun.GetWorkflowID()] || isDeploymentRun(workflowNameMap[ghRun.GetWorkflowID()], workflowPathMap[ghRun.GetWorkflowID()], ghRun.GetEvent())

		if _, err := h.storage.UpsertRun(ctx, run); err != nil {
			log.Error().Err(err).Int64("run_id", ghRun.GetID()).Msg("Failed to save workflow run")
			continue
		}
		syncedRuns++
	}

	return syncedWorkflows, syncedRuns, nil
}

// SyncRepository performs a light sync for a single repository (workflows + runs only).
// Used after re-run so the new run appears without a full sync.
func (h *Handler) SyncRepository(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	repoID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if repoID <= 0 {
		http.Error(w, "Invalid repository ID", http.StatusBadRequest)
		return
	}

	repo, err := h.storage.GetRepository(r.Context(), repoID)
	if err != nil || repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	workflows, runs, err := h.runSyncOneRepo(r.Context(), client, repo)
	if err != nil {
		log.Error().Err(err).Int("repo_id", repoID).Str("repo", repo.FullName).Msg("Light sync failed")
		http.Error(w, "Sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Re-score the repository from GitHub (fetch fresh metadata, then run scorer)
	scoreUpdated := false
	if h.scorer != nil {
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) == 2 {
			owner, repoName := parts[0], parts[1]
			ghRepo, errGh := h.ghClient.GetRepository(r.Context(), client, owner, repoName)
			if errGh == nil && ghRepo != nil {
				workflowsList, _ := h.storage.ListWorkflows(r.Context(), &repoID)
				meta := &scorer.RepoMeta{
					HasWorkflows:  len(workflowsList) > 0,
					WorkflowNames: make([]string, 0, len(workflowsList)),
					Topics:        ghRepo.Topics,
				}
				if ghRepo.PushedAt != nil {
					t := ghRepo.PushedAt.Time
					meta.PushedAt = &t
				}
				meta.Archived = ghRepo.GetArchived()
				for _, w := range workflowsList {
					meta.WorkflowNames = append(meta.WorkflowNames, w.Name)
				}
				score, errScore := h.scorer.ScoreRepository(r.Context(), client, owner, repoName, repo, meta)
				if errScore == nil && score != nil {
					if _, errUpsert := h.storage.UpsertRepositoryScore(r.Context(), score); errUpsert == nil {
						scoreUpdated = true
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"workflows":      workflows,
		"runs":           runs,
		"score_updated":  scoreUpdated,
	})
}

// BackfillDeploymentRuns retroactively sets is_deployment on existing workflow runs that match deployment heuristics.
func (h *Handler) BackfillDeploymentRuns(w http.ResponseWriter, r *http.Request) {
	updated, err := h.storage.BackfillDeploymentRuns(r.Context())
	if err != nil {
		http.Error(w, "Failed to backfill deployment runs", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]int{"updated": updated})
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

	_ = json.NewEncoder(w).Encode(workflows)
}

// GetWorkflow gets a single workflow
func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	wf, err := h.storage.GetWorkflow(r.Context(), id)
	if err != nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(wf)
}

// UpdateWorkflow updates workflow settings (e.g. is_deployment_workflow)
func (h *Handler) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	existing, err := h.storage.GetWorkflow(r.Context(), id)
	if err != nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	var body struct {
		IsDeploymentWorkflow *bool `json:"is_deployment_workflow"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if body.IsDeploymentWorkflow != nil {
		existing.IsDeploymentWorkflow = *body.IsDeploymentWorkflow
	}

	updated, err := h.storage.UpdateWorkflow(r.Context(), id, existing)
	if err != nil {
		http.Error(w, "Failed to update workflow", http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(updated)
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

	_ = json.NewEncoder(w).Encode(models.ListResponse[models.WorkflowRun]{
		Data: runs,
		Pagination: models.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		},
	})
}

// ListActivePipelines returns all runs with status in_progress or queued, sorted by started_at DESC.
// If query param refresh=true is set and the user is authenticated, the handler first pulls the latest
// workflow runs from GitHub for all known repos (so newly triggered pipelines appear), then returns the list.
func (h *Handler) ListActivePipelines(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1"
	if refresh {
		user := h.getUserFromContext(r.Context())
		if user != nil {
			h.pullLatestRunsFromGitHub(r.Context(), user)
		}
	}

	runs, err := h.storage.ListActivePipelines(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch active pipelines")
		http.Error(w, "Failed to fetch active pipelines", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(runs)
}

// GetRun gets a single run. If query param refresh=true is set, fetches the latest
// from GitHub, updates storage, and returns the updated run (avoids stale data after sync).
func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	refresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1"

	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	if refresh {
		user := h.getUserFromContext(r.Context())
		if user != nil {
			if updated, errRefresh := h.refreshRunFromGitHub(r.Context(), run, user); errRefresh == nil {
				run = updated
			}
		}
	}

	_ = json.NewEncoder(w).Encode(run)
}

// GetRunJobs gets jobs for a run - fetches from GitHub on-demand
func (h *Handler) GetRunJobs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	refresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1"

	if !refresh {
		// Return cached jobs if available
		jobs, err := h.storage.ListJobsForRun(r.Context(), id)
		if err == nil && len(jobs) > 0 {
			_ = json.NewEncoder(w).Encode(jobs)
			return
		}
	}

	// Fetch fresh jobs from GitHub
	user := h.getUserFromContext(r.Context())
	if user == nil {
		// Fallback to cached if not authenticated
		jobs, err := h.storage.ListJobsForRun(r.Context(), id)
		if err == nil && len(jobs) > 0 {
			_ = json.NewEncoder(w).Encode(jobs)
			return
		}
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	ghJobs, err := h.ghClient.ListWorkflowJobs(r.Context(), client, owner, repoName, run.GitHubID)
	if err != nil {
		log.Error().Err(err).Int64("run_github_id", run.GitHubID).Msg("Failed to fetch jobs from GitHub")
		// Fallback to cached jobs on GitHub error
		jobs, errCached := h.storage.ListJobsForRun(r.Context(), id)
		if errCached == nil && len(jobs) > 0 {
			_ = json.NewEncoder(w).Encode(jobs)
			return
		}
		_ = json.NewEncoder(w).Encode([]models.WorkflowJob{})
		return
	}

	var savedJobs []models.WorkflowJob
	for _, ghJob := range ghJobs {
		job := h.convertWorkflowJob(ghJob, id)
		if _, err := h.storage.UpsertJob(r.Context(), job); err != nil {
			log.Error().Err(err).Int64("job_id", ghJob.GetID()).Msg("Failed to save job")
			continue
		}
		savedJobs = append(savedJobs, *job)
	}

	_ = json.NewEncoder(w).Encode(savedJobs)
}

// GetRunLogs gets logs URL for a run
func (h *Handler) GetRunLogs(w http.ResponseWriter, r *http.Request) {
	// For now, return a placeholder
	_ = json.NewEncoder(w).Encode(map[string]string{
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

	_ = json.NewEncoder(w).Encode(annotations)
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

	_ = json.NewEncoder(w).Encode(map[string]string{
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
	Name     string                      `yaml:"name,omitempty" json:"name,omitempty"`
	Needs    interface{}                 `yaml:"needs,omitempty" json:"needs,omitempty"` // Can be string or []string
	Uses     string                      `yaml:"uses,omitempty" json:"uses,omitempty"`   // For reusable workflows
	Strategy *WorkflowStrategyDefinition `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

// WorkflowStrategyDefinition represents the strategy section of a job
type WorkflowStrategyDefinition struct {
	// Matrix can be either a map[string]interface{} for static matrices
	// or a string for dynamic expressions like "${{ fromJson(needs.job.outputs.matrix) }}"
	Matrix interface{} `yaml:"matrix,omitempty" json:"matrix,omitempty"`
}

// JobDependency represents a job and its dependencies for the frontend
type JobDependency struct {
	JobID    string   `json:"job_id"`    // The job key in the YAML (e.g., "build", "test")
	Name     string   `json:"name"`      // The display name (from 'name' field or job_id)
	Needs    []string `json:"needs"`     // List of job IDs this job depends on
	IsMatrix bool     `json:"is_matrix"` // Whether this job uses a matrix strategy
	Prefix   string   `json:"prefix"`    // Prefix for job names (e.g., calling job name for reusable workflows)
}

// parseWorkflowNeeds extracts needs from the YAML needs field which can be string or []string
func parseWorkflowNeeds(needs interface{}) []string {
	result := []string{}
	switch n := needs.(type) {
	case string:
		if n != "" {
			result = []string{n}
		}
	case []interface{}:
		for _, item := range n {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	}
	return result
}

// extractJobDependencies parses a workflow definition and returns job dependencies
// prefix is used when parsing reusable workflows to prefix job names
// callingJobNeeds contains the needs of the calling job (for reusable workflows)
func (h *Handler) extractJobDependencies(workflowDef *WorkflowDefinition, prefix string, callingJobNeeds []string) []JobDependency {
	dependencies := make([]JobDependency, 0, len(workflowDef.Jobs))

	for jobID, jobDef := range workflowDef.Jobs {
		dep := JobDependency{
			JobID:    jobID,
			Name:     jobDef.Name,
			Needs:    parseWorkflowNeeds(jobDef.Needs),
			IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
			Prefix:   prefix,
		}

		// Use jobID as name if name is not set
		if dep.Name == "" {
			dep.Name = jobID
		}

		// If this job has no dependencies AND we have calling job needs,
		// it means this job depends on the calling job's dependencies
		if len(dep.Needs) == 0 && len(callingJobNeeds) > 0 {
			dep.Needs = callingJobNeeds
		} else if prefix != "" && len(dep.Needs) > 0 {
			// Prefix the internal needs with the prefix as well
			prefixedNeeds := make([]string, len(dep.Needs))
			for i, need := range dep.Needs {
				prefixedNeeds[i] = need // Keep original, frontend will handle matching
			}
			dep.Needs = prefixedNeeds
		}

		dependencies = append(dependencies, dep)
	}

	return dependencies
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
		_ = json.NewEncoder(w).Encode([]JobDependency{})
		return
	}

	// Parse the YAML
	var workflowDef WorkflowDefinition
	if err := yaml.Unmarshal(content, &workflowDef); err != nil {
		log.Error().Err(err).Msg("Failed to parse workflow YAML")
		_ = json.NewEncoder(w).Encode([]JobDependency{})
		return
	}

	// Extract job dependencies, handling reusable workflows
	allDependencies := []JobDependency{}

	// DEBUG: Log the parsed workflow structure
	log.Debug().Int("job_count", len(workflowDef.Jobs)).Msg("Parsed workflow definition")
	for jobID, jobDef := range workflowDef.Jobs {
		log.Debug().
			Str("job_id", jobID).
			Str("name", jobDef.Name).
			Str("uses", jobDef.Uses).
			Interface("needs", jobDef.Needs).
			Bool("has_strategy", jobDef.Strategy != nil).
			Msg("Job definition details")
	}

	for jobID, jobDef := range workflowDef.Jobs {
		callingJobNeeds := parseWorkflowNeeds(jobDef.Needs)
		callingJobName := jobDef.Name
		if callingJobName == "" {
			callingJobName = jobID
		}

		// Check if this job uses a reusable workflow
		if jobDef.Uses != "" {
			// Try to fetch and parse the reusable workflow
			reusablePath := jobDef.Uses
			log.Debug().Str("uses", reusablePath).Str("calling_job", callingJobName).Msg("Processing reusable workflow reference")

			var reusableOwner, reusableRepo, reusableFilePath, reusableRef string
			var isLocal bool

			if strings.HasPrefix(reusablePath, "./") {
				// Local workflow file
				isLocal = true
				reusableFilePath = strings.TrimPrefix(reusablePath, "./")
				// Remove any @ref suffix
				if atIdx := strings.Index(reusableFilePath, "@"); atIdx > 0 {
					reusableFilePath = reusableFilePath[:atIdx]
				}
				reusableOwner = owner
				reusableRepo = repoName
				reusableRef = run.CommitSHA
			} else {
				// External reusable workflow: org/repo/.github/workflows/file.yml@ref
				// or org/repo/path/to/workflow.yml@ref
				isLocal = false
				
				// Parse the external workflow path
				// Format: {owner}/{repo}/{path}@{ref} or {owner}/{repo}/.github/workflows/{filename}@{ref}
				atIdx := strings.LastIndex(reusablePath, "@")
				if atIdx > 0 {
					reusableRef = reusablePath[atIdx+1:]
					reusablePath = reusablePath[:atIdx]
				} else {
					reusableRef = "main" // Default to main if no ref specified
				}
				
				// Split the path: owner/repo/path/to/file.yml
				pathParts := strings.SplitN(reusablePath, "/", 3)
				if len(pathParts) >= 3 {
					reusableOwner = pathParts[0]
					reusableRepo = pathParts[1]
					reusableFilePath = pathParts[2]
				} else {
					log.Warn().Str("uses", jobDef.Uses).Msg("Could not parse external workflow path")
					allDependencies = append(allDependencies, JobDependency{
						JobID:    jobID,
						Name:     callingJobName,
						Needs:    callingJobNeeds,
						IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
						Prefix:   callingJobName,
					})
					continue
				}
			}

			log.Debug().
				Bool("is_local", isLocal).
				Str("owner", reusableOwner).
				Str("repo", reusableRepo).
				Str("path", reusableFilePath).
				Str("ref", reusableRef).
				Msg("Fetching reusable workflow")

			reusableContent, err := h.ghClient.GetWorkflowContent(r.Context(), client, reusableOwner, reusableRepo, reusableFilePath, reusableRef)
			if err != nil {
				// Use Debug level for 404s (expected when workflow doesn't exist or no access)
				// Use Warn level for unexpected errors (server errors, network issues)
				if isGitHubNotFoundError(err) {
					log.Debug().Str("path", reusableFilePath).Str("owner", reusableOwner).Str("repo", reusableRepo).Msg("Reusable workflow not found, adding as single job")
				} else {
					log.Warn().Err(err).Str("path", reusableFilePath).Str("owner", reusableOwner).Str("repo", reusableRepo).Msg("Failed to fetch reusable workflow, adding as single job")
				}
				// Add the calling job as a single entry with prefix
				allDependencies = append(allDependencies, JobDependency{
					JobID:    jobID,
					Name:     callingJobName,
					Needs:    callingJobNeeds,
					IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
					Prefix:   callingJobName,
				})
				continue
			}

			var reusableWorkflowDef WorkflowDefinition
			if err := yaml.Unmarshal(reusableContent, &reusableWorkflowDef); err != nil {
				log.Warn().Err(err).Str("path", reusableFilePath).Msg("Failed to parse reusable workflow YAML")
				allDependencies = append(allDependencies, JobDependency{
					JobID:    jobID,
					Name:     callingJobName,
					Needs:    callingJobNeeds,
					IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
					Prefix:   callingJobName,
				})
				continue
			}

			// Extract jobs from reusable workflow with prefix
			reusableDeps := h.extractJobDependencies(&reusableWorkflowDef, callingJobName, callingJobNeeds)
			log.Debug().Int("deps_count", len(reusableDeps)).Str("calling_job", callingJobName).Msg("Extracted reusable workflow dependencies")
			allDependencies = append(allDependencies, reusableDeps...)
		} else {
			// Regular job
			allDependencies = append(allDependencies, JobDependency{
				JobID:    jobID,
				Name:     callingJobName,
				Needs:    callingJobNeeds,
				IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
				Prefix:   "",
			})
		}
	}

	_ = json.NewEncoder(w).Encode(allDependencies)
}

// RerunWorkflow reruns a workflow on GitHub
func (h *Handler) RerunWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	if err := h.ghClient.RerunWorkflow(r.Context(), client, owner, repoName, run.GitHubID); err != nil {
		log.Error().Err(err).Int("run_id", id).Int64("github_id", run.GitHubID).Msg("Failed to re-run workflow")
		errMsg := err.Error()
		var ghErr *gh.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil {
			switch ghErr.Response.StatusCode {
			case http.StatusConflict:
				if strings.Contains(errMsg, "re-run") && strings.Contains(errMsg, "not yet") {
					http.Error(w, "This run cannot be re-run yet. Try again in a moment.", http.StatusConflict)
					return
				}
				http.Error(w, "Re-run not allowed: "+errMsg, http.StatusConflict)
				return
			case http.StatusForbidden:
				http.Error(w, "You do not have permission to re-run this workflow.", http.StatusForbidden)
				return
			case http.StatusNotFound:
				http.Error(w, "Workflow run not found on GitHub.", http.StatusNotFound)
				return
			}
		}
		http.Error(w, "Failed to re-run: "+errMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// CancelRun cancels a workflow run on GitHub
func (h *Handler) CancelRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	run, err := h.storage.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	if run.Status != "in_progress" && run.Status != "queued" {
		http.Error(w, "Run cannot be cancelled (not in progress or queued)", http.StatusBadRequest)
		return
	}

	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}
	owner, repoName := parts[0], parts[1]

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	if err := h.ghClient.CancelWorkflowRun(r.Context(), client, owner, repoName, run.GitHubID); err != nil {
		log.Error().Err(err).Int("run_id", id).Int64("github_id", run.GitHubID).Msg("Failed to cancel workflow run")
		errMsg := err.Error()
		// GitHub returns 409 for re-runs that have not yet queued - return user-friendly message
		var ghErr *gh.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusConflict {
			if strings.Contains(errMsg, "re-run") && strings.Contains(errMsg, "not yet queued") {
				http.Error(w, "This run cannot be cancelled yet. Re-runs must be queued before they can be cancelled. Try again in a moment.", http.StatusConflict)
				return
			}
		}
		http.Error(w, "Failed to cancel run: "+errMsg, http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

// ===== Dashboard Handlers =====

// GetDashboardSummary returns dashboard summary
func (h *Handler) GetDashboardSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.storage.GetDashboardSummary(r.Context())
	if err != nil {
		http.Error(w, "Failed to fetch dashboard summary", http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(summary)
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

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"trends": trends,
	})
}

// ===== Helper Functions =====

// isDeploymentRun returns true if the workflow run should be counted as a deployment.
// It uses heuristics: workflow name or path contains release/deploy/cd, or event is deployment/release.
func isDeploymentRun(workflowName, workflowPath, event string) bool {
	lowerEvent := strings.ToLower(event)
	if lowerEvent == "deployment" || lowerEvent == "release" {
		return true
	}
	lowerName := strings.ToLower(workflowName)
	lowerPath := strings.ToLower(workflowPath)
	for _, keyword := range []string{"release", "deploy", "cd"} {
		if strings.Contains(lowerName, keyword) || strings.Contains(lowerPath, keyword) {
			return true
		}
	}
	return false
}

func (h *Handler) getUserFromContext(ctx context.Context) *models.User {
	user, _ := ctx.Value(userContextKey).(*models.User)
	return user
}

// pullLatestRunsFromGitHub fetches the latest workflow runs from GitHub for all known repos
// and upserts them into storage. This allows newly triggered pipelines to appear without a full sync.
// Uses a timeout to avoid blocking the request too long.
func (h *Handler) pullLatestRunsFromGitHub(ctx context.Context, user *models.User) {
	const pullTimeout = 25 * time.Second
	ctx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(ctx, token)

	repos, _, err := h.storage.ListRepositories(ctx, 1, 50, "")
	if err != nil || len(repos) == 0 {
		return
	}

	for _, repo := range repos {
		if ctx.Err() != nil {
			break
		}
		parts := strings.Split(repo.FullName, "/")
		if len(parts) != 2 {
			continue
		}
		owner, repoName := parts[0], parts[1]

		workflows, err := h.storage.ListWorkflows(ctx, &repo.ID)
		if err != nil || len(workflows) == 0 {
			continue
		}

		workflowIDMap := make(map[int64]int)
		workflowPathMap := make(map[int64]string)
		workflowNameMap := make(map[int64]string)
		workflowDeploymentMap := make(map[int64]bool)
		for _, wf := range workflows {
			workflowIDMap[wf.GitHubID] = wf.ID
			workflowPathMap[wf.GitHubID] = wf.Path
			workflowNameMap[wf.GitHubID] = wf.Name
			workflowDeploymentMap[wf.GitHubID] = wf.IsDeploymentWorkflow
		}

		ghRuns, err := h.ghClient.ListWorkflowRuns(ctx, client, owner, repoName, nil, 30)
		if err != nil {
			log.Debug().Err(err).Str("repo", repo.FullName).Msg("Failed to fetch workflow runs for refresh")
			continue
		}

		for _, ghRun := range ghRuns {
			workflowID, ok := workflowIDMap[ghRun.GetWorkflowID()]
			if !ok {
				continue
			}
			run := &models.WorkflowRun{
				GitHubID:   ghRun.GetID(),
				WorkflowID: workflowID,
				RepoID:     repo.ID,
				RunNumber:  ghRun.GetRunNumber(),
				Name:       ghRun.GetName(),
				Status:     ghRun.GetStatus(),
				Event:      ghRun.GetEvent(),
				Branch:     ghRun.GetHeadBranch(),
				CommitSHA:  ghRun.GetHeadSHA(),
				ActorLogin: ghRun.GetActor().GetLogin(),
				HTMLURL:    ghRun.GetHTMLURL(),
				StartedAt:  ghRun.GetRunStartedAt().Time,
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
			run.IsDeployment = workflowDeploymentMap[ghRun.GetWorkflowID()] || isDeploymentRun(workflowNameMap[ghRun.GetWorkflowID()], workflowPathMap[ghRun.GetWorkflowID()], ghRun.GetEvent())

			if _, err := h.storage.UpsertRun(ctx, run); err != nil {
				log.Debug().Err(err).Int64("run_id", ghRun.GetID()).Msg("Failed to upsert run during refresh")
			}
		}
	}
}

// refreshRunFromGitHub fetches the latest run from GitHub, upserts it, and returns the updated run.
// On any error (repo lookup, GitHub API, upsert) returns the original run and the error.
func (h *Handler) refreshRunFromGitHub(ctx context.Context, run *models.WorkflowRun, user *models.User) (*models.WorkflowRun, error) {
	repo, err := h.storage.GetRepository(ctx, run.RepoID)
	if err != nil {
		return run, err
	}
	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		return run, errors.New("invalid repo full name")
	}
	owner, repoName := parts[0], parts[1]
	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(ctx, token)
	ghRun, err := h.ghClient.GetWorkflowRun(ctx, client, owner, repoName, run.GitHubID)
	if err != nil {
		return run, err
	}
	updated := h.convertWorkflowRun(ghRun, nil)
	updated.RepoID = run.RepoID
	updated.WorkflowID = run.WorkflowID
	saved, err := h.storage.UpsertRun(ctx, updated)
	if err != nil {
		return run, err
	}
	return saved, nil
}

// refreshRunsFromGitHub refreshes each run from GitHub with a capped timeout. Runs that fail to refresh are left unchanged.
func (h *Handler) refreshRunsFromGitHub(ctx context.Context, runs []models.WorkflowRun, user *models.User) []models.WorkflowRun {
	const refreshTimeout = 15 * time.Second
	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	result := make([]models.WorkflowRun, len(runs))
	copy(result, runs)
	for i := range result {
		if ctx.Err() != nil {
			break
		}
		updated, err := h.refreshRunFromGitHub(ctx, &result[i], user)
		if err == nil {
			result[i] = *updated
		}
	}
	return result
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

	// Commit timestamp for lead time (commit-to-deploy)
	if run.HeadCommit != nil && !run.HeadCommit.GetTimestamp().IsZero() {
		t := run.HeadCommit.GetTimestamp().Time
		result.CommitTimestamp = &t
	}

	result.IsDeployment = isDeploymentRun(run.GetName(), "", run.GetEvent())

	return result
}

// convertAndPersistDeployment creates a deployment record from a GitHub deployment event and persists it.
// Returns the persisted deployment or nil on error (e.g. repo not found).
func (h *Handler) convertAndPersistDeployment(ctx context.Context, repo *gh.Repository, ghDep *gh.Deployment, _ *gh.DeploymentStatus) *models.Deployment {
	if repo == nil || ghDep == nil {
		return nil
	}
	ourRepo, err := h.storage.GetRepositoryByGitHubID(ctx, repo.GetID())
	if err != nil {
		log.Warn().Err(err).Int64("repo_github_id", repo.GetID()).Msg("Repo not found for deployment event")
		return nil
	}
	creatorLogin := ""
	if ghDep.GetCreator() != nil {
		creatorLogin = ghDep.GetCreator().GetLogin()
	}
	dep := &models.Deployment{
		GitHubID:     ghDep.GetID(),
		RepoID:       ourRepo.ID,
		RunID:        nil,
		Environment:  ghDep.GetEnvironment(),
		Status:       "created",
		Description:  ghDep.Description,
		CreatorLogin: creatorLogin,
		SHA:          ghDep.GetSHA(),
		Ref:          ghDep.GetRef(),
		CreatedAt:    ghDep.GetCreatedAt().Time,
		UpdatedAt:    ghDep.GetUpdatedAt().Time,
	}
	saved, err := h.storage.UpsertDeployment(ctx, dep)
	if err != nil {
		log.Error().Err(err).Int64("deployment_id", ghDep.GetID()).Msg("Failed to persist deployment")
		return nil
	}
	return saved
}

// convertAndPersistDeploymentStatus updates a deployment record from a deployment_status webhook and persists it.
func (h *Handler) convertAndPersistDeploymentStatus(ctx context.Context, repo *gh.Repository, ghDep *gh.Deployment, ghStatus *gh.DeploymentStatus) *models.Deployment {
	if repo == nil || ghDep == nil || ghStatus == nil {
		return nil
	}
	ourRepo, err := h.storage.GetRepositoryByGitHubID(ctx, repo.GetID())
	if err != nil {
		log.Warn().Err(err).Int64("repo_github_id", repo.GetID()).Msg("Repo not found for deployment_status event")
		return nil
	}
	creatorLogin := ""
	if ghDep.GetCreator() != nil {
		creatorLogin = ghDep.GetCreator().GetLogin()
	}
	status := ghStatus.GetState()
	dep := &models.Deployment{
		GitHubID:     ghDep.GetID(),
		RepoID:       ourRepo.ID,
		RunID:        nil,
		Environment:  ghDep.GetEnvironment(),
		Status:       status,
		Description:  ghDep.Description,
		CreatorLogin: creatorLogin,
		SHA:          ghDep.GetSHA(),
		Ref:          ghDep.GetRef(),
		CreatedAt:    ghDep.GetCreatedAt().Time,
		UpdatedAt:    ghDep.GetUpdatedAt().Time,
	}
	if status == "success" && !ghStatus.GetUpdatedAt().IsZero() {
		t := ghStatus.GetUpdatedAt().Time
		dep.DeployedAt = &t
	}
	saved, err := h.storage.UpsertDeployment(ctx, dep)
	if err != nil {
		log.Error().Err(err).Int64("deployment_id", ghDep.GetID()).Msg("Failed to persist deployment status")
		return nil
	}
	return saved
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

// isSecureRequest returns true when the connection is HTTPS, either directly
// (r.TLS is non-nil) or via a TLS-terminating reverse proxy (X-Forwarded-Proto).
func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
