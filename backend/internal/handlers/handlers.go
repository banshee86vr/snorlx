package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/github"
	"snorlx/backend/internal/models"
	"snorlx/backend/internal/scorer"
	"snorlx/backend/internal/storage"
	"snorlx/backend/internal/version"
	"snorlx/backend/internal/websocket"

	"github.com/go-chi/chi/v5"
	gh "github.com/google/go-github/v92/github"
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
	// watchers records recent API activity per user so the live poller only works for people who
	// are looking at the dashboard.
	watchers *watchers
	// etags remembers the GitHub ETag of every polled resource for conditional requests.
	etags *etagCache
	// wakeActive and wakeDiscovery make the poll loops run early when a page becomes visible.
	wakeActive    chan struct{}
	wakeDiscovery chan struct{}
}

// New creates a new Handler
func New(cfg *config.Config, store storage.Storage, ghClient *github.Client, wsHub *websocket.Hub, sc *scorer.Scorer) *Handler {
	h := &Handler{
		config:        cfg,
		storage:       store,
		ghClient:      ghClient,
		wsHub:         wsHub,
		scorer:        sc,
		watchers:      newWatchers(),
		etags:         newETagCache(),
		wakeActive:    make(chan struct{}, 1),
		wakeDiscovery: make(chan struct{}, 1),
	}
	if wsHub != nil {
		wsHub.SetActivationListener(h.UserActivated)
	}
	return h
}

// Context key for user
type contextKey string

const userContextKey contextKey = "user"
const scopesContextKey contextKey = "scopes"
const authMethodContextKey contextKey = "auth_method"

const authMethodBearer = "bearer"
const authMethodSession = "session"

var defaultScopes = []string{"read", "write"}

// IsBearerAuth reports whether the request was authenticated with a validated API token.
func IsBearerAuth(ctx context.Context) bool {
	method, _ := ctx.Value(authMethodContextKey).(string)
	return method == authMethodBearer
}

const (
	oauthStateCookieName = "oauth_state"
	// sessionCookieSecureName uses the __Host- prefix: browsers only accept it over HTTPS, with
	// Path=/ and without Domain, so a subdomain or plain-http attacker cannot plant it.
	sessionCookieSecureName   = "__Host-session"
	sessionCookieInsecureName = "session"
)

// sessionCookieName returns the cookie name that matches the configured Secure flag.
func (h *Handler) sessionCookieName() string {
	if h.config.CookieSecure {
		return sessionCookieSecureName
	}
	return sessionCookieInsecureName
}

// readSessionCookie returns the session cookie value, accepting only the name that matches the
// current security mode.
func (h *Handler) readSessionCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(h.sessionCookieName())
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (h *Handler) authCookie(name, value string, maxAge int, expires time.Time) *http.Cookie {
	// #nosec G124 -- Secure follows the FRONTEND_URL scheme (config.CookieSecure): it is true for every
	// https deployment and false only for plain-http loopback development, which config.Load enforces.
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if maxAge != 0 {
		cookie.MaxAge = maxAge
	}
	if !expires.IsZero() {
		cookie.Expires = expires
	}
	return cookie
}

// requireRepoAccess writes a 404 and returns false when user may not see repoID. The response is
// identical to a missing resource so callers cannot enumerate other users' repositories.
func (h *Handler) requireRepoAccess(w http.ResponseWriter, r *http.Request, user *models.User, repoID int) bool {
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	allowed, err := h.storage.HasRepositoryAccess(r.Context(), user.ID, repoID)
	if err != nil {
		log.Error().Err(err).Int("user_id", user.ID).Int("repo_id", repoID).Msg("Failed to check repository access")
		http.Error(w, "Not found", http.StatusNotFound)
		return false
	}
	if !allowed {
		http.Error(w, "Not found", http.StatusNotFound)
		return false
	}
	return true
}

// loadAccessibleRun fetches a run and verifies the user may see its repository.
func (h *Handler) loadAccessibleRun(w http.ResponseWriter, r *http.Request, user *models.User, runID int) (*models.WorkflowRun, bool) {
	run, err := h.storage.GetRun(r.Context(), runID)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return nil, false
	}
	if !h.requireRepoAccess(w, r, user, run.RepoID) {
		return nil, false
	}
	return run, true
}

// loadAccessibleRepo fetches a repository and verifies the user may see it.
func (h *Handler) loadAccessibleRepo(w http.ResponseWriter, r *http.Request, user *models.User, repoID int) (*models.Repository, bool) {
	if !h.requireRepoAccess(w, r, user, repoID) {
		return nil, false
	}
	repo, err := h.storage.GetRepository(r.Context(), repoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return nil, false
	}
	return repo, true
}

// splitFullName splits "owner/name" into its two parts.
func splitFullName(fullName string) (owner, name string, ok bool) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// repoViewers returns the IDs of the users who may receive events about repoID.
func (h *Handler) repoViewers(ctx context.Context, repoID int) []int {
	users, err := h.storage.ListUsersWithRepositoryAccess(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Int("repo_id", repoID).Msg("Failed to list repository viewers")
		return nil
	}
	return users
}

// Health reports process liveness and the release version.
// Probes use the HTTP status code. The body is JSON for clients.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}{
		Status:  "ok",
		Version: version.Version,
	})
}

