package storage

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"snorlx/backend/internal/models"

	"github.com/rs/zerolog/log"
)

// MemoryStorage implements Storage interface using in-memory maps
type MemoryStorage struct {
	mu sync.RWMutex

	// Auto-increment IDs
	orgIDCounter    int32
	repoIDCounter   int32
	workflowIDCounter int32
	runIDCounter    int32
	jobIDCounter    int32
	deployIDCounter int32
	userIDCounter   int32

	// Data stores
	organizations map[int]*models.Organization
	repositories  map[int]*models.Repository
	workflows     map[int]*models.Workflow
	runs          map[int]*models.WorkflowRun
	jobs          map[int]*models.WorkflowJob
	deployments   map[int]*models.Deployment
	users         map[int]*models.User
	sessions      map[string]*models.Session

	// GitHub ID indexes for fast lookups
	orgGitHubIndex      map[int64]int
	repoGitHubIndex     map[int64]int
	workflowGitHubIndex map[int64]int
	runGitHubIndex      map[int64]int
	userGitHubIndex     map[int64]int
}

// NewMemoryStorage creates a new in-memory storage instance
func NewMemoryStorage() *MemoryStorage {
	log.Info().Msg("Using in-memory storage (no persistence)")
	return &MemoryStorage{
		organizations:       make(map[int]*models.Organization),
		repositories:        make(map[int]*models.Repository),
		workflows:           make(map[int]*models.Workflow),
		runs:                make(map[int]*models.WorkflowRun),
		jobs:                make(map[int]*models.WorkflowJob),
		deployments:         make(map[int]*models.Deployment),
		users:               make(map[int]*models.User),
		sessions:            make(map[string]*models.Session),
		orgGitHubIndex:      make(map[int64]int),
		repoGitHubIndex:     make(map[int64]int),
		workflowGitHubIndex: make(map[int64]int),
		runGitHubIndex:      make(map[int64]int),
		userGitHubIndex:     make(map[int64]int),
	}
}

// Close implements Storage interface
func (m *MemoryStorage) Close() error {
	return nil
}

// Migrate implements Storage interface (no-op for memory storage)
func (m *MemoryStorage) Migrate() error {
	log.Info().Msg("Memory storage initialized (no migrations needed)")
	return nil
}

// ===== Organizations =====

func (m *MemoryStorage) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	orgs := make([]models.Organization, 0, len(m.organizations))
	for _, org := range m.organizations {
		orgs = append(orgs, *org)
	}

	// Sort by login
	sort.Slice(orgs, func(i, j int) bool {
		return orgs[i].Login < orgs[j].Login
	})

	return orgs, nil
}

func (m *MemoryStorage) GetOrganization(ctx context.Context, id int) (*models.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	org, ok := m.organizations[id]
	if !ok {
		return nil, errors.New("organization not found")
	}
	return org, nil
}

func (m *MemoryStorage) GetOrganizationByGitHubID(ctx context.Context, githubID int64) (*models.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.orgGitHubIndex[githubID]
	if !ok {
		return nil, errors.New("organization not found")
	}
	return m.organizations[id], nil
}

func (m *MemoryStorage) UpsertOrganization(ctx context.Context, org *models.Organization) (*models.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.orgGitHubIndex[org.GitHubID]; ok {
		// Update existing
		existing := m.organizations[existingID]
		existing.Login = org.Login
		existing.Name = org.Name
		existing.AvatarURL = org.AvatarURL
		existing.Settings = org.Settings
		existing.UpdatedAt = time.Now()
		return existing, nil
	}

	// Create new
	org.ID = int(atomic.AddInt32(&m.orgIDCounter, 1))
	org.CreatedAt = time.Now()
	org.UpdatedAt = time.Now()

	m.organizations[org.ID] = org
	m.orgGitHubIndex[org.GitHubID] = org.ID

	return org, nil
}

// ===== Repositories =====

