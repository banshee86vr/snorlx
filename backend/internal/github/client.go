package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/models"

	"github.com/google/go-github/v60/github"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	ghOAuth "golang.org/x/oauth2/github"
)

const (
	maxRetries    = 3
	retryDelay    = 500 * time.Millisecond
	maxRetryDelay = 5 * time.Second
)

// isRetryableError checks if the error is a transient error worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	// Check for GitHub API error responses
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		switch ghErr.Response.StatusCode {
		case http.StatusBadGateway,      // 502
			http.StatusServiceUnavailable, // 503
			http.StatusGatewayTimeout,     // 504
			http.StatusTooManyRequests:    // 429
			return true
		}
	}
	
	// Check for context errors (not retryable)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	
	// Check error message for common transient issues
	errStr := err.Error()
	return strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout")
}

// retryWithBackoff executes a function with exponential backoff retry
func retryWithBackoff[T any](ctx context.Context, operation string, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	
	delay := retryDelay
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		
		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}
		
		if !isRetryableError(lastErr) {
			return result, lastErr
		}
		
		if attempt < maxRetries-1 {
			log.Warn().
				Err(lastErr).
				Str("operation", operation).
				Int("attempt", attempt+1).
				Dur("retry_in", delay).
				Msg("Retrying after transient error")
			
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
			}
			
			// Exponential backoff
			delay *= 2
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
		}
	}
	
	return result, lastErr
}

// Client wraps the GitHub API client using OAuth App authentication
type Client struct {
	config      *config.Config
	oauthConfig *oauth2.Config
}

// NewClient creates a new GitHub OAuth client
func NewClient(cfg *config.Config) (*Client, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		Scopes:       []string{"read:user", "user:email", "repo", "read:org"},
		Endpoint:     ghOAuth.Endpoint,
	}

	return &Client{
		config:      cfg,
		oauthConfig: oauthConfig,
	}, nil
}

// GetAuthURL returns the OAuth authorization URL
func (c *Client) GetAuthURL(state string) string {
	return c.oauthConfig.AuthCodeURL(state)
}

// ExchangeCode exchanges an authorization code for tokens
func (c *Client) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	return c.oauthConfig.Exchange(ctx, code)
}