// Ready reports whether the storage backend answers. Kubernetes readiness probes use it so a pod
// whose database is unreachable stops receiving traffic instead of serving an empty dashboard.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := h.storage.Ping(ctx); err != nil {
		log.Warn().Err(err).Msg("Readiness check failed")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// ===== Auth Handlers =====

// Login initiates GitHub OAuth
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	state := generateState()

	// Store state in cookie
	http.SetCookie(w, h.authCookie(oauthStateCookieName, state, 300, time.Time{}))

	url := h.ghClient.GetAuthURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Callback handles GitHub OAuth callback
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(r.URL.Query().Get("state"))) != 1 {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	// Clear state cookie
	http.SetCookie(w, h.authCookie(oauthStateCookieName, "", -1, time.Time{}))

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

	// Enforce the login allowlist before anything is persisted
	allowed, err := h.isLoginAllowed(r.Context(), client, ghUser.Login)
	if err != nil {
		log.Error().Err(err).Str("login", ghUser.Login).Msg("Failed to evaluate login allowlist")
		http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		return
	}
	if !allowed {
		log.Warn().Str("login", ghUser.Login).Msg("Login rejected: account not in allowlist")
		http.Error(w, "This GitHub account is not allowed to sign in", http.StatusForbidden)
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
	http.SetCookie(w, h.authCookie(h.sessionCookieName(), sessionID, 0, expiresAt))

	// Redirect to frontend
	http.Redirect(w, r, h.config.FrontendURL, http.StatusTemporaryRedirect)
}

// isLoginAllowed applies ALLOWED_GITHUB_USERS and ALLOWED_GITHUB_ORGS. With no allowlist every
// GitHub account may sign in. Organization membership is read with the user's own token.
func (h *Handler) isLoginAllowed(ctx context.Context, client *gh.Client, login string) (bool, error) {
	if !h.config.LoginRestricted() {
		return true, nil
	}
	for _, allowed := range h.config.AllowedGitHubUsers {
		if strings.EqualFold(allowed, login) {
			return true, nil
		}
	}
	if len(h.config.AllowedGitHubOrgs) == 0 {
		return false, nil
	}
	orgs, err := h.ghClient.ListOrganizations(ctx, client)
	if err != nil {
		return false, err
	}
	for _, org := range orgs {
		for _, allowed := range h.config.AllowedGitHubOrgs {
			if strings.EqualFold(allowed, org.GetLogin()) {
				return true, nil
			}
		}
	}
	return false, nil
}

// Logout logs out the user
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if sessionID, ok := h.readSessionCookie(r); ok {
		_ = h.storage.DeleteSession(r.Context(), sessionID)
	}

	// Clear session cookie
	http.SetCookie(w, h.authCookie(h.sessionCookieName(), "", -1, time.Time{}))

	w.WriteHeader(http.StatusOK)
}

// AuthStatus returns the current authentication status
func (h *Handler) AuthStatus(w http.ResponseWriter, r *http.Request) {
	// Check session cookie directly (this endpoint is not behind AuthMiddleware)
	sessionID, ok := h.readSessionCookie(r)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Get session from storage
	_, user, err := h.storage.GetSession(r.Context(), sessionID)
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

// AuthMiddleware checks if the user is authenticated via Bearer API token or session cookie.
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, scopes, ok := h.authenticateBearer(r); ok {
			h.noteAPIActivity(user.ID, true)
			ctx := context.WithValue(r.Context(), userContextKey, user)
			ctx = context.WithValue(ctx, scopesContextKey, scopes)
			ctx = context.WithValue(ctx, authMethodContextKey, authMethodBearer)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Reject clearly invalid API tokens instead of falling through to session auth.
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			plaintext := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			if strings.HasPrefix(plaintext, "snorlx_") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		sessionID, ok := h.readSessionCookie(r)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		_, user, err := h.storage.GetSession(r.Context(), sessionID)
		if err != nil || user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		h.noteAPIActivity(user.ID, false)
		ctx := context.WithValue(r.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, scopesContextKey, append([]string{}, defaultScopes...))
		ctx = context.WithValue(ctx, authMethodContextKey, authMethodSession)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireWriteScope rejects mutating requests when the API token lacks write scope.
func (h *Handler) RequireWriteScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			scopes := h.getScopesFromContext(r.Context())
			if !hasScope(scopes, "write") {
				http.Error(w, "Forbidden: write scope required", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) authenticateBearer(r *http.Request) (*models.User, []string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, nil, false
	}
	plaintext := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if plaintext == "" || !strings.HasPrefix(plaintext, "snorlx_") {
		return nil, nil, false
	}
	hash := hashApiToken(plaintext)
	token, user, err := h.storage.GetApiTokenByHash(r.Context(), hash)
	if err != nil || user == nil || token == nil {
		return nil, nil, false
	}
	_ = h.storage.TouchApiTokenLastUsed(r.Context(), token.ID)
	scopes := token.Scopes
	if len(scopes) == 0 {
		scopes = append([]string{}, defaultScopes...)
	}
	return user, scopes, true
}

func hashApiToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func (h *Handler) getScopesFromContext(ctx context.Context) []string {
	scopes, _ := ctx.Value(scopesContextKey).([]string)
	return scopes
}

// ===== API Token Handlers =====

type createApiTokenRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// ListApiTokens lists the current user's non-revoked API tokens.
func (h *Handler) ListApiTokens(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	tokens, err := h.storage.ListApiTokens(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Failed to list tokens", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": tokens})
}

// CreateApiToken mints a personal API token; plaintext is returned only once.
func (h *Handler) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req createApiTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	scopes, err := normalizeScopes(req.Scopes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	plaintext, err := generateApiTokenPlaintext("snorlx_")
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	token := &models.ApiToken{
		UserID:      user.ID,
		Name:        name,
		TokenPrefix: plaintext[:min(12, len(plaintext))],
		TokenHash:   hashApiToken(plaintext),
		Scopes:      scopes,
	}
	created, err := h.storage.CreateApiToken(r.Context(), token)
	if err != nil {
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":           created.ID,
		"name":         created.Name,
		"token_prefix": created.TokenPrefix,
		"scopes":       created.Scopes,
		"created_at":   created.CreatedAt,
		"token":        plaintext,
	})
}