func (m *MemoryStorage) ListRepositories(ctx context.Context, page, pageSize int, search string) ([]models.Repository, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Normalize search query
	searchLower := strings.ToLower(strings.TrimSpace(search))

	// Filter active repos and apply search
	var repos []models.Repository
	for _, repo := range m.repositories {
		if repo.IsActive {
			// Apply search filter if provided
			if searchLower != "" {
				nameLower := strings.ToLower(repo.Name)
				fullNameLower := strings.ToLower(repo.FullName)
				if !strings.Contains(nameLower, searchLower) && !strings.Contains(fullNameLower, searchLower) {
					continue
				}
			}

			// Count workflows for this repo
			workflowCount := 0
			for _, wf := range m.workflows {
				if wf.RepoID == repo.ID {
					workflowCount++
				}
			}
			repoCopy := *repo
			repoCopy.WorkflowCount = workflowCount
			repos = append(repos, repoCopy)
		}
	}

	// Sort by full name
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].FullName < repos[j].FullName
	})

	total := len(repos)

	// Apply pagination
	offset := (page - 1) * pageSize
	if offset >= len(repos) {
		return []models.Repository{}, total, nil
	}
	end := offset + pageSize
	if end > len(repos) {
		end = len(repos)
	}

	return repos[offset:end], total, nil
}

func (m *MemoryStorage) GetRepository(ctx context.Context, id int) (*models.Repository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	repo, ok := m.repositories[id]
	if !ok {
		return nil, errors.New("repository not found")
	}
	return repo, nil
}

func (m *MemoryStorage) GetRepositoryByGitHubID(ctx context.Context, githubID int64) (*models.Repository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.repoGitHubIndex[githubID]
	if !ok {
		return nil, errors.New("repository not found")
	}
	return m.repositories[id], nil
}

func (m *MemoryStorage) UpsertRepository(ctx context.Context, repo *models.Repository) (*models.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.repoGitHubIndex[repo.GitHubID]; ok {
		// Update existing
		existing := m.repositories[existingID]
		existing.Name = repo.Name
		existing.FullName = repo.FullName
		existing.Description = repo.Description
		existing.DefaultBranch = repo.DefaultBranch
		existing.HTMLURL = repo.HTMLURL
		existing.IsPrivate = repo.IsPrivate
		existing.IsActive = repo.IsActive
		existing.Settings = repo.Settings
		existing.UpdatedAt = time.Now()
		return existing, nil
	}

	// Create new
	repo.ID = int(atomic.AddInt32(&m.repoIDCounter, 1))
	repo.CreatedAt = time.Now()
	repo.UpdatedAt = time.Now()

	m.repositories[repo.ID] = repo
	m.repoGitHubIndex[repo.GitHubID] = repo.ID

	return repo, nil
}

func (m *MemoryStorage) UpdateRepository(ctx context.Context, id int, repo *models.Repository) (*models.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.repositories[id]
	if !ok {
		return nil, errors.New("repository not found")
	}

	existing.Name = repo.Name
	existing.FullName = repo.FullName
	existing.Description = repo.Description
	existing.DefaultBranch = repo.DefaultBranch
	existing.HTMLURL = repo.HTMLURL
	existing.IsPrivate = repo.IsPrivate
	existing.IsActive = repo.IsActive
	existing.Settings = repo.Settings
	existing.UpdatedAt = time.Now()

	return existing, nil
}

// ===== Workflows =====

func (m *MemoryStorage) ListWorkflows(ctx context.Context, repoID *int) ([]models.Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var workflows []models.Workflow
	for _, wf := range m.workflows {
		if repoID != nil && wf.RepoID != *repoID {
			continue
		}

		wfCopy := *wf
		// Add repository info
		if repo, ok := m.repositories[wf.RepoID]; ok {
			wfCopy.Repository = &models.Repository{FullName: repo.FullName}
		}

		// Find the last run and compute stats for this workflow
		var lastRun *models.WorkflowRun
		var lastRunTime time.Time
		var totalRuns, successfulRuns, completedRuns int
		var totalDuration int
		for _, run := range m.runs {
			if run.WorkflowID == wf.ID {
				totalRuns++
				if lastRun == nil || run.StartedAt.After(lastRunTime) {
					runCopy := *run
					lastRun = &runCopy
					lastRunTime = run.StartedAt
				}
				if run.Conclusion != nil {
					completedRuns++
					if *run.Conclusion == "success" {
						successfulRuns++
					}
				}
				if run.DurationSeconds != nil {
					totalDuration += *run.DurationSeconds
				}
			}
		}
		wfCopy.LastRun = lastRun
		wfCopy.TotalRuns = totalRuns
		if completedRuns > 0 {
			wfCopy.SuccessRate = float64(successfulRuns) / float64(completedRuns) * 100.0
		}
		if totalRuns > 0 {
			wfCopy.AvgDuration = totalDuration / totalRuns
		}

		workflows = append(workflows, wfCopy)
	}

	// Sort by name
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	return workflows, nil
}