// GetUserClient returns a GitHub client authenticated with a user token
func (c *Client) GetUserClient(ctx context.Context, token *oauth2.Token) *github.Client {
	ts := c.oauthConfig.TokenSource(ctx, token)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// ValidateWebhookSignature validates the webhook signature
func (c *Client) ValidateWebhookSignature(payload []byte, signature string) bool {
	if c.config.GitHubWebhookSecret == "" {
		log.Warn().Msg("Webhook secret not configured, skipping signature validation")
		return true
	}

	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(c.config.GitHubWebhookSecret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

// GetUser fetches the authenticated user's profile
func (c *Client) GetUser(ctx context.Context, client *github.Client) (*models.User, error) {
	ghUser, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, err
	}

	user := &models.User{
		GitHubID:  ghUser.GetID(),
		Login:     ghUser.GetLogin(),
		AvatarURL: github.String(ghUser.GetAvatarURL()),
	}

	if ghUser.Name != nil {
		user.Name = ghUser.Name
	}
	if ghUser.Email != nil {
		user.Email = ghUser.Email
	}

	return user, nil
}

// ListOrganizations lists organizations for the authenticated user
func (c *Client) ListOrganizations(ctx context.Context, client *github.Client) ([]*github.Organization, error) {
	var allOrgs []*github.Organization
	opts := &github.ListOptions{PerPage: 100}

	for {
		orgs, resp, err := client.Organizations.List(ctx, "", opts)
		if err != nil {
			return nil, err
		}
		allOrgs = append(allOrgs, orgs...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allOrgs, nil
}

// ListRepositories lists repositories for an organization
func (c *Client) ListRepositories(ctx context.Context, client *github.Client, org string) ([]*github.Repository, error) {
	var allRepos []*github.Repository
	opts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// ListUserRepositories lists repositories for the authenticated user
func (c *Client) ListUserRepositories(ctx context.Context, client *github.Client) ([]*github.Repository, error) {
	var allRepos []*github.Repository
	opts := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Visibility:  "all",
		Affiliation: "owner,collaborator,organization_member",
	}

	for {
		repos, resp, err := client.Repositories.List(ctx, "", opts)
		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// ListWorkflows lists workflows for a repository
func (c *Client) ListWorkflows(ctx context.Context, client *github.Client, owner, repo string) ([]*github.Workflow, error) {
	var allWorkflows []*github.Workflow
	opts := &github.ListOptions{PerPage: 100}

	for {
		type listResult struct {
			workflows *github.Workflows
			resp      *github.Response
		}
		
		result, err := retryWithBackoff(ctx, "ListWorkflows:"+owner+"/"+repo, func() (listResult, error) {
			workflows, resp, err := client.Actions.ListWorkflows(ctx, owner, repo, opts)
			return listResult{workflows, resp}, err
		})
		
		if err != nil {
			return allWorkflows, err // Return what we have so far
		}
		allWorkflows = append(allWorkflows, result.workflows.Workflows...)

		if result.resp.NextPage == 0 {
			break
		}
		opts.Page = result.resp.NextPage
	}

	return allWorkflows, nil
}

// ListWorkflowRuns lists workflow runs with optional filters
// maxRuns limits the total number of runs fetched (0 = no limit, defaults to 500)
func (c *Client) ListWorkflowRuns(ctx context.Context, client *github.Client, owner, repo string, opts *github.ListWorkflowRunsOptions, maxRuns int) ([]*github.WorkflowRun, error) {
	var allRuns []*github.WorkflowRun

	if opts == nil {
		opts = &github.ListWorkflowRunsOptions{}
	}
	
	// Default limit
	if maxRuns <= 0 {
		maxRuns = 500
	}
	
	// Optimize page size based on maxRuns
	perPage := 100
	if maxRuns < 100 {
		perPage = maxRuns
	}
	opts.ListOptions.PerPage = perPage

	for {
		type listResult struct {
			runs *github.WorkflowRuns
			resp *github.Response
		}
		
		result, err := retryWithBackoff(ctx, "ListWorkflowRuns:"+owner+"/"+repo, func() (listResult, error) {
			runs, resp, err := client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
			return listResult{runs, resp}, err
		})
		
		if err != nil {
			return allRuns, err // Return what we have so far on error
		}
		allRuns = append(allRuns, result.runs.WorkflowRuns...)

		if result.resp.NextPage == 0 || len(allRuns) >= maxRuns {
			break
		}
		opts.ListOptions.Page = result.resp.NextPage
	}

	return allRuns, nil
}

// GetWorkflowRun gets a specific workflow run
func (c *Client) GetWorkflowRun(ctx context.Context, client *github.Client, owner, repo string, runID int64) (*github.WorkflowRun, error) {
	run, _, err := client.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	return run, err
}

// ListWorkflowJobs lists jobs for a workflow run
func (c *Client) ListWorkflowJobs(ctx context.Context, client *github.Client, owner, repo string, runID int64) ([]*github.WorkflowJob, error) {
	var allJobs []*github.WorkflowJob
	opts := &github.ListWorkflowJobsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		jobs, resp, err := client.Actions.ListWorkflowJobs(ctx, owner, repo, runID, opts)
		if err != nil {
			return nil, err
		}
		allJobs = append(allJobs, jobs.Jobs...)

		if resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}

	return allJobs, nil
}

// GetWorkflowRunLogs gets the logs URL for a workflow run
func (c *Client) GetWorkflowRunLogs(ctx context.Context, client *github.Client, owner, repo string, runID int64) (string, error) {
	url, _, err := client.Actions.GetWorkflowRunLogs(ctx, owner, repo, runID, 2)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

// RerunWorkflow reruns a workflow
func (c *Client) RerunWorkflow(ctx context.Context, client *github.Client, owner, repo string, runID int64) error {
	_, err := client.Actions.RerunWorkflowByID(ctx, owner, repo, runID)
	return err
}

// CancelWorkflowRun cancels a workflow run
func (c *Client) CancelWorkflowRun(ctx context.Context, client *github.Client, owner, repo string, runID int64) error {
	_, err := client.Actions.CancelWorkflowRunByID(ctx, owner, repo, runID)
	return err
}

// ParseWebhookEvent parses a webhook event
func (c *Client) ParseWebhookEvent(eventType string, payload []byte) (interface{}, error) {
	switch eventType {
	case "workflow_run":
		var event github.WorkflowRunEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		return &event, nil
	case "workflow_job":
		var event github.WorkflowJobEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		return &event, nil
	case "deployment":
		var event github.DeploymentEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		return &event, nil
	case "deployment_status":
		var event github.DeploymentStatusEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		return &event, nil
	default:
		return nil, errors.New("unsupported event type: " + eventType)
	}
}