// RevokeApiToken revokes one of the current user's API tokens.
func (h *Handler) RevokeApiToken(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid token id", http.StatusBadRequest)
		return
	}
	if err := h.storage.RevokeApiToken(r.Context(), user.ID, id); err != nil {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return append([]string{}, defaultScopes...), nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "read" && s != "write" {
			return nil, errors.New("scopes must be read and/or write")
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if !seen["read"] {
		out = append([]string{"read"}, out...)
	}
	return out, nil
}

func generateApiTokenPlaintext(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf), nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch e := event.(type) {
	case *gh.WorkflowRunEvent:
		log.Info().
			Str("action", e.GetAction()).
			Int64("run_id", e.GetWorkflowRun().GetID()).
			Msg("Processing workflow_run event")

		// Only runs of repositories somebody already synced are stored; the webhook payload alone
		// does not tell us which users may see the repository.
		repo, err := h.storage.GetRepositoryByGitHubID(ctx, e.GetRepo().GetID())
		if err != nil {
			log.Debug().Int64("repo_github_id", e.GetRepo().GetID()).Msg("Ignoring workflow_run for unknown repository")
			return
		}
		workflow, err := h.storage.GetWorkflowByGitHubID(ctx, e.GetWorkflowRun().GetWorkflowID())
		if err != nil {
			log.Debug().Int64("workflow_github_id", e.GetWorkflowRun().GetWorkflowID()).Msg("Ignoring workflow_run for unknown workflow")
			return
		}

		run := h.convertWorkflowRun(e.GetWorkflowRun())
		run.RepoID = repo.ID
		run.WorkflowID = workflow.ID
		run.IsDeployment = workflow.IsDeploymentWorkflow || isDeploymentRun(workflow.Name, workflow.Path, run.Event)
		saved, err := h.storage.UpsertRun(ctx, run)
		if err != nil {
			log.Error().Err(err).Msg("Failed to save workflow run")
			return
		}

		h.wsHub.SendWorkflowRunUpdate(h.repoViewers(ctx, repo.ID), saved)

	case *gh.WorkflowJobEvent:
		log.Info().
			Str("action", e.GetAction()).
			Int64("job_id", e.GetWorkflowJob().GetID()).
			Msg("Processing workflow_job event")

		repo, err := h.storage.GetRepositoryByGitHubID(ctx, e.GetRepo().GetID())
		if err != nil {
			log.Debug().Int64("repo_github_id", e.GetRepo().GetID()).Msg("Ignoring workflow_job for unknown repository")
			return
		}
		run, err := h.storage.GetRunByGitHubID(ctx, e.GetWorkflowJob().GetRunID())
		if err != nil || run == nil {
			log.Debug().Int64("run_github_id", e.GetWorkflowJob().GetRunID()).Msg("Ignoring workflow_job for unknown run")
			return
		}
		// The run must belong to the repository named in the delivery; a job must never be attached
		// to a run of another repository.
		if run.RepoID != repo.ID {
			log.Warn().Int64("run_github_id", run.GitHubID).Int("run_repo_id", run.RepoID).Int("delivery_repo_id", repo.ID).Msg("Ignoring workflow_job whose run belongs to another repository")
			return
		}
		if _, err := h.storage.UpsertJob(ctx, h.convertWorkflowJob(e.GetWorkflowJob(), run.ID)); err != nil {
			log.Error().Err(err).Int64("job_id", e.GetWorkflowJob().GetID()).Msg("Failed to save workflow job")
			return
		}
		h.wsHub.SendWorkflowJobUpdate(h.repoViewers(ctx, repo.ID), runJobsEvent{RunID: run.ID, RunGitHubID: run.GitHubID})

	case *gh.DeploymentEvent:
		log.Info().
			Int64("deployment_id", e.GetDeployment().GetID()).
			Msg("Processing deployment event")

		if dep := h.convertAndPersistDeployment(ctx, e.GetRepo(), e.GetDeployment(), nil); dep != nil {
			h.wsHub.SendDeploymentUpdate(h.repoViewers(ctx, dep.RepoID), e.GetDeployment())
		}

	case *gh.DeploymentStatusEvent:
		log.Info().
			Int64("deployment_id", e.GetDeployment().GetID()).
			Str("status", e.GetDeploymentStatus().GetState()).
			Msg("Processing deployment_status event")

		dep := h.convertAndPersistDeploymentStatus(ctx, e.GetRepo(), e.GetDeployment(), e.GetDeploymentStatus())
		if dep != nil {
			h.wsHub.SendDeploymentUpdate(h.repoViewers(ctx, dep.RepoID), e.GetDeployment())
		}
	}
}

