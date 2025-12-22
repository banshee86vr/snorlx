package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"snorlx/backend/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// DatabaseStorage implements Storage interface using PostgreSQL + TimescaleDB
type DatabaseStorage struct {
	pool *pgxpool.Pool
}

// NewDatabaseStorage creates a new database storage instance
func NewDatabaseStorage(databaseURL string) (*DatabaseStorage, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	log.Info().Msg("Database connection established (PostgreSQL)")

	return &DatabaseStorage{pool: pool}, nil
}

// Close closes the database connection pool
func (d *DatabaseStorage) Close() error {
	d.pool.Close()
	return nil
}

// Migrate runs database migrations
func (d *DatabaseStorage) Migrate() error {
	ctx := context.Background()

	_, err := d.pool.Exec(ctx, migrationSQL)
	if err != nil {
		return err
	}

	log.Info().Msg("Database migrations completed")
	return nil
}

// ===== Organizations =====

func (d *DatabaseStorage) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, github_id, login, name, avatar_url, settings, created_at, updated_at
		FROM organizations
		ORDER BY login
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []models.Organization
	for rows.Next() {
		var org models.Organization
		err := rows.Scan(&org.ID, &org.GitHubID, &org.Login, &org.Name, &org.AvatarURL, &org.Settings, &org.CreatedAt, &org.UpdatedAt)
		if err != nil {
			continue
		}
		orgs = append(orgs, org)
	}

	return orgs, nil
}

func (d *DatabaseStorage) GetOrganization(ctx context.Context, id int) (*models.Organization, error) {
	var org models.Organization
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, login, name, avatar_url, settings, created_at, updated_at
		FROM organizations WHERE id = $1
	`, id).Scan(&org.ID, &org.GitHubID, &org.Login, &org.Name, &org.AvatarURL, &org.Settings, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, errors.New("organization not found")
	}
	return &org, nil
}

func (d *DatabaseStorage) GetOrganizationByGitHubID(ctx context.Context, githubID int64) (*models.Organization, error) {
	var org models.Organization
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, login, name, avatar_url, settings, created_at, updated_at
		FROM organizations WHERE github_id = $1
	`, githubID).Scan(&org.ID, &org.GitHubID, &org.Login, &org.Name, &org.AvatarURL, &org.Settings, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, errors.New("organization not found")
	}
	return &org, nil
}

func (d *DatabaseStorage) UpsertOrganization(ctx context.Context, org *models.Organization) (*models.Organization, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO organizations (github_id, login, name, avatar_url, settings)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (github_id) DO UPDATE SET
			login = EXCLUDED.login,
			name = EXCLUDED.name,
			avatar_url = EXCLUDED.avatar_url,
			settings = EXCLUDED.settings,
			updated_at = NOW()
		RETURNING id, github_id, login, name, avatar_url, settings, created_at, updated_at
	`, org.GitHubID, org.Login, org.Name, org.AvatarURL, org.Settings).Scan(
		&org.ID, &org.GitHubID, &org.Login, &org.Name, &org.AvatarURL, &org.Settings, &org.CreatedAt, &org.UpdatedAt,
	)
	return org, err
}

// ===== Repositories =====

func (d *DatabaseStorage) ListRepositories(ctx context.Context, page, pageSize int) ([]models.Repository, int, error) {
	offset := (page - 1) * pageSize

	// Get total count
	var total int
	d.pool.QueryRow(ctx, "SELECT COUNT(*) FROM repositories WHERE is_active = true").Scan(&total)

	rows, err := d.pool.Query(ctx, `
		SELECT r.id, r.github_id, r.org_id, r.name, r.full_name, r.description, 
		       r.default_branch, r.html_url, r.is_private, r.is_active, r.settings,
		       r.created_at, r.updated_at,
		       COUNT(DISTINCT w.id) as workflow_count
		FROM repositories r
		LEFT JOIN workflows w ON w.repo_id = r.id
		WHERE r.is_active = true
		GROUP BY r.id
		ORDER BY r.full_name
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var repos []models.Repository
	for rows.Next() {
		var repo models.Repository
		err := rows.Scan(
			&repo.ID, &repo.GitHubID, &repo.OrgID, &repo.Name, &repo.FullName, &repo.Description,
			&repo.DefaultBranch, &repo.HTMLURL, &repo.IsPrivate, &repo.IsActive, &repo.Settings,
			&repo.CreatedAt, &repo.UpdatedAt, &repo.WorkflowCount,
		)
		if err != nil {
			continue
		}
		repos = append(repos, repo)
	}

	return repos, total, nil
}

