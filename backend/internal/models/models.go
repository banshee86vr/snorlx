package models

import (
	"time"
)

// Organization represents a GitHub organization
type Organization struct {
	ID        int       `json:"id"`
	GitHubID  int64     `json:"github_id"`
	Login     string    `json:"login"`
	Name      *string   `json:"name,omitempty"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	Settings  JSONMap   `json:"settings"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Repository represents a GitHub repository
type Repository struct {
	ID            int       `json:"id"`
	GitHubID      int64     `json:"github_id"`
	OrgID         *int      `json:"org_id,omitempty"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   *string   `json:"description,omitempty"`
	DefaultBranch string    `json:"default_branch"`
	HTMLURL       string    `json:"html_url"`
	IsPrivate     bool      `json:"is_private"`
	IsActive      bool      `json:"is_active"`
	Settings      JSONMap   `json:"settings"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Computed fields
	Organization *Organization `json:"organization,omitempty"`
	WorkflowCount int          `json:"workflow_count,omitempty"`
}

// Workflow represents a GitHub Actions workflow
type Workflow struct {
	ID        int       `json:"id"`
	GitHubID  int64     `json:"github_id"`
	RepoID    int       `json:"repo_id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	State     string    `json:"state"`
	BadgeURL  *string   `json:"badge_url,omitempty"`
	HTMLURL   *string   `json:"html_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed fields
	Repository     *Repository `json:"repository,omitempty"`
	LastRun        *WorkflowRun `json:"last_run,omitempty"`
	TotalRuns      int         `json:"total_runs,omitempty"`
	SuccessRate    float64     `json:"success_rate,omitempty"`
	AvgDuration    int         `json:"avg_duration_seconds,omitempty"`
}

// WorkflowRun represents a single execution of a workflow
type WorkflowRun struct {
	ID              int        `json:"id"`
	GitHubID        int64      `json:"github_id"`
	WorkflowID      int        `json:"workflow_id"`
	RepoID          int        `json:"repo_id"`
	RunNumber       int        `json:"run_number"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Conclusion      *string    `json:"conclusion,omitempty"`
	Event           string     `json:"event"`
	Branch          string     `json:"branch"`
	CommitSHA       string     `json:"commit_sha"`
	CommitMessage   *string    `json:"commit_message,omitempty"`
	ActorLogin      string     `json:"actor_login"`
	ActorAvatar     *string    `json:"actor_avatar,omitempty"`
	HTMLURL         string     `json:"html_url"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationSeconds *int       `json:"duration_seconds,omitempty"`
	IsDeployment    bool       `json:"is_deployment"`
	Environment     *string    `json:"environment,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`

	// Computed fields
	Workflow   *Workflow     `json:"workflow,omitempty"`
	Repository *Repository   `json:"repository,omitempty"`
	Jobs       []WorkflowJob `json:"jobs,omitempty"`
}

// WorkflowJob represents a job within a workflow run
type WorkflowJob struct {
	ID              int        `json:"id"`
	GitHubID        int64      `json:"github_id"`
	RunID           int        `json:"run_id"`
	RunStartedAt    time.Time  `json:"run_started_at"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Conclusion      *string    `json:"conclusion,omitempty"`
	RunnerName      *string    `json:"runner_name,omitempty"`
	RunnerGroup     *string    `json:"runner_group,omitempty"`
	Labels          JSONArray  `json:"labels"`
	Steps           JSONArray  `json:"steps"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationSeconds *int       `json:"duration_seconds,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// JobStep represents a step within a job
type JobStep struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion,omitempty"`
	Number      int        `json:"number"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Deployment represents a deployment event
type Deployment struct {
	ID           int        `json:"id"`
	GitHubID     int64      `json:"github_id"`
	RepoID       int        `json:"repo_id"`
	RunID        *int       `json:"run_id,omitempty"`
	Environment  string     `json:"environment"`
	Status       string     `json:"status"`
	Description  *string    `json:"description,omitempty"`
	CreatorLogin string     `json:"creator_login"`
	SHA          string     `json:"sha"`
	Ref          string     `json:"ref"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// User represents an authenticated user
type User struct {
	ID             int        `json:"id"`
	GitHubID       int64      `json:"github_id"`
	Login          string     `json:"login"`
	Name           *string    `json:"name,omitempty"`
	Email          *string    `json:"email,omitempty"`
	AvatarURL      *string    `json:"avatar_url,omitempty"`
	AccessToken    string     `json:"-"`
	RefreshToken   *string    `json:"-"`
	TokenExpiresAt *time.Time `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Session represents a user session
type Session struct {
	ID        string    `json:"id"`
	UserID    int       `json:"user_id"`
	Data      JSONMap   `json:"data"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// DevOpsMetrics represents DevOps performance metrics
type DevOpsMetrics struct {
	DeploymentFrequency DeploymentFrequency `json:"deployment_frequency"`
	LeadTime            LeadTime            `json:"lead_time"`
	ChangeFailureRate   ChangeFailureRate   `json:"change_failure_rate"`
	MTTR                MTTR                `json:"mttr"`
	Period              string              `json:"period"`
	StartDate           time.Time           `json:"start_date"`
	EndDate             time.Time           `json:"end_date"`
}

// DeploymentFrequency represents the deployment frequency metric
type DeploymentFrequency struct {
	TotalDeployments   int     `json:"total_deployments"`
	DeploymentsPerDay  float64 `json:"deployments_per_day"`
	DeploymentsPerWeek float64 `json:"deployments_per_week"`
	Rating             string  `json:"rating"` // elite, high, medium, low
}

// LeadTime represents the lead time for changes metric
type LeadTime struct {
	MedianMinutes int    `json:"median_minutes"`
	P95Minutes    int    `json:"p95_minutes"`
	Rating        string `json:"rating"`
}

// ChangeFailureRate represents the change failure rate metric
type ChangeFailureRate struct {
	TotalDeployments  int     `json:"total_deployments"`
	FailedDeployments int     `json:"failed_deployments"`
	Rate              float64 `json:"rate"`
	Rating            string  `json:"rating"`
}

// MTTR represents mean time to recovery metric
type MTTR struct {
	MedianMinutes int    `json:"median_minutes"`
	P95Minutes    int    `json:"p95_minutes"`
	Rating        string `json:"rating"`
}

// DashboardSummary represents the dashboard overview
type DashboardSummary struct {
	Repositories     RepositorySummary `json:"repositories"`
	Workflows        WorkflowSummary   `json:"workflows"`
	Runs             RunSummary        `json:"runs"`
	PreviousRuns     RunSummary        `json:"previous_runs"`
	RecentRuns       []WorkflowRun     `json:"recent_runs"`
	FailedRuns       []WorkflowRun     `json:"failed_runs"`
	TopRepositories  []RepositoryStats `json:"top_repositories"`
}

// RepositorySummary represents repository statistics
type RepositorySummary struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
}

// WorkflowSummary represents workflow statistics
type WorkflowSummary struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Disabled int `json:"disabled"`
}

// RunSummary represents run statistics
type RunSummary struct {
	Total         int     `json:"total"`
	Success       int     `json:"success"`
	Failed        int     `json:"failed"`
	InProgress    int     `json:"in_progress"`
	Queued        int     `json:"queued"`
	Cancelled     int     `json:"cancelled"`
	SuccessRate   float64 `json:"success_rate"`
	TotalDuration int     `json:"total_duration_seconds"`
}

// RepositoryStats represents statistics for a repository
type RepositoryStats struct {
	Repository  Repository `json:"repository"`
	TotalRuns   int        `json:"total_runs"`
	SuccessRate float64    `json:"success_rate"`
	AvgDuration int        `json:"avg_duration_seconds"`
}

// Trend represents trend data over time
type Trend struct {
	Date            time.Time `json:"date"`
	TotalRuns       int       `json:"total_runs"`
	SuccessfulRuns  int       `json:"successful_runs"`
	FailedRuns      int       `json:"failed_runs"`
	AvgDuration     int       `json:"avg_duration_seconds"`
	DeploymentCount int       `json:"deployment_count"`
}

// JSONMap is a type alias for JSON object columns
type JSONMap map[string]interface{}

// JSONArray is a type alias for JSON array columns
type JSONArray []interface{}

// Pagination represents pagination parameters
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// ListResponse represents a paginated list response
type ListResponse[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// RunFilters represents filters for workflow runs
type RunFilters struct {
	Status     string    `json:"status,omitempty"`
	Conclusion string    `json:"conclusion,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	Event      string    `json:"event,omitempty"`
	Actor      string    `json:"actor,omitempty"`
	WorkflowID int       `json:"workflow_id,omitempty"`
	RepoID     int       `json:"repo_id,omitempty"`
	StartDate  time.Time `json:"start_date,omitempty"`
	EndDate    time.Time `json:"end_date,omitempty"`
}
