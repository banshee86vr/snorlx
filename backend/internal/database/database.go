package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// DB wraps the database connection pool
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new database connection
func New(databaseURL string) (*DB, error) {
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

	log.Info().Msg("Database connection established")

	return &DB{Pool: pool}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	db.Pool.Close()
}

// Migrate runs database migrations
func (db *DB) Migrate() error {
	ctx := context.Background()

	// Create tables
	_, err := db.Pool.Exec(ctx, migrationSQL)
	if err != nil {
		return err
	}

	log.Info().Msg("Database migrations completed")
	return nil
}

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
-- Note: Foreign keys to other hypertables are not supported in TimescaleDB
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

-- Users table (for session management)
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