func (d *DatabaseStorage) GetRepository(ctx context.Context, id int) (*models.Repository, error) {
	var repo models.Repository
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, org_id, name, full_name, description, 
		       default_branch, html_url, is_private, is_active, settings,
		       created_at, updated_at
		FROM repositories WHERE id = $1
	`, id).Scan(
		&repo.ID, &repo.GitHubID, &repo.OrgID, &repo.Name, &repo.FullName, &repo.Description,
		&repo.DefaultBranch, &repo.HTMLURL, &repo.IsPrivate, &repo.IsActive, &repo.Settings,
		&repo.CreatedAt, &repo.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("repository not found")
	}
	return &repo, nil
}

func (d *DatabaseStorage) GetRepositoryByGitHubID(ctx context.Context, githubID int64) (*models.Repository, error) {
	var repo models.Repository
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, org_id, name, full_name, description, 
		       default_branch, html_url, is_private, is_active, settings,
		       created_at, updated_at
		FROM repositories WHERE github_id = $1
	`, githubID).Scan(
		&repo.ID, &repo.GitHubID, &repo.OrgID, &repo.Name, &repo.FullName, &repo.Description,
		&repo.DefaultBranch, &repo.HTMLURL, &repo.IsPrivate, &repo.IsActive, &repo.Settings,
		&repo.CreatedAt, &repo.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("repository not found")
	}
	return &repo, nil
}

func (d *DatabaseStorage) UpsertRepository(ctx context.Context, repo *models.Repository) (*models.Repository, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO repositories (github_id, org_id, name, full_name, description, default_branch, html_url, is_private, is_active, settings)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (github_id) DO UPDATE SET
			name = EXCLUDED.name,
			full_name = EXCLUDED.full_name,
			description = EXCLUDED.description,
			default_branch = EXCLUDED.default_branch,
			html_url = EXCLUDED.html_url,
			is_private = EXCLUDED.is_private,
			is_active = EXCLUDED.is_active,
			settings = EXCLUDED.settings,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`, repo.GitHubID, repo.OrgID, repo.Name, repo.FullName, repo.Description, repo.DefaultBranch, repo.HTMLURL, repo.IsPrivate, repo.IsActive, repo.Settings).Scan(
		&repo.ID, &repo.CreatedAt, &repo.UpdatedAt,
	)
	return repo, err
}

func (d *DatabaseStorage) UpdateRepository(ctx context.Context, id int, repo *models.Repository) (*models.Repository, error) {
	err := d.pool.QueryRow(ctx, `
		UPDATE repositories SET
			name = $2, full_name = $3, description = $4, default_branch = $5,
			html_url = $6, is_private = $7, is_active = $8, settings = $9, updated_at = NOW()
		WHERE id = $1
		RETURNING id, github_id, org_id, name, full_name, description, default_branch, html_url, is_private, is_active, settings, created_at, updated_at
	`, id, repo.Name, repo.FullName, repo.Description, repo.DefaultBranch, repo.HTMLURL, repo.IsPrivate, repo.IsActive, repo.Settings).Scan(
		&repo.ID, &repo.GitHubID, &repo.OrgID, &repo.Name, &repo.FullName, &repo.Description,
		&repo.DefaultBranch, &repo.HTMLURL, &repo.IsPrivate, &repo.IsActive, &repo.Settings,
		&repo.CreatedAt, &repo.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("repository not found")
	}
	return repo, nil
}

// ===== Workflows =====

func (d *DatabaseStorage) ListWorkflows(ctx context.Context, repoID *int) ([]models.Workflow, error) {
	query := `
		SELECT w.id, w.github_id, w.repo_id, w.name, w.path, w.state, w.badge_url, w.html_url,
		       w.created_at, w.updated_at,
		       r.full_name as repo_full_name
		FROM workflows w
		JOIN repositories r ON r.id = w.repo_id
		WHERE 1=1
	`
	args := []interface{}{}

	if repoID != nil {
		query += " AND w.repo_id = $1"
		args = append(args, *repoID)
	}
	query += " ORDER BY w.name"

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []models.Workflow
	for rows.Next() {
		var wf models.Workflow
		var repoFullName string
		err := rows.Scan(
			&wf.ID, &wf.GitHubID, &wf.RepoID, &wf.Name, &wf.Path, &wf.State, &wf.BadgeURL, &wf.HTMLURL,
			&wf.CreatedAt, &wf.UpdatedAt, &repoFullName,
		)
		if err != nil {
			continue
		}
		wf.Repository = &models.Repository{FullName: repoFullName}
		workflows = append(workflows, wf)
	}

	return workflows, nil
}

func (d *DatabaseStorage) GetWorkflow(ctx context.Context, id int) (*models.Workflow, error) {
	var wf models.Workflow
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, repo_id, name, path, state, badge_url, html_url, created_at, updated_at
		FROM workflows WHERE id = $1
	`, id).Scan(
		&wf.ID, &wf.GitHubID, &wf.RepoID, &wf.Name, &wf.Path, &wf.State, &wf.BadgeURL, &wf.HTMLURL,
		&wf.CreatedAt, &wf.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("workflow not found")
	}
	return &wf, nil
}