func (m *MemoryStorage) GetWorkflow(ctx context.Context, id int) (*models.Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wf, ok := m.workflows[id]
	if !ok {
		return nil, errors.New("workflow not found")
	}
	return wf, nil
}

func (m *MemoryStorage) GetWorkflowByGitHubID(ctx context.Context, githubID int64) (*models.Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.workflowGitHubIndex[githubID]
	if !ok {
		return nil, errors.New("workflow not found")
	}
	return m.workflows[id], nil
}

func (m *MemoryStorage) UpsertWorkflow(ctx context.Context, workflow *models.Workflow) (*models.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.workflowGitHubIndex[workflow.GitHubID]; ok {
		// Update existing (preserve IsDeploymentWorkflow - user setting)
		existing := m.workflows[existingID]
		existing.Name = workflow.Name
		existing.Path = workflow.Path
		existing.State = workflow.State
		existing.BadgeURL = workflow.BadgeURL
		existing.HTMLURL = workflow.HTMLURL
		existing.UpdatedAt = time.Now()
		workflow.ID = existing.ID
		workflow.IsDeploymentWorkflow = existing.IsDeploymentWorkflow
		workflow.CreatedAt = existing.CreatedAt
		workflow.UpdatedAt = existing.UpdatedAt
		return workflow, nil
	}

	// Create new
	workflow.ID = int(atomic.AddInt32(&m.workflowIDCounter, 1))
	workflow.CreatedAt = time.Now()
	workflow.UpdatedAt = time.Now()

	m.workflows[workflow.ID] = workflow
	m.workflowGitHubIndex[workflow.GitHubID] = workflow.ID

	return workflow, nil
}

func (m *MemoryStorage) UpdateWorkflow(ctx context.Context, id int, workflow *models.Workflow) (*models.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.workflows[id]
	if !ok {
		return nil, errors.New("workflow not found")
	}
	existing.IsDeploymentWorkflow = workflow.IsDeploymentWorkflow
	existing.UpdatedAt = time.Now()
	workflow.ID = existing.ID
	workflow.GitHubID = existing.GitHubID
	workflow.RepoID = existing.RepoID
	workflow.Name = existing.Name
	workflow.Path = existing.Path
	workflow.State = existing.State
	workflow.BadgeURL = existing.BadgeURL
	workflow.HTMLURL = existing.HTMLURL
	workflow.IsDeploymentWorkflow = existing.IsDeploymentWorkflow
	workflow.CreatedAt = existing.CreatedAt
	workflow.UpdatedAt = existing.UpdatedAt
	return workflow, nil
}

// ===== Workflow Runs =====

func (m *MemoryStorage) ListRuns(ctx context.Context, filters *models.RunFilters, page, pageSize int) ([]models.WorkflowRun, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var runs []models.WorkflowRun
	for _, run := range m.runs {
		// Apply filters
		if filters != nil {
			if filters.WorkflowID != 0 && run.WorkflowID != filters.WorkflowID {
				continue
			}
			if filters.RepoID != 0 && run.RepoID != filters.RepoID {
				continue
			}
			if filters.Status != "" && run.Status != filters.Status {
				continue
			}
			if filters.Conclusion != "" && (run.Conclusion == nil || *run.Conclusion != filters.Conclusion) {
				continue
			}
			if filters.Branch != "" && run.Branch != filters.Branch {
				continue
			}
			if filters.Event != "" && run.Event != filters.Event {
				continue
			}
			if filters.Actor != "" && run.ActorLogin != filters.Actor {
				continue
			}
		}

		runCopy := *run
		// Add workflow and repository info
		if wf, ok := m.workflows[run.WorkflowID]; ok {
			runCopy.Workflow = &models.Workflow{Name: wf.Name}
		}
		if repo, ok := m.repositories[run.RepoID]; ok {
			runCopy.Repository = &models.Repository{FullName: repo.FullName}
		}
		runs = append(runs, runCopy)
	}

	// Sort by started_at descending
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})

	total := len(runs)

	// Apply pagination
	offset := (page - 1) * pageSize
	if offset >= len(runs) {
		return []models.WorkflowRun{}, total, nil
	}
	end := offset + pageSize
	if end > len(runs) {
		end = len(runs)
	}

	return runs[offset:end], total, nil
}