// ===== WebSocket Handler =====

// WebSocketHandler handles WebSocket connections
func (h *Handler) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate via session cookie before upgrading; WebSocket connections
	// bypass standard HTTP middleware once upgraded, so we must check here.
	sessionID, ok := h.readSessionCookie(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	_, user, err := h.storage.GetSession(r.Context(), sessionID)
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

// ListOrganizations lists the organizations of the repositories the user may see
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	orgs, err := h.storage.ListOrganizations(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Failed to fetch organizations", http.StatusInternalServerError)
		return
	}
	if orgs == nil {
		orgs = []models.Organization{}
	}

	_ = json.NewEncoder(w).Encode(orgs)
}

// GetOrganization gets a single organization
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	org, err := h.storage.GetOrganization(r.Context(), user.ID, id)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(org)
}

// ===== Repository Handlers =====

// ListRepositories lists the repositories the user may see
func (h *Handler) ListRepositories(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
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

	repos, total, err := h.storage.ListRepositories(r.Context(), user.ID, page, pageSize, search)
	if err != nil {
		http.Error(w, "Failed to fetch repositories", http.StatusInternalServerError)
		return
	}
	if repos == nil {
		repos = []models.Repository{}
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

	repo, ok := h.loadAccessibleRepo(w, r, h.getUserFromContext(r.Context()), id)
	if !ok {
		return
	}

	_ = json.NewEncoder(w).Encode(repo)
}

// GetRepositoryScore returns the latest score for a repository
func (h *Handler) GetRepositoryScore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if !h.requireRepoAccess(w, r, h.getUserFromContext(r.Context()), id) {
		return
	}

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

// ListRepositoryScores returns the latest score for every repository the user may see
func (h *Handler) ListRepositoryScores(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	scores, err := h.storage.ListLatestRepositoryScores(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Failed to list scores", http.StatusInternalServerError)
		return
	}
	if scores == nil {
		scores = []models.RepositoryScore{}
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

	// Copy what the background goroutine needs; the request context ends when the handler returns.
	userID := user.ID
	accessToken := user.AccessToken

	// Return immediately - sync runs in background
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "started",
		"message": "Sync started, progress will be sent via WebSocket",
	})

	// Run sync in background; do not use r.Context(): it is cancelled when the handler returns (after 202),
	// which would immediately cancel the sync and trigger sync:error. Use Background so sync runs to completion.
	go h.runSync(context.Background(), userID, accessToken) // #nosec G118 -- intentional: sync must outlive the HTTP request
}