func (d *DatabaseStorage) GetWorkflowByGitHubID(ctx context.Context, githubID int64) (*models.Workflow, error) {
	var wf models.Workflow
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, repo_id, name, path, state, badge_url, html_url, created_at, updated_at
		FROM workflows WHERE github_id = $1
	`, githubID).Scan(
		&wf.ID, &wf.GitHubID, &wf.RepoID, &wf.Name, &wf.Path, &wf.State, &wf.BadgeURL, &wf.HTMLURL,
		&wf.CreatedAt, &wf.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("workflow not found")
	}
	return &wf, nil
}

func (d *DatabaseStorage) UpsertWorkflow(ctx context.Context, workflow *models.Workflow) (*models.Workflow, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO workflows (github_id, repo_id, name, path, state, badge_url, html_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (github_id) DO UPDATE SET
			name = EXCLUDED.name,
			path = EXCLUDED.path,
			state = EXCLUDED.state,
			badge_url = EXCLUDED.badge_url,
			html_url = EXCLUDED.html_url,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`, workflow.GitHubID, workflow.RepoID, workflow.Name, workflow.Path, workflow.State, workflow.BadgeURL, workflow.HTMLURL).Scan(
		&workflow.ID, &workflow.CreatedAt, &workflow.UpdatedAt,
	)
	return workflow, err
}

// ===== Workflow Runs =====

func (d *DatabaseStorage) ListRuns(ctx context.Context, filters *models.RunFilters, page, pageSize int) ([]models.WorkflowRun, int, error) {
	offset := (page - 1) * pageSize

	query := `
		SELECT wr.id, wr.github_id, wr.workflow_id, wr.repo_id, wr.run_number, wr.name,
		       wr.status, wr.conclusion, wr.event, wr.branch, wr.commit_sha, wr.commit_message,
		       wr.actor_login, wr.actor_avatar, wr.html_url, wr.started_at, wr.completed_at,
		       wr.duration_seconds, wr.is_deployment, wr.environment, wr.created_at,
		       w.name as workflow_name, r.full_name as repo_full_name
		FROM workflow_runs wr
		JOIN workflows w ON w.id = wr.workflow_id
		JOIN repositories r ON r.id = wr.repo_id
		WHERE 1=1
	`
	countQuery := "SELECT COUNT(*) FROM workflow_runs wr WHERE 1=1"
	args := []interface{}{}
	argCount := 0

	// Apply filters
	if filters != nil {
		if filters.WorkflowID != 0 {
			argCount++
			query += fmt.Sprintf(" AND wr.workflow_id = $%d", argCount)
			countQuery += fmt.Sprintf(" AND wr.workflow_id = $%d", argCount)
			args = append(args, filters.WorkflowID)
		}
		if filters.RepoID != 0 {
			argCount++
			query += fmt.Sprintf(" AND wr.repo_id = $%d", argCount)
			countQuery += fmt.Sprintf(" AND wr.repo_id = $%d", argCount)
			args = append(args, filters.RepoID)
		}
		if filters.Status != "" {
			argCount++
			query += fmt.Sprintf(" AND wr.status = $%d", argCount)
			countQuery += fmt.Sprintf(" AND wr.status = $%d", argCount)
			args = append(args, filters.Status)
		}
		if filters.Conclusion != "" {
			argCount++
			query += fmt.Sprintf(" AND wr.conclusion = $%d", argCount)
			countQuery += fmt.Sprintf(" AND wr.conclusion = $%d", argCount)
			args = append(args, filters.Conclusion)
		}
		if filters.Branch != "" {
			argCount++
			query += fmt.Sprintf(" AND wr.branch = $%d", argCount)
			countQuery += fmt.Sprintf(" AND wr.branch = $%d", argCount)
			args = append(args, filters.Branch)
		}
	}

	// Get total count
	var total int
	d.pool.QueryRow(ctx, countQuery, args...).Scan(&total)

	// Add pagination
	query += fmt.Sprintf(" ORDER BY wr.started_at DESC LIMIT $%d OFFSET $%d", argCount+1, argCount+2)
	args = append(args, pageSize, offset)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []models.WorkflowRun
	for rows.Next() {
		var run models.WorkflowRun
		var workflowName, repoFullName string
		err := rows.Scan(
			&run.ID, &run.GitHubID, &run.WorkflowID, &run.RepoID, &run.RunNumber, &run.Name,
			&run.Status, &run.Conclusion, &run.Event, &run.Branch, &run.CommitSHA, &run.CommitMessage,
			&run.ActorLogin, &run.ActorAvatar, &run.HTMLURL, &run.StartedAt, &run.CompletedAt,
			&run.DurationSeconds, &run.IsDeployment, &run.Environment, &run.CreatedAt,
			&workflowName, &repoFullName,
		)
		if err != nil {
			continue
		}
		run.Workflow = &models.Workflow{Name: workflowName}
		run.Repository = &models.Repository{FullName: repoFullName}
		runs = append(runs, run)
	}

	return runs, total, nil
}