func (m *MemoryStorage) GetRun(ctx context.Context, id int) (*models.WorkflowRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	run, ok := m.runs[id]
	if !ok {
		return nil, errors.New("run not found")
	}
	return run, nil
}

func (m *MemoryStorage) GetRunByGitHubID(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.runGitHubIndex[githubID]
	if !ok {
		return nil, errors.New("run not found")
	}
	return m.runs[id], nil
}

func (m *MemoryStorage) UpsertRun(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.runGitHubIndex[run.GitHubID]; ok {
		// Update existing
		existing := m.runs[existingID]
		existing.Status = run.Status
		existing.Conclusion = run.Conclusion
		existing.CompletedAt = run.CompletedAt
		existing.DurationSeconds = run.DurationSeconds
		existing.IsDeployment = run.IsDeployment
		if run.CommitTimestamp != nil {
			existing.CommitTimestamp = run.CommitTimestamp
		}
		return existing, nil
	}

	// Create new
	run.ID = int(atomic.AddInt32(&m.runIDCounter, 1))
	run.CreatedAt = time.Now()

	m.runs[run.ID] = run
	m.runGitHubIndex[run.GitHubID] = run.ID

	return run, nil
}

// ===== Workflow Jobs =====

func (m *MemoryStorage) ListJobsForRun(ctx context.Context, runID int) ([]models.WorkflowJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var jobs []models.WorkflowJob
	for _, job := range m.jobs {
		if job.RunID == runID {
			jobs = append(jobs, *job)
		}
	}

	// Sort by started_at
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].StartedAt.Before(jobs[j].StartedAt)
	})

	return jobs, nil
}

func (m *MemoryStorage) GetJob(ctx context.Context, id int) (*models.WorkflowJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	job, ok := m.jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}
	return job, nil
}

func (m *MemoryStorage) UpsertJob(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if exists by GitHub ID
	for id, existing := range m.jobs {
		if existing.GitHubID == job.GitHubID {
			existing.Status = job.Status
			existing.Conclusion = job.Conclusion
			existing.CompletedAt = job.CompletedAt
			existing.DurationSeconds = job.DurationSeconds
			existing.Steps = job.Steps
			return m.jobs[id], nil
		}
	}

	// Create new
	job.ID = int(atomic.AddInt32(&m.jobIDCounter, 1))
	job.CreatedAt = time.Now()

	m.jobs[job.ID] = job

	return job, nil
}

// ===== Deployments =====

func (m *MemoryStorage) ListDeployments(ctx context.Context, repoID *int) ([]models.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var deployments []models.Deployment
	for _, d := range m.deployments {
		if repoID != nil && d.RepoID != *repoID {
			continue
		}
		deployments = append(deployments, *d)
	}

	return deployments, nil
}

func (m *MemoryStorage) GetDeployment(ctx context.Context, id int) (*models.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.deployments[id]
	if !ok {
		return nil, errors.New("deployment not found")
	}
	return d, nil
}

func (m *MemoryStorage) UpsertDeployment(ctx context.Context, deployment *models.Deployment) (*models.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if exists by GitHub ID
	for id, existing := range m.deployments {
		if existing.GitHubID == deployment.GitHubID {
			existing.Status = deployment.Status
			existing.DeployedAt = deployment.DeployedAt
			existing.UpdatedAt = time.Now()
			return m.deployments[id], nil
		}
	}

	// Create new
	deployment.ID = int(atomic.AddInt32(&m.deployIDCounter, 1))
	deployment.CreatedAt = time.Now()
	deployment.UpdatedAt = time.Now()

	m.deployments[deployment.ID] = deployment

	return deployment, nil
}

// ===== Users & Sessions =====

func (m *MemoryStorage) GetUserByGitHubID(ctx context.Context, githubID int64) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.userGitHubIndex[githubID]
	if !ok {
		return nil, errors.New("user not found")
	}
	return m.users[id], nil
}