// runSync performs the actual sync operation in the background. Every repository it stores is
// granted to userID, which is what makes it visible to that user afterwards.
func (h *Handler) runSync(ctx context.Context, userID int, accessToken string) {

	// Create GitHub client with user's token
	token := &oauth2.Token{AccessToken: accessToken}
	client := h.ghClient.GetUserClient(ctx, token)

	// Fetch user's repositories from GitHub
	ghRepos, err := h.ghClient.ListUserRepositories(ctx, client)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch repositories from GitHub")
		h.wsHub.SendSyncError(userID, "Failed to fetch repositories from GitHub")
		return
	}

	// Apply filters from config
	ghRepos = h.filterRepositories(ghRepos)

	total := len(ghRepos)
	syncedRepos := 0
	syncedWorkflows := 0
	syncedRuns := 0

	h.wsHub.SendSyncStart(userID, total)

	for i, ghRepo := range ghRepos {
		h.wsHub.SendSyncProgress(userID, i, total, ghRepo.GetFullName())

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
		if err := h.storage.GrantRepositoryAccess(ctx, userID, savedRepo.ID); err != nil {
			log.Error().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to grant repository access")
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

		savedWorkflows := make([]models.Workflow, 0, len(ghWorkflows))
		for _, ghWorkflow := range ghWorkflows {
			savedWorkflow, err := h.storage.UpsertWorkflow(ctx, workflowFromGitHub(ghWorkflow, savedRepo.ID))
			if err != nil {
				log.Error().Err(err).Str("workflow", ghWorkflow.GetName()).Msg("Failed to save workflow")
				continue
			}
			syncedWorkflows++
			savedWorkflows = append(savedWorkflows, *savedWorkflow)
		}

		// Fetch and save workflow runs for this repository (limit to 50 recent runs for faster sync)
		ghRuns, err := h.ghClient.ListWorkflowRuns(ctx, client, owner, repoName, nil, 50)
		if err != nil {
			log.Warn().Err(err).Str("repo", ghRepo.GetFullName()).Msg("Failed to fetch workflow runs")
			continue
		}

		workflows := indexWorkflows(savedWorkflows)
		for _, ghRun := range ghRuns {
			run, ok := h.runFromGitHub(ghRun, savedRepo.ID, workflows)
			if !ok {
				continue
			}
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
		Int("user_id", userID).
		Int("repositories", syncedRepos).
		Int("workflows", syncedWorkflows).
		Int("runs", syncedRuns).
		Msg("Sync completed")

	h.wsHub.SendSyncComplete(userID, syncedRepos, syncedWorkflows, syncedRuns)
}

// runSyncOneRepo syncs workflows and runs for a single repository (light sync).
// repo must exist in storage; owner/name are derived from repo.FullName.
func (h *Handler) runSyncOneRepo(ctx context.Context, client *gh.Client, repo *models.Repository) (syncedWorkflows, syncedRuns int, err error) {
	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		return 0, 0, errors.New("invalid repository full_name")
	}

	ghWorkflows, err := h.ghClient.ListWorkflows(ctx, client, owner, repoName)
	if err != nil {
		return 0, 0, err
	}

	savedWorkflows := make([]models.Workflow, 0, len(ghWorkflows))
	for _, ghWorkflow := range ghWorkflows {
		savedWorkflow, err := h.storage.UpsertWorkflow(ctx, workflowFromGitHub(ghWorkflow, repo.ID))
		if err != nil {
			log.Error().Err(err).Str("workflow", ghWorkflow.GetName()).Msg("Failed to save workflow")
			continue
		}
		syncedWorkflows++
		savedWorkflows = append(savedWorkflows, *savedWorkflow)
	}

	ghRuns, err := h.ghClient.ListWorkflowRuns(ctx, client, owner, repoName, nil, 50)
	if err != nil {
		return syncedWorkflows, syncedRuns, err
	}

	workflows := indexWorkflows(savedWorkflows)
	for _, ghRun := range ghRuns {
		run, ok := h.runFromGitHub(ghRun, repo.ID, workflows)
		if !ok {
			continue
		}
		if _, err := h.storage.UpsertRun(ctx, run); err != nil {
			log.Error().Err(err).Int64("run_id", ghRun.GetID()).Msg("Failed to save workflow run")
			continue
		}
		syncedRuns++
	}

	return syncedWorkflows, syncedRuns, nil
}

// workflowFromGitHub maps a GitHub workflow to the storage model. The deployment flag is left
// false: UpsertWorkflow keeps the stored value, which the user may have set by hand.
func workflowFromGitHub(ghWorkflow *gh.Workflow, repoID int) *models.Workflow {
	workflow := &models.Workflow{
		GitHubID: ghWorkflow.GetID(),
		RepoID:   repoID,
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
	return workflow
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

	repo, ok := h.loadAccessibleRepo(w, r, user, repoID)
	if !ok {
		return
	}

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	workflows, runs, err := h.runSyncOneRepo(r.Context(), client, repo)
	if err != nil {
		log.Error().Err(err).Int("repo_id", repoID).Str("repo", repo.FullName).Msg("Light sync failed")
		http.Error(w, "Sync failed: GitHub did not return the repository data", http.StatusBadGateway)
		return
	}

	// Re-score the repository from GitHub (fetch fresh metadata, then run scorer)
	scoreUpdated := false
	if h.scorer != nil {
		if owner, repoName, ok := splitFullName(repo.FullName); ok {
			ghRepo, errGh := h.ghClient.GetRepository(r.Context(), client, owner, repoName)
			if errGh == nil && ghRepo != nil {
				workflowsList, _ := h.storage.ListWorkflows(r.Context(), user.ID, &repoID)
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
		"status":        "ok",
		"workflows":     workflows,
		"runs":          runs,
		"score_updated": scoreUpdated,
	})
}

// BackfillDeploymentRuns retroactively sets is_deployment on the user's workflow runs that match deployment heuristics.
func (h *Handler) BackfillDeploymentRuns(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	updated, err := h.storage.BackfillDeploymentRuns(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Failed to backfill deployment runs", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]int{"updated": updated})
}

// ===== Workflow Handlers =====

// ListWorkflows lists the workflows of the repositories the user may see
func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var repoID *int
	if repoIDStr := r.URL.Query().Get("repo_id"); repoIDStr != "" {
		id, _ := strconv.Atoi(repoIDStr)
		repoID = &id
	}

	workflows, err := h.storage.ListWorkflows(r.Context(), user.ID, repoID)
	if err != nil {
		http.Error(w, "Failed to fetch workflows", http.StatusInternalServerError)
		return
	}
	if workflows == nil {
		workflows = []models.Workflow{}
	}

	_ = json.NewEncoder(w).Encode(workflows)
}

// loadAccessibleWorkflow fetches a workflow and verifies the user may see its repository.
func (h *Handler) loadAccessibleWorkflow(w http.ResponseWriter, r *http.Request, user *models.User, id int) (*models.Workflow, bool) {
	wf, err := h.storage.GetWorkflow(r.Context(), id)
	if err != nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return nil, false
	}
	if !h.requireRepoAccess(w, r, user, wf.RepoID) {
		return nil, false
	}
	return wf, true
}

// GetWorkflow gets a single workflow
func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	wf, ok := h.loadAccessibleWorkflow(w, r, h.getUserFromContext(r.Context()), id)
	if !ok {
		return
	}

	_ = json.NewEncoder(w).Encode(wf)
}

// UpdateWorkflow updates workflow settings (e.g. is_deployment_workflow)
func (h *Handler) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	existing, ok := h.loadAccessibleWorkflow(w, r, h.getUserFromContext(r.Context()), id)
	if !ok {
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
	user := h.getUserFromContext(r.Context())
	if _, ok := h.loadAccessibleWorkflow(w, r, user, id); !ok {
		return
	}
	h.listRunsWithFilter(w, r, user, &models.RunFilters{WorkflowID: id})
}

// ===== Run Handlers =====

// ListRuns lists the workflow runs of the repositories the user may see
func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	h.listRunsWithFilter(w, r, user, nil)
}