func (d *DatabaseStorage) GetRun(ctx context.Context, id int) (*models.WorkflowRun, error) {
	var run models.WorkflowRun
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, workflow_id, repo_id, run_number, name,
		       status, conclusion, event, branch, commit_sha, commit_message,
		       actor_login, actor_avatar, html_url, started_at, completed_at,
		       duration_seconds, is_deployment, environment, created_at
		FROM workflow_runs WHERE id = $1
	`, id).Scan(
		&run.ID, &run.GitHubID, &run.WorkflowID, &run.RepoID, &run.RunNumber, &run.Name,
		&run.Status, &run.Conclusion, &run.Event, &run.Branch, &run.CommitSHA, &run.CommitMessage,
		&run.ActorLogin, &run.ActorAvatar, &run.HTMLURL, &run.StartedAt, &run.CompletedAt,
		&run.DurationSeconds, &run.IsDeployment, &run.Environment, &run.CreatedAt,
	)
	if err != nil {
		return nil, errors.New("run not found")
	}
	return &run, nil
}

func (d *DatabaseStorage) GetRunByGitHubID(ctx context.Context, githubID int64) (*models.WorkflowRun, error) {
	var run models.WorkflowRun
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, workflow_id, repo_id, run_number, name,
		       status, conclusion, event, branch, commit_sha, commit_message,
		       actor_login, actor_avatar, html_url, started_at, completed_at,
		       duration_seconds, is_deployment, environment, created_at
		FROM workflow_runs WHERE github_id = $1
	`, githubID).Scan(
		&run.ID, &run.GitHubID, &run.WorkflowID, &run.RepoID, &run.RunNumber, &run.Name,
		&run.Status, &run.Conclusion, &run.Event, &run.Branch, &run.CommitSHA, &run.CommitMessage,
		&run.ActorLogin, &run.ActorAvatar, &run.HTMLURL, &run.StartedAt, &run.CompletedAt,
		&run.DurationSeconds, &run.IsDeployment, &run.Environment, &run.CreatedAt,
	)
	if err != nil {
		return nil, errors.New("run not found")
	}
	return &run, nil
}