func (m *MemoryStorage) UpsertUser(ctx context.Context, user *models.User) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.userGitHubIndex[user.GitHubID]; ok {
		// Update existing
		existing := m.users[existingID]
		existing.Login = user.Login
		existing.Name = user.Name
		existing.Email = user.Email
		existing.AvatarURL = user.AvatarURL
		existing.AccessToken = user.AccessToken
		existing.RefreshToken = user.RefreshToken
		existing.TokenExpiresAt = user.TokenExpiresAt
		existing.UpdatedAt = time.Now()
		return existing, nil
	}

	// Create new
	user.ID = int(atomic.AddInt32(&m.userIDCounter, 1))
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	m.users[user.ID] = user
	m.userGitHubIndex[user.GitHubID] = user.ID

	return user, nil
}

func (m *MemoryStorage) CreateSession(ctx context.Context, session *models.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session.CreatedAt = time.Now()
	m.sessions[session.ID] = session
	return nil
}

func (m *MemoryStorage) GetSession(ctx context.Context, sessionID string) (*models.Session, *models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil, errors.New("session not found")
	}

	if session.ExpiresAt.Before(time.Now()) {
		return nil, nil, errors.New("session expired")
	}

	user, ok := m.users[session.UserID]
	if !ok {
		return nil, nil, errors.New("user not found")
	}

	return session, user, nil
}

func (m *MemoryStorage) DeleteSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
	return nil
}

func (m *MemoryStorage) CleanExpiredSessions(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, session := range m.sessions {
		if session.ExpiresAt.Before(now) {
			delete(m.sessions, id)
		}
	}
	return nil
}

// ===== Dashboard & Metrics =====

func (m *MemoryStorage) GetDashboardSummary(ctx context.Context) (*models.DashboardSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := &models.DashboardSummary{}

	// Repository stats
	for _, repo := range m.repositories {
		summary.Repositories.Total++
		if repo.IsActive {
			summary.Repositories.Active++
		} else {
			summary.Repositories.Inactive++
		}
	}

	// Workflow stats
	for _, wf := range m.workflows {
		summary.Workflows.Total++
		if strings.ToLower(wf.State) == "active" {
			summary.Workflows.Active++
		} else {
			summary.Workflows.Disabled++
		}
	}

	// Calculate date ranges for current and previous periods (last 30 days and 30-60 days ago)
	now := time.Now()
	currentPeriodStart := now.AddDate(0, 0, -30) // 30 days ago
	previousPeriodStart := now.AddDate(0, 0, -60) // 60 days ago
	previousPeriodEnd := currentPeriodStart

	// Debug logging
	totalRuns := len(m.runs)
	var minDate, maxDate time.Time
	for _, run := range m.runs {
		if minDate.IsZero() || run.StartedAt.Before(minDate) {
			minDate = run.StartedAt
		}
		if maxDate.IsZero() || run.StartedAt.After(maxDate) {
			maxDate = run.StartedAt
		}
	}
	log.Debug().
		Int("total_runs_in_memory", totalRuns).
		Time("min_run_date", minDate).
		Time("max_run_date", maxDate).
		Time("current_period_start", currentPeriodStart).
		Time("previous_period_start", previousPeriodStart).
		Msg("Dashboard summary date ranges")

	for _, run := range m.runs {
		// Current period (last 30 days)
		if !run.StartedAt.Before(currentPeriodStart) {
			summary.Runs.Total++
			if run.Conclusion != nil {
				switch *run.Conclusion {
				case "success":
					summary.Runs.Success++
				case "failure":
					summary.Runs.Failed++
				case "cancelled":
					summary.Runs.Cancelled++
				}
			}
			if run.DurationSeconds != nil {
				summary.Runs.TotalDuration += *run.DurationSeconds
			}
			if run.Status == "in_progress" {
				summary.Runs.InProgress++
			} else if run.Status == "queued" {
				summary.Runs.Queued++
			}
		}

		// Previous period (30-60 days ago)
		if !run.StartedAt.Before(previousPeriodStart) && run.StartedAt.Before(previousPeriodEnd) {
			summary.PreviousRuns.Total++
			if run.Conclusion != nil {
				switch *run.Conclusion {
				case "success":
					summary.PreviousRuns.Success++
				case "failure":
					summary.PreviousRuns.Failed++
				case "cancelled":
					summary.PreviousRuns.Cancelled++
				}
			}
			if run.DurationSeconds != nil {
				summary.PreviousRuns.TotalDuration += *run.DurationSeconds
			}
			if run.Status == "in_progress" {
				summary.PreviousRuns.InProgress++
			} else if run.Status == "queued" {
				summary.PreviousRuns.Queued++
			}
		}
	}

	log.Debug().
		Int("current_period_runs", summary.Runs.Total).
		Int("previous_period_runs", summary.PreviousRuns.Total).
		Msg("Dashboard summary results")

	if summary.Runs.Total > 0 {
		summary.Runs.SuccessRate = float64(summary.Runs.Success) / float64(summary.Runs.Total) * 100
	}
	if summary.PreviousRuns.Total > 0 {
		summary.PreviousRuns.SuccessRate = float64(summary.PreviousRuns.Success) / float64(summary.PreviousRuns.Total) * 100
	}

	// Recent runs (with repository info populated)
	var recentRuns []models.WorkflowRun
	for _, run := range m.runs {
		runCopy := *run
		// Populate repository info
		if repo, ok := m.repositories[run.RepoID]; ok {
			runCopy.Repository = &models.Repository{
				ID:       repo.ID,
				FullName: repo.FullName,
				Name:     repo.Name,
			}
		}
		recentRuns = append(recentRuns, runCopy)
	}
	sort.Slice(recentRuns, func(i, j int) bool {
		return recentRuns[i].StartedAt.After(recentRuns[j].StartedAt)
	})
	if len(recentRuns) > 10 {
		recentRuns = recentRuns[:10]
	}
	summary.RecentRuns = recentRuns

	return summary, nil
}