func (h *Handler) listRunsWithFilter(w http.ResponseWriter, r *http.Request, user *models.User, baseFilters *models.RunFilters) {
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

	runs, total, err := h.storage.ListRuns(r.Context(), user.ID, filters, page, pageSize)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch runs")
		http.Error(w, "Failed to fetch runs", http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []models.WorkflowRun{}
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
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1"
	if refresh {
		h.refreshUserRuns(r.Context(), user.ID)
	}

	runs, err := h.storage.ListActivePipelines(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch active pipelines")
		http.Error(w, "Failed to fetch active pipelines", http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []models.WorkflowRun{}
	}
	_ = json.NewEncoder(w).Encode(runs)
}

// GetRun gets a single run. If query param refresh=true is set, fetches the latest
// from GitHub, updates storage, and returns the updated run (avoids stale data after sync).
func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	refresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1"

	user := h.getUserFromContext(r.Context())
	run, ok := h.loadAccessibleRun(w, r, user, id)
	if !ok {
		return
	}

	if refresh {
		if updated, errRefresh := h.refreshRunFromGitHub(r.Context(), run, user); errRefresh == nil {
			run = updated
		}
	}

	_ = json.NewEncoder(w).Encode(run)
}

// GetRunJobs gets jobs for a run - fetches from GitHub on-demand
func (h *Handler) GetRunJobs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	refresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1"

	user := h.getUserFromContext(r.Context())
	run, ok := h.loadAccessibleRun(w, r, user, id)
	if !ok {
		return
	}

	if !refresh {
		// Return cached jobs if available
		jobs, err := h.storage.ListJobsForRun(r.Context(), id)
		if err == nil && len(jobs) > 0 {
			_ = json.NewEncoder(w).Encode(jobs)
			return
		}
	}

	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}

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

	savedJobs := make([]models.WorkflowJob, 0, len(ghJobs))
	for _, ghJob := range ghJobs {
		saved, err := h.storage.UpsertJob(r.Context(), h.convertWorkflowJob(ghJob, id))
		if err != nil {
			log.Error().Err(err).Int64("job_id", ghJob.GetID()).Msg("Failed to save job")
			continue
		}
		savedJobs = append(savedJobs, *saved)
	}

	_ = json.NewEncoder(w).Encode(savedJobs)
}

// GetRunLogs gets logs URL for a run
func (h *Handler) GetRunLogs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if _, ok := h.loadAccessibleRun(w, r, h.getUserFromContext(r.Context()), id); !ok {
		return
	}
	// Run-level log archives need a GitHub App installation token; job logs are available per job.
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Run-level logs are not available; use the per-job logs endpoint",
	})
}

// GetRunAnnotations fetches annotations for a run from GitHub
func (h *Handler) GetRunAnnotations(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	run, ok := h.loadAccessibleRun(w, r, user, id)
	if !ok {
		return
	}

	// Get the repository to find owner/repo name
	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}

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

	// Get the job from storage. Same message as the access check so job IDs cannot be enumerated.
	job, err := h.storage.GetJob(r.Context(), id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Get the run to find the repo and verify access
	run, ok := h.loadAccessibleRun(w, r, user, job.RunID)
	if !ok {
		return
	}

	// Get the repository
	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}

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
	Name string                           `yaml:"name" json:"name"`
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

// Reusable workflow references are content controlled by whoever writes to the monitored
// repository. They drive GitHub API calls made with the viewer's token, so only the documented
// shapes are accepted: "./.github/workflows/file.yml" or "owner/repo/.github/workflows/file.yml@ref".
var (
	reusableWorkflowPathPattern = regexp.MustCompile(`^\.github/workflows/[A-Za-z0-9_.-]+\.ya?ml$`)
	gitHubNamePattern           = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	gitRefPattern               = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
)

// reusableWorkflowRef is a validated reference to a reusable workflow file.
type reusableWorkflowRef struct {
	owner, repo, path, ref string
	isLocal                bool
}

// parseReusableWorkflowRef validates a `uses:` value. localOwner, localRepo and localRef describe the
// calling repository and are used for "./" references.
func parseReusableWorkflowRef(uses, localOwner, localRepo, localRef string) (reusableWorkflowRef, bool) {
	if strings.HasPrefix(uses, "./") {
		path := strings.TrimPrefix(uses, "./")
		if at := strings.Index(path, "@"); at >= 0 {
			path = path[:at]
		}
		if !reusableWorkflowPathPattern.MatchString(path) {
			return reusableWorkflowRef{}, false
		}
		return reusableWorkflowRef{owner: localOwner, repo: localRepo, path: path, ref: localRef, isLocal: true}, true
	}

	ref := "main"
	spec := uses
	if at := strings.LastIndex(spec, "@"); at >= 0 {
		ref = spec[at+1:]
		spec = spec[:at]
	}
	parts := strings.SplitN(spec, "/", 3)
	if len(parts) != 3 {
		return reusableWorkflowRef{}, false
	}
	owner, repo, path := parts[0], parts[1], parts[2]
	if !gitHubNamePattern.MatchString(owner) || !gitHubNamePattern.MatchString(repo) {
		return reusableWorkflowRef{}, false
	}
	if !reusableWorkflowPathPattern.MatchString(path) || !gitRefPattern.MatchString(ref) || strings.Contains(ref, "..") {
		return reusableWorkflowRef{}, false
	}
	return reusableWorkflowRef{owner: owner, repo: repo, path: path, ref: ref}, true
}