func (d *DatabaseStorage) UpsertRun(ctx context.Context, run *models.WorkflowRun) (*models.WorkflowRun, error) {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO workflow_runs (
			github_id, workflow_id, repo_id, run_number, name, status, conclusion,
			event, branch, commit_sha, commit_message, actor_login, actor_avatar,
			html_url, started_at, completed_at, duration_seconds, is_deployment, environment
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (github_id) DO UPDATE SET
			status = EXCLUDED.status,
			conclusion = EXCLUDED.conclusion,
			completed_at = EXCLUDED.completed_at,
			duration_seconds = EXCLUDED.duration_seconds
	`,
		run.GitHubID, run.WorkflowID, run.RepoID, run.RunNumber, run.Name, run.Status, run.Conclusion,
		run.Event, run.Branch, run.CommitSHA, run.CommitMessage, run.ActorLogin, run.ActorAvatar,
		run.HTMLURL, run.StartedAt, run.CompletedAt, run.DurationSeconds, run.IsDeployment, run.Environment,
	)
	return run, err
}

// ===== Workflow Jobs =====

func (d *DatabaseStorage) ListJobsForRun(ctx context.Context, runID int) ([]models.WorkflowJob, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, github_id, run_id, name, status, conclusion, runner_name, runner_group,
		       labels, steps, started_at, completed_at, duration_seconds, created_at
		FROM workflow_jobs WHERE run_id = $1
		ORDER BY started_at
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.WorkflowJob
	for rows.Next() {
		var job models.WorkflowJob
		err := rows.Scan(
			&job.ID, &job.GitHubID, &job.RunID, &job.Name, &job.Status, &job.Conclusion,
			&job.RunnerName, &job.RunnerGroup, &job.Labels, &job.Steps,
			&job.StartedAt, &job.CompletedAt, &job.DurationSeconds, &job.CreatedAt,
		)
		if err != nil {
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

func (d *DatabaseStorage) GetJob(ctx context.Context, id int) (*models.WorkflowJob, error) {
	var job models.WorkflowJob
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, run_id, name, status, conclusion, runner_name, runner_group,
		       labels, steps, started_at, completed_at, duration_seconds, created_at
		FROM workflow_jobs WHERE id = $1
	`, id).Scan(
		&job.ID, &job.GitHubID, &job.RunID, &job.Name, &job.Status, &job.Conclusion,
		&job.RunnerName, &job.RunnerGroup, &job.Labels, &job.Steps,
		&job.StartedAt, &job.CompletedAt, &job.DurationSeconds, &job.CreatedAt,
	)
	if err != nil {
		return nil, errors.New("job not found")
	}
	return &job, nil
}

func (d *DatabaseStorage) UpsertJob(ctx context.Context, job *models.WorkflowJob) (*models.WorkflowJob, error) {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO workflow_jobs (
			github_id, run_id, run_github_id, name, status, conclusion, runner_name, runner_group,
			labels, steps, started_at, completed_at, duration_seconds
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (github_id) DO UPDATE SET
			status = EXCLUDED.status,
			conclusion = EXCLUDED.conclusion,
			completed_at = EXCLUDED.completed_at,
			duration_seconds = EXCLUDED.duration_seconds,
			steps = EXCLUDED.steps
	`,
		job.GitHubID, job.RunID, job.GitHubID, job.Name, job.Status, job.Conclusion, job.RunnerName, job.RunnerGroup,
		job.Labels, job.Steps, job.StartedAt, job.CompletedAt, job.DurationSeconds,
	)
	return job, err
}

// ===== Deployments =====

func (d *DatabaseStorage) ListDeployments(ctx context.Context, repoID *int) ([]models.Deployment, error) {
	query := "SELECT id, github_id, repo_id, run_id, environment, status, description, creator_login, sha, ref, deployed_at, created_at, updated_at FROM deployments WHERE 1=1"
	args := []interface{}{}
	if repoID != nil {
		query += " AND repo_id = $1"
		args = append(args, *repoID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.Deployment
	for rows.Next() {
		var dep models.Deployment
		err := rows.Scan(
			&dep.ID, &dep.GitHubID, &dep.RepoID, &dep.RunID, &dep.Environment, &dep.Status,
			&dep.Description, &dep.CreatorLogin, &dep.SHA, &dep.Ref, &dep.DeployedAt,
			&dep.CreatedAt, &dep.UpdatedAt,
		)
		if err != nil {
			continue
		}
		deployments = append(deployments, dep)
	}

	return deployments, nil
}

func (d *DatabaseStorage) GetDeployment(ctx context.Context, id int) (*models.Deployment, error) {
	var dep models.Deployment
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, repo_id, run_id, environment, status, description, creator_login, sha, ref, deployed_at, created_at, updated_at
		FROM deployments WHERE id = $1
	`, id).Scan(
		&dep.ID, &dep.GitHubID, &dep.RepoID, &dep.RunID, &dep.Environment, &dep.Status,
		&dep.Description, &dep.CreatorLogin, &dep.SHA, &dep.Ref, &dep.DeployedAt,
		&dep.CreatedAt, &dep.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("deployment not found")
	}
	return &dep, nil
}