func (m *MemoryStorage) GetTrends(ctx context.Context, days int) ([]models.Trend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	startDate := time.Now().AddDate(0, 0, -days)

	// Group runs by day
	dailyStats := make(map[string]*models.Trend)

	for _, run := range m.runs {
		if run.StartedAt.Before(startDate) {
			continue
		}

		day := run.StartedAt.Format("2006-01-02")
		if _, ok := dailyStats[day]; !ok {
			date, _ := time.Parse("2006-01-02", day)
			dailyStats[day] = &models.Trend{Date: date}
		}

		stat := dailyStats[day]
		stat.TotalRuns++
		if run.Conclusion != nil {
			switch *run.Conclusion {
			case "success":
				stat.SuccessfulRuns++
			case "failure":
				stat.FailedRuns++
			}
		}
		if run.DurationSeconds != nil {
			stat.AvgDuration = (stat.AvgDuration*(stat.TotalRuns-1) + *run.DurationSeconds) / stat.TotalRuns
		}
		if run.IsDeployment {
			stat.DeploymentCount++
		}
	}

	// Convert to slice and sort
	var trends []models.Trend
	for _, trend := range dailyStats {
		trends = append(trends, *trend)
	}
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].Date.Before(trends[j].Date)
	})

	return trends, nil
}

