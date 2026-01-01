package storage

import (
	"context"
	"time"

	"snorlx/backend/internal/models"
)

// Storage defines the interface for all storage operations
// This allows swapping between in-memory and database storage
type Storage interface {
	// Lifecycle
	Close() error
	Migrate() error

	// Organizations
	ListOrganizations(ctx context.Context) ([]models.Organization, error)
	GetOrganization(ctx context.Context, id int) (*models.Organization, error)
	GetOrganizationByGitHubID(ctx context.Context, githubID int64) (*models.Organization, error)
	UpsertOrganization(ctx context.Context, org *models.Organization) (*models.Organization, error)

	// Repositories
	ListRepositories(ctx context.Context, page, pageSize int, search string) ([]models.Repository, int, error)
	GetRepository(ctx context.Context, id int) (*models.Repository, error)
	GetRepositoryByGitHubID(ctx context.Context, githubID int64) (*models.Repository, error)
	UpsertRepository(ctx context.Context, repo *models.Repository) (*models.Repository, error)
	UpdateRepository(ctx context.Context, id int, repo *models.Repository) (*models.Repository, error)

	// Workflows
	ListWorkflows(ctx context.Context, repoID *int) ([]models.Workflow, error)
	GetWorkflow(ctx context.Context, id int) (*models.Workflow, error)
	GetWorkflowByGitHubID(ctx context.Context, githubID int64) (*models.Workflow, error)
	UpsertWorkflow(ctx context.Context, workflow *models.Workflow) (*models.Workflow, error)

	// Workflow Runs
	ListRuns(ctx context.Context, filters *models.RunFilters, page, pageSize int) ([]models.WorkflowRun, int, error)
	GetRun(ctx context.Context, id int) (*models.WorkflowRun, error)
	GetRunByGitHubID(ctx context.Context, githubID int64) (*models.WorkflowRun, error)
	UpsertRun(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error)

	// Workflow Jobs
	ListJobsForRun(ctx context.Context, runID int) ([]models.WorkflowJob, error)
	GetJob(ctx context.Context, id int) (*models.WorkflowJob, error)
	UpsertJob(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error)

	// Deployments
	ListDeployments(ctx context.Context, repoID *int) ([]models.Deployment, error)
	GetDeployment(ctx context.Context, id int) (*models.Deployment, error)
	UpsertDeployment(ctx context.Context, deployment *models.Deployment) (*models.Deployment, error)

	// Users & Sessions
	GetUserByGitHubID(ctx context.Context, githubID int64) (*models.User, error)
	UpsertUser(ctx context.Context, user *models.User) (*models.User, error)
	CreateSession(ctx context.Context, session *models.Session) error
	GetSession(ctx context.Context, sessionID string) (*models.Session, *models.User, error)
	DeleteSession(ctx context.Context, sessionID string) error
	CleanExpiredSessions(ctx context.Context) error

	// Dashboard & Metrics
	GetDashboardSummary(ctx context.Context) (*models.DashboardSummary, error)
	GetTrends(ctx context.Context, days int) ([]models.Trend, error)
	GetDevOpsMetrics(ctx context.Context, startDate, endDate time.Time) (*models.DevOpsMetrics, error)
}

// StorageMode defines the storage backend type
type StorageMode string

const (
	StorageModeMemory   StorageMode = "memory"
	StorageModeDatabase StorageMode = "database"
)