func (d *DatabaseStorage) UpsertDeployment(ctx context.Context, deployment *models.Deployment) (*models.Deployment, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO deployments (github_id, repo_id, run_id, environment, status, description, creator_login, sha, ref, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (github_id) DO UPDATE SET
			status = EXCLUDED.status,
			deployed_at = EXCLUDED.deployed_at,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`, deployment.GitHubID, deployment.RepoID, deployment.RunID, deployment.Environment, deployment.Status,
		deployment.Description, deployment.CreatorLogin, deployment.SHA, deployment.Ref, deployment.DeployedAt).Scan(
		&deployment.ID, &deployment.CreatedAt, &deployment.UpdatedAt,
	)
	return deployment, err
}

// ===== Users & Sessions =====

func (d *DatabaseStorage) GetUserByGitHubID(ctx context.Context, githubID int64) (*models.User, error) {
	var user models.User
	err := d.pool.QueryRow(ctx, `
		SELECT id, github_id, login, name, email, avatar_url, access_token, refresh_token, token_expires_at, created_at, updated_at
		FROM users WHERE github_id = $1
	`, githubID).Scan(
		&user.ID, &user.GitHubID, &user.Login, &user.Name, &user.Email, &user.AvatarURL,
		&user.AccessToken, &user.RefreshToken, &user.TokenExpiresAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, errors.New("user not found")
	}
	return &user, nil
}

func (d *DatabaseStorage) UpsertUser(ctx context.Context, user *models.User) (*models.User, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO users (github_id, login, name, email, avatar_url, access_token, token_expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (github_id) DO UPDATE SET
			login = EXCLUDED.login,
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			avatar_url = EXCLUDED.avatar_url,
			access_token = EXCLUDED.access_token,
			token_expires_at = EXCLUDED.token_expires_at,
			updated_at = NOW()
		RETURNING id, github_id, login, name, email, avatar_url
	`, user.GitHubID, user.Login, user.Name, user.Email, user.AvatarURL, user.AccessToken, user.TokenExpiresAt).Scan(
		&user.ID, &user.GitHubID, &user.Login, &user.Name, &user.Email, &user.AvatarURL,
	)
	return user, err
}

func (d *DatabaseStorage) CreateSession(ctx context.Context, session *models.Session) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, session.ID, session.UserID, session.ExpiresAt)
	return err
}

func (d *DatabaseStorage) GetSession(ctx context.Context, sessionID string) (*models.Session, *models.User, error) {
	var user models.User
	var session models.Session

	err := d.pool.QueryRow(ctx, `
		SELECT u.id, u.github_id, u.login, u.name, u.email, u.avatar_url,
		       s.id, s.user_id, s.expires_at, s.created_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.id = $1 AND s.expires_at > NOW()
	`, sessionID).Scan(
		&user.ID, &user.GitHubID, &user.Login, &user.Name, &user.Email, &user.AvatarURL,
		&session.ID, &session.UserID, &session.ExpiresAt, &session.CreatedAt,
	)
	if err != nil {
		return nil, nil, errors.New("session not found or expired")
	}

	return &session, &user, nil
}

func (d *DatabaseStorage) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := d.pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", sessionID)
	return err
}

func (d *DatabaseStorage) CleanExpiredSessions(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, "DELETE FROM sessions WHERE expires_at < NOW()")
	return err
}

// ===== Dashboard & Metrics =====