func (m *MemoryStorage) GetDevOpsMetrics(ctx context.Context, startDate, endDate time.Time) (*models.DevOpsMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	days := int(endDate.Sub(startDate).Hours() / 24)
	if days <= 0 {
		days = 1
	}

	metrics := &models.DevOpsMetrics{
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Count deployments and calculate metrics
	var totalDeploys, failedDeploys int
	var leadTimeMinutes []int

	for _, run := range m.runs {
		if run.StartedAt.Before(startDate) || run.StartedAt.After(endDate) {
			continue
		}

		if run.IsDeployment {
			totalDeploys++
			if run.Conclusion != nil && *run.Conclusion == "failure" {
				failedDeploys++
			}
			// Lead time: commit-to-deploy when commit_timestamp set, else run duration
			if run.CompletedAt != nil {
				var mins int
				if run.CommitTimestamp != nil {
					mins = int(run.CompletedAt.Sub(*run.CommitTimestamp).Minutes())
				} else if run.DurationSeconds != nil {
					mins = *run.DurationSeconds / 60
				}
				if mins >= 0 {
					leadTimeMinutes = append(leadTimeMinutes, mins)
				}
			}
		}
	}

	// Deployment Frequency
	deploysPerDay := float64(totalDeploys) / float64(days)
	metrics.DeploymentFrequency = models.DeploymentFrequency{
		TotalDeployments:   totalDeploys,
		DeploymentsPerDay:  deploysPerDay,
		DeploymentsPerWeek: deploysPerDay * 7,
		Rating:             getDeploymentFrequencyRating(deploysPerDay),
	}

	// Lead Time (median of commit-to-deploy or duration)
	medianMinutes := 0
	if len(leadTimeMinutes) > 0 {
		sort.Ints(leadTimeMinutes)
		medianMinutes = leadTimeMinutes[len(leadTimeMinutes)/2]
	}
	metrics.LeadTime = models.LeadTime{
		MedianMinutes: medianMinutes,
		Rating:        getLeadTimeRating(medianMinutes),
	}

	// Change Failure Rate
	failureRate := 0.0
	if totalDeploys > 0 {
		failureRate = float64(failedDeploys) / float64(totalDeploys) * 100
	}
	metrics.ChangeFailureRate = models.ChangeFailureRate{
		TotalDeployments:  totalDeploys,
		FailedDeployments: failedDeploys,
		Rate:              failureRate,
		Rating:            getChangeFailureRateRating(failureRate),
	}

	// MTTR: time from failed deployment to next successful deployment
	var recoveryMinutes []int
	for _, run := range m.runs {
		if !run.IsDeployment || run.Conclusion == nil || *run.Conclusion != "failure" || run.CompletedAt == nil {
			continue
		}
		if run.StartedAt.Before(startDate) {
			continue
		}
		failedAt := *run.CompletedAt
		var minRecovery *time.Duration
		for _, r2 := range m.runs {
			if !r2.IsDeployment || r2.Conclusion == nil || *r2.Conclusion != "success" || r2.CompletedAt == nil {
				continue
			}
			if r2.CompletedAt.After(failedAt) {
				d := r2.CompletedAt.Sub(failedAt)
				if minRecovery == nil || d < *minRecovery {
					minRecovery = &d
				}
			}
		}
		if minRecovery != nil && *minRecovery > 0 {
			recoveryMinutes = append(recoveryMinutes, int(minRecovery.Minutes()))
		}
	}
	medianMTTR := 60
	p95MTTR := 60
	if len(recoveryMinutes) > 0 {
		sort.Ints(recoveryMinutes)
		medianMTTR = recoveryMinutes[len(recoveryMinutes)/2]
		p95Idx := int(float64(len(recoveryMinutes)) * 0.95)
		if p95Idx >= len(recoveryMinutes) {
			p95Idx = len(recoveryMinutes) - 1
		}
		p95MTTR = recoveryMinutes[p95Idx]
	}
	metrics.MTTR = models.MTTR{
		MedianMinutes: medianMTTR,
		P95Minutes:    p95MTTR,
		Rating:        getMTTRRating(medianMTTR),
	}

	return metrics, nil
}

// backfillIsDeploymentRun returns true if the workflow run should be counted as a deployment (same heuristic as handlers).
func backfillIsDeploymentRun(workflowName, workflowPath, event string) bool {
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

// BackfillDeploymentRuns sets IsDeployment on runs that match deployment heuristics.
func (m *MemoryStorage) BackfillDeploymentRuns(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated := 0
	for _, run := range m.runs {
		wf, ok := m.workflows[run.WorkflowID]
		if !ok {
			continue
		}
		shouldBeDeploy := wf.IsDeploymentWorkflow || backfillIsDeploymentRun(wf.Name, wf.Path, run.Event)
		if shouldBeDeploy && !run.IsDeployment {
			run.IsDeployment = true
			updated++
		}
	}
	return updated, nil
}

// Helper functions for ratings
func getDeploymentFrequencyRating(deploysPerDay float64) string {
	switch {
	case deploysPerDay >= 1:
		return "elite"
	case deploysPerDay >= 0.14:
		return "high"
	case deploysPerDay >= 0.033:
		return "medium"
	default:
		return "low"
	}
}

func getLeadTimeRating(minutes int) string {
	switch {
	case minutes <= 60:
		return "elite"
	case minutes <= 1440:
		return "high"
	case minutes <= 10080:
		return "medium"
	default:
		return "low"
	}
}

func getChangeFailureRateRating(rate float64) string {
	switch {
	case rate <= 15:
		return "elite"
	case rate <= 30:
		return "high"
	case rate <= 45:
		return "medium"
	default:
		return "low"
	}
}

func getMTTRRating(medianMinutes int) string {
	switch {
	case medianMinutes <= 60:
		return "elite"
	case medianMinutes <= 1440:
		return "high"
	case medianMinutes <= 10080:
		return "medium"
	default:
		return "low"
	}
}