// GetRunWorkflowDefinition fetches and parses the workflow YAML to extract job dependencies
func (h *Handler) GetRunWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	run, ok := h.loadAccessibleRun(w, r, user, id)
	if !ok {
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

	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}

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

	for jobID, jobDef := range workflowDef.Jobs {
		callingJobNeeds := parseWorkflowNeeds(jobDef.Needs)
		callingJobName := jobDef.Name
		if callingJobName == "" {
			callingJobName = jobID
		}

		// Check if this job uses a reusable workflow
		if jobDef.Uses != "" {
			// The calling job is reported as a single node whenever the reusable workflow cannot be expanded.
			singleJob := JobDependency{
				JobID:    jobID,
				Name:     callingJobName,
				Needs:    callingJobNeeds,
				IsMatrix: jobDef.Strategy != nil && jobDef.Strategy.Matrix != nil,
				Prefix:   callingJobName,
			}

			reusable, ok := parseReusableWorkflowRef(jobDef.Uses, owner, repoName, run.CommitSHA)
			if !ok {
				log.Warn().Str("uses", jobDef.Uses).Msg("Unsupported reusable workflow reference, adding as single job")
				allDependencies = append(allDependencies, singleJob)
				continue
			}

			log.Debug().
				Bool("is_local", reusable.isLocal).
				Str("owner", reusable.owner).
				Str("repo", reusable.repo).
				Str("path", reusable.path).
				Str("ref", reusable.ref).
				Msg("Fetching reusable workflow")

			reusableContent, err := h.ghClient.GetWorkflowContent(r.Context(), client, reusable.owner, reusable.repo, reusable.path, reusable.ref)
			if err != nil {
				// 404 is expected when the workflow does not exist or the viewer has no access
				if isGitHubNotFoundError(err) {
					log.Debug().Str("path", reusable.path).Str("owner", reusable.owner).Str("repo", reusable.repo).Msg("Reusable workflow not found, adding as single job")
				} else {
					log.Warn().Err(err).Str("path", reusable.path).Str("owner", reusable.owner).Str("repo", reusable.repo).Msg("Failed to fetch reusable workflow, adding as single job")
				}
				allDependencies = append(allDependencies, singleJob)
				continue
			}

			var reusableWorkflowDef WorkflowDefinition
			if err := yaml.Unmarshal(reusableContent, &reusableWorkflowDef); err != nil {
				log.Warn().Err(err).Str("path", reusable.path).Msg("Failed to parse reusable workflow YAML")
				allDependencies = append(allDependencies, singleJob)
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

// gitHubStatus returns the HTTP status GitHub answered with, or 0 for non-GitHub errors.
func gitHubStatus(err error) int {
	var ghErr *gh.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil {
		return ghErr.Response.StatusCode
	}
	return 0
}

// RerunWorkflow reruns a workflow on GitHub
func (h *Handler) RerunWorkflow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	run, ok := h.loadAccessibleRun(w, r, user, id)
	if !ok {
		return
	}

	repo, err := h.storage.GetRepository(r.Context(), run.RepoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	if err := h.ghClient.RerunWorkflow(r.Context(), client, owner, repoName, run.GitHubID); err != nil {
		log.Error().Err(err).Int("run_id", id).Int64("github_id", run.GitHubID).Msg("Failed to re-run workflow")
		// GitHub error strings include the full API URL, so only fixed messages reach the client.
		switch gitHubStatus(err) {
		case http.StatusConflict:
			http.Error(w, "This run cannot be re-run right now. Wait for it to finish or try again in a moment.", http.StatusConflict)
		case http.StatusForbidden:
			http.Error(w, "You do not have permission to re-run this workflow.", http.StatusForbidden)
		case http.StatusNotFound:
			http.Error(w, "Workflow run not found on GitHub.", http.StatusNotFound)
		default:
			http.Error(w, "GitHub did not accept the re-run request.", http.StatusBadGateway)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// CancelRun cancels a workflow run on GitHub
func (h *Handler) CancelRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	user := h.getUserFromContext(r.Context())
	run, ok := h.loadAccessibleRun(w, r, user, id)
	if !ok {
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

	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		http.Error(w, "Invalid repository name", http.StatusInternalServerError)
		return
	}

	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(r.Context(), token)

	if err := h.ghClient.CancelWorkflowRun(r.Context(), client, owner, repoName, run.GitHubID); err != nil {
		log.Error().Err(err).Int("run_id", id).Int64("github_id", run.GitHubID).Msg("Failed to cancel workflow run")
		switch gitHubStatus(err) {
		case http.StatusConflict:
			// GitHub returns 409 for re-runs that have not yet queued
			http.Error(w, "This run cannot be cancelled yet. Re-runs must be queued before they can be cancelled. Try again in a moment.", http.StatusConflict)
		case http.StatusForbidden:
			http.Error(w, "You do not have permission to cancel this run.", http.StatusForbidden)
		case http.StatusNotFound:
			http.Error(w, "Workflow run not found on GitHub.", http.StatusNotFound)
		default:
			http.Error(w, "GitHub did not accept the cancel request.", http.StatusBadGateway)
		}
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

// ===== Dashboard Handlers =====

// GetDashboardSummary returns the dashboard summary for the repositories the user may see
func (h *Handler) GetDashboardSummary(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	summary, err := h.storage.GetDashboardSummary(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Failed to fetch dashboard summary", http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(summary)
}

// maxTrendDays bounds the trends window so a single request cannot scan the whole hypertable.
const maxTrendDays = 365

// GetTrends returns trend data
func (h *Handler) GetTrends(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	if days > maxTrendDays {
		days = maxTrendDays
	}

	trends, err := h.storage.GetTrends(r.Context(), user.ID, days)
	if err != nil {
		http.Error(w, "Failed to fetch trends", http.StatusInternalServerError)
		return
	}
	if trends == nil {
		trends = []models.Trend{}
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

// refreshRunFromGitHub fetches the latest run from GitHub, upserts it, and returns the updated run.
// On any error (repo lookup, GitHub API, upsert) returns the original run and the error.
func (h *Handler) refreshRunFromGitHub(ctx context.Context, run *models.WorkflowRun, user *models.User) (*models.WorkflowRun, error) {
	repo, err := h.storage.GetRepository(ctx, run.RepoID)
	if err != nil {
		return run, err
	}
	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		return run, errors.New("invalid repo full name")
	}
	token := &oauth2.Token{AccessToken: user.AccessToken}
	client := h.ghClient.GetUserClient(ctx, token)
	ghRun, err := h.ghClient.GetWorkflowRun(ctx, client, owner, repoName, run.GitHubID)
	if err != nil {
		return run, err
	}
	saved, err := h.storage.UpsertRun(ctx, h.applyGitHubRun(run, ghRun))
	if err != nil {
		return run, err
	}
	return saved, nil
}

// applyGitHubRun converts a fresh GitHub payload for a run that is already stored. The internal
// identifiers and the deployment classification (which knows the workflow path) come from the
// stored run.
func (h *Handler) applyGitHubRun(stored *models.WorkflowRun, ghRun *gh.WorkflowRun) *models.WorkflowRun {
	updated := h.convertWorkflowRun(ghRun)
	updated.RepoID = stored.RepoID
	updated.WorkflowID = stored.WorkflowID
	updated.IsDeployment = stored.IsDeployment
	return updated
}

// workflowIndex maps GitHub workflow IDs to the stored workflows of one repository.
type workflowIndex map[int64]models.Workflow

func indexWorkflows(workflows []models.Workflow) workflowIndex {
	idx := make(workflowIndex, len(workflows))
	for _, wf := range workflows {
		idx[wf.GitHubID] = wf
	}
	return idx
}

// runFromGitHub converts a run listed for a repository. Runs of workflows that are not stored
// (deleted or never synced) are skipped.
func (h *Handler) runFromGitHub(ghRun *gh.WorkflowRun, repoID int, workflows workflowIndex) (*models.WorkflowRun, bool) {
	wf, ok := workflows[ghRun.GetWorkflowID()]
	if !ok {
		return nil, false
	}
	run := h.convertWorkflowRun(ghRun)
	run.RepoID = repoID
	run.WorkflowID = wf.ID
	run.IsDeployment = wf.IsDeploymentWorkflow || isDeploymentRun(wf.Name, wf.Path, run.Event)
	return run, true
}

// convertWorkflowRun maps a GitHub run to the storage model. RepoID and WorkflowID are left for the
// caller, which knows the internal identifiers. Completion fields are set only for completed runs:
// GitHub updates updated_at on every status change, so it is not a completion time before that.
func (h *Handler) convertWorkflowRun(run *gh.WorkflowRun) *models.WorkflowRun {
	result := &models.WorkflowRun{
		GitHubID:   run.GetID(),
		RunNumber:  run.GetRunNumber(),
		Name:       run.GetName(),
		Status:     run.GetStatus(),
		Event:      run.GetEvent(),
		Branch:     run.GetHeadBranch(),
		CommitSHA:  run.GetHeadSHA(),
		ActorLogin: run.GetActor().GetLogin(),
		HTMLURL:    run.GetHTMLURL(),
		StartedAt:  run.GetRunStartedAt().Time,
	}

	if run.Conclusion != nil {
		result.Conclusion = run.Conclusion
	}
	if run.GetActor() != nil {
		avatar := run.GetActor().GetAvatarURL()
		result.ActorAvatar = &avatar
	}
	if !run.GetUpdatedAt().IsZero() && run.GetStatus() == "completed" {
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

// generateState returns 128 random bits as hex (OAuth state, WebSocket client IDs).
func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// generateSessionID returns 256 random bits as hex.
func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