func (d *DatabaseStorage) GetDashboardSummary(ctx context.Context) (*models.DashboardSummary, error) {
	summary := &models.DashboardSummary{}

	// Get repository stats
	d.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE is_active = true),
			COUNT(*) FILTER (WHERE is_active = false)
		FROM repositories
	`).Scan(&summary.Repositories.Total, &summary.Repositories.Active, &summary.Repositories.Inactive)

	// Get workflow stats
	d.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE state = 'active'),
			COUNT(*) FILTER (WHERE state = 'disabled')
		FROM workflows
	`).Scan(&summary.Workflows.Total, &summary.Workflows.Active, &summary.Workflows.Disabled)

	// Get run stats (last 24 hours)
	d.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE conclusion = 'success'),
			COUNT(*) FILTER (WHERE conclusion = 'failure'),
			COUNT(*) FILTER (WHERE status = 'in_progress'),
			COUNT(*) FILTER (WHERE status = 'queued'),
			COUNT(*) FILTER (WHERE conclusion = 'cancelled')
		FROM workflow_runs
		WHERE started_at >= NOW() - INTERVAL '24 hours'
	`).Scan(
		&summary.Runs.Total,
		&summary.Runs.Success,
		&summary.Runs.Failed,
		&summary.Runs.InProgress,
		&summary.Runs.Queued,
		&summary.Runs.Cancelled,
	)

	if summary.Runs.Total > 0 {
		summary.Runs.SuccessRate = float64(summary.Runs.Success) / float64(summary.Runs.Total) * 100
	}

	// Get recent runs
	rows, _ := d.pool.Query(ctx, `
		SELECT id, github_id, workflow_id, repo_id, run_number, name,
		       status, conclusion, event, branch, commit_sha, actor_login,
		       html_url, started_at, completed_at, duration_seconds
		FROM workflow_runs
		ORDER BY started_at DESC
		LIMIT 10
	`)
	defer rows.Close()

	for rows.Next() {
		var run models.WorkflowRun
		rows.Scan(
			&run.ID, &run.GitHubID, &run.WorkflowID, &run.RepoID, &run.RunNumber, &run.Name,
			&run.Status, &run.Conclusion, &run.Event, &run.Branch, &run.CommitSHA, &run.ActorLogin,
			&run.HTMLURL, &run.StartedAt, &run.CompletedAt, &run.DurationSeconds,
		)
		summary.RecentRuns = append(summary.RecentRuns, run)
	}

	return summary, nil
}

func (d *DatabaseStorage) GetTrends(ctx context.Context, days int) ([]models.Trend, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT 
			DATE_TRUNC('day', started_at) as date,
			COUNT(*) as total_runs,
			COUNT(*) FILTER (WHERE conclusion = 'success') as successful_runs,
			COUNT(*) FILTER (WHERE conclusion = 'failure') as failed_runs,
			COALESCE(AVG(duration_seconds), 0) as avg_duration,
			COUNT(*) FILTER (WHERE is_deployment = true) as deployment_count
		FROM workflow_runs
		WHERE started_at >= NOW() - INTERVAL '1 day' * $1
		GROUP BY DATE_TRUNC('day', started_at)
		ORDER BY date
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []models.Trend
	for rows.Next() {
		var trend models.Trend
		rows.Scan(
			&trend.Date, &trend.TotalRuns, &trend.SuccessfulRuns,
			&trend.FailedRuns, &trend.AvgDuration, &trend.DeploymentCount,
		)
		trends = append(trends, trend)
	}

	return trends, nil
}

func (d *DatabaseStorage) GetDevOpsMetrics(ctx context.Context, startDate, endDate time.Time) (*models.DevOpsMetrics, error) {
	days := int(endDate.Sub(startDate).Hours() / 24)
	if days <= 0 {
		days = 1
	}

	metrics := &models.DevOpsMetrics{
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Get deployment frequency
	var totalDeploys int
	d.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workflow_runs
		WHERE is_deployment = true AND started_at >= $1
	`, startDate).Scan(&totalDeploys)

	deploysPerDay := float64(totalDeploys) / float64(days)
	metrics.DeploymentFrequency = models.DeploymentFrequency{
		TotalDeployments:   totalDeploys,
		DeploymentsPerDay:  deploysPerDay,
		DeploymentsPerWeek: deploysPerDay * 7,
		Rating:             getDeploymentFrequencyRating(deploysPerDay),
	}

	// Get lead time
	var medianLeadTime int
	d.pool.QueryRow(ctx, `
		SELECT COALESCE(
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_seconds), 0
		)
		FROM workflow_runs
		WHERE is_deployment = true AND completed_at IS NOT NULL AND started_at >= $1
	`, startDate).Scan(&medianLeadTime)

	metrics.LeadTime = models.LeadTime{
		MedianMinutes: medianLeadTime / 60,
		Rating:        getLeadTimeRating(medianLeadTime / 60),
	}

	// Get change failure rate
	var totalRuns, failedRuns int
	d.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE conclusion = 'failure')
		FROM workflow_runs
		WHERE is_deployment = true AND started_at >= $1
	`, startDate).Scan(&totalRuns, &failedRuns)

	failureRate := 0.0
	if totalRuns > 0 {
		failureRate = float64(failedRuns) / float64(totalRuns) * 100
	}

	metrics.ChangeFailureRate = models.ChangeFailureRate{
		TotalDeployments:  totalRuns,
		FailedDeployments: failedRuns,
		Rate:              failureRate,
		Rating:            getChangeFailureRateRating(failureRate),
	}

	// MTTR (placeholder)
	metrics.MTTR = models.MTTR{
		MedianMinutes: 60,
		Rating:        "medium",
	}

	return metrics, nil
}

// migrationSQL contains the database schema
const migrationSQL = `
-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- Organizations table
CREATE TABLE IF NOT EXISTS organizations (
    id SERIAL PRIMARY KEY,
    github_id BIGINT UNIQUE NOT NULL,
    login VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    avatar_url TEXT,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Repositories table
CREATE TABLE IF NOT EXISTS repositories (
    id SERIAL PRIMARY KEY,
    github_id BIGINT UNIQUE NOT NULL,
    org_id INTEGER REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    description TEXT,
    default_branch VARCHAR(255) DEFAULT 'main',
    html_url TEXT,
    is_private BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_repositories_org_id ON repositories(org_id);
CREATE INDEX IF NOT EXISTS idx_repositories_full_name ON repositories(full_name);

-- Workflows table
CREATE TABLE IF NOT EXISTS workflows (
    id SERIAL PRIMARY KEY,
    github_id BIGINT UNIQUE NOT NULL,
    repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    path VARCHAR(255) NOT NULL,
    state VARCHAR(50) DEFAULT 'active',
    badge_url TEXT,
    html_url TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflows_repo_id ON workflows(repo_id);

-- Workflow runs table (TimescaleDB hypertable)
CREATE TABLE IF NOT EXISTS workflow_runs (
    id SERIAL,
    github_id BIGINT NOT NULL,
    workflow_id INTEGER REFERENCES workflows(id) ON DELETE CASCADE,
    repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    run_number INTEGER,
    name VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    conclusion VARCHAR(50),
    event VARCHAR(100),
    branch VARCHAR(255),
    commit_sha VARCHAR(40),
    commit_message TEXT,
    actor_login VARCHAR(255),
    actor_avatar TEXT,
    html_url TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_seconds INTEGER,
    is_deployment BOOLEAN DEFAULT false,
    environment VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, started_at)
);

-- Convert to hypertable if not already
SELECT create_hypertable('workflow_runs', 'started_at', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow_id ON workflow_runs(workflow_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_repo_id ON workflow_runs(repo_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_github_id ON workflow_runs(github_id);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status, started_at DESC);

-- Workflow jobs table (TimescaleDB hypertable)
CREATE TABLE IF NOT EXISTS workflow_jobs (
    id SERIAL,
    github_id BIGINT NOT NULL,
    run_id INTEGER NOT NULL,
    run_github_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    conclusion VARCHAR(50),
    runner_name VARCHAR(255),
    runner_group VARCHAR(255),
    labels JSONB DEFAULT '[]',
    steps JSONB DEFAULT '[]',
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_seconds INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, started_at)
);

-- Convert to hypertable if not already
SELECT create_hypertable('workflow_jobs', 'started_at', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_workflow_jobs_run_id ON workflow_jobs(run_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_jobs_github_id ON workflow_jobs(github_id);

-- Deployments table
CREATE TABLE IF NOT EXISTS deployments (
    id SERIAL PRIMARY KEY,
    github_id BIGINT UNIQUE NOT NULL,
    repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    run_id INTEGER,
    environment VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    description TEXT,
    creator_login VARCHAR(255),
    sha VARCHAR(40),
    ref VARCHAR(255),
    deployed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deployments_repo_id ON deployments(repo_id);
CREATE INDEX IF NOT EXISTS idx_deployments_environment ON deployments(environment);

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    github_id BIGINT UNIQUE NOT NULL,
    login VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    email VARCHAR(255),
    avatar_url TEXT,
    access_token TEXT,
    refresh_token TEXT,
    token_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(255) PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    data JSONB DEFAULT '{}',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- Continuous aggregate for daily metrics
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_workflow_metrics
WITH (timescaledb.continuous) AS
SELECT 
    workflow_id,
    repo_id,
    time_bucket('1 day', started_at) AS day,
    COUNT(*) AS total_runs,
    COUNT(*) FILTER (WHERE conclusion = 'success') AS successful_runs,
    COUNT(*) FILTER (WHERE conclusion = 'failure') AS failed_runs,
    COUNT(*) FILTER (WHERE conclusion = 'cancelled') AS cancelled_runs,
    AVG(duration_seconds) AS avg_duration,
    PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY duration_seconds) AS p50_duration,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) AS p95_duration,
    MIN(duration_seconds) AS min_duration,
    MAX(duration_seconds) AS max_duration
FROM workflow_runs
WHERE completed_at IS NOT NULL
GROUP BY workflow_id, repo_id, time_bucket('1 day', started_at)
WITH NO DATA;

-- Refresh policy for continuous aggregate
SELECT add_continuous_aggregate_policy('daily_workflow_metrics',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- Retention policy (keep data for 1 year)
SELECT add_retention_policy('workflow_runs', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('workflow_jobs', INTERVAL '1 year', if_not_exists => TRUE);
`

