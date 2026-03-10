# Snorlx - CI/CD Dashboard for GitHub Actions

[![CI](https://github.com/banshee86vr/snorlx/actions/workflows/ci.yml/badge.svg)](https://github.com/banshee86vr/snorlx/actions/workflows/ci.yml) [![Security Scans](https://github.com/banshee86vr/snorlx/actions/workflows/security.yml/badge.svg)](https://github.com/banshee86vr/snorlx/actions/workflows/security.yml)

A comprehensive, self-hosted dashboard that provides centralized visibility over GitHub Actions pipelines, performance metrics, and costs distributed across multiple repositories and organizations.

![Dashboard Preview](docs/preview.png)

## Features

- **Centralized Visibility**: Single pane of glass for all workflow runs across repositories
- **Repository Scoring**: Grade repos with gold/silver/bronze tiers across Security, Testing, CI/CD, Documentation, Code Quality, Maintenance, and Community
- **Real-time Updates**: Live pipeline status via WebSocket
- **GitHub OAuth**: Sign in with GitHub; no GitHub App setup required
- **Cost Tracking**: Per-workflow and per-repository cost analysis
- **Multi-repository Support**: Monitor workflows across multiple repos and organizations
- **Beautiful UI**: Modern, responsive design with dark/light mode
- **Two Storage Modes**: Quick start with memory storage or persistent PostgreSQL database

## Architecture

![Architecture](docs/architecture.jpg)

## Quick Start

### ⚡ TL;DR - Get Running in 5 Minutes

**No Database (Memory Mode):**

```bash
# 1. Clone and install
git clone https://github.com/banshee86vr/snorlx.git && cd snorlx && pnpm install

# 2. Configure (edit .env with your GitHub credentials)
cp env.example .env

# 3. Set STORAGE_MODE=memory in .env (default)

# 4. Start both frontend and backend
pnpm run dev
```

**With PostgreSQL Database:**

```bash
# 1. Clone and install
git clone https://github.com/banshee86vr/snorlx.git && cd snorlx && pnpm install

# 2. Start PostgreSQL (Docker)
docker run --name snorlx-postgres \
  -e POSTGRES_DB=snorlx \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 -d timescale/timescaledb:latest-pg16

# 3. Configure (edit .env with your credentials and DATABASE_URL)
cp env.example .env

# 4. Set STORAGE_MODE=database and DATABASE_URL in .env

# 5. Start both frontend and backend
pnpm run dev
```

Access at: http://localhost:5173

### Prerequisites

- Go 1.26+
- Node.js 20+
- pnpm or npm
- PostgreSQL 14+ with TimescaleDB (optional - only for database mode)
- GitHub OAuth App (see [Setup Guide](#github-oauth-app-setup))

## 🚀 Local Development (No Docker Required)

### Option 1: Memory Mode (No Database)

**Perfect for testing and development!** Start in minutes without setting up a database.

1. **Clone and install dependencies**

```bash
git clone https://github.com/banshee86vr/snorlx.git
cd snorlx
pnpm install
```

2. **Configure environment**

```bash
# Copy example environment file
cp env.example .env
```

Edit `.env` with your credentials:

```env
# Development mode (skip GitHub OAuth validation)
DEV_MODE=true

# Storage Mode - Use memory for quick start (no database needed)
STORAGE_MODE=memory

# GitHub OAuth (create at: https://github.com/settings/developers)
GITHUB_CLIENT_ID=your_oauth_client_id
GITHUB_CLIENT_SECRET=your_oauth_client_secret

# Session Security (generate with: openssl rand -base64 32)
SESSION_SECRET=your_random_32_character_secret_string

# URLs
PORT=8080
FRONTEND_URL=http://localhost:5173
```

3. **Start the application**

```bash
# From project root - starts both frontend and backend concurrently
pnpm run dev
```

The application will start:

- Frontend: http://localhost:5173
- Backend API: http://localhost:8080

**Or start separately:**

```bash
# Backend only
pnpm run dev:backend

# Frontend only (in another terminal)
pnpm run dev:frontend
```

### Option 2: Database Mode (Persistent Storage)

For persistent data storage in production environments.

#### Step 1: Install Dependencies

```bash
git clone https://github.com/banshee86vr/snorlx.git
cd snorlx
pnpm install
```

#### Step 2: Setup PostgreSQL + TimescaleDB

**Using Docker (Recommended)**

```bash
docker run --name snorlx-postgres \
  -e POSTGRES_DB=snorlx \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  -d timescale/timescaledb:latest-pg16
```

**Using Local PostgreSQL (macOS with Homebrew)**

```bash
# Install TimescaleDB
brew install postgresql@16 timescaledb

# Start PostgreSQL
brew services start postgresql@16

# Create database
psql postgres -c "CREATE DATABASE snorlx;"
```

#### Step 3: Configure Environment

```bash
cp env.example .env
```

Edit `.env`:

```env
# Development mode
DEV_MODE=true

# Storage Mode - Use database for persistence
STORAGE_MODE=database
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/snorlx?sslmode=disable

# GitHub Configuration
GITHUB_CLIENT_ID=your_oauth_client_id
GITHUB_CLIENT_SECRET=your_oauth_client_secret

# Session Security
SESSION_SECRET=your_random_32_character_secret_string

# URLs
PORT=8080
FRONTEND_URL=http://localhost:5173
```

#### Step 4: Start the Application

```bash
# From project root - starts both frontend and backend concurrently
# Migrations run automatically on backend startup
pnpm run dev
```

Access the dashboard at http://localhost:5173

## 🐳 Docker Compose Setup

For production deployments with all services containerized.

1. **Configure environment**

```bash
cp env.example .env
# Edit .env with your production settings
# Make sure STORAGE_MODE=database for Docker
```

2. **Start all services**

```bash
docker compose up -d
```

The dashboard will be available at http://localhost:5174 (frontend) and http://localhost:3001 (backend API)

3. **View logs**

```bash
docker compose logs -f
```

4. **Stop services**

```bash
docker compose down
```

## Repository Scoring

Repositories are graded with an overall percentage and a tier (gold / silver / bronze) based on checks in seven categories:

| Category       | Weight | Examples |
| -------------- | ------ | -------- |
| Security       | 25%    | Branch protection, Dependabot, code scanning |
| Testing        | 20%    | Test configs, coverage, CI test jobs |
| CI/CD          | 15%    | Workflows, deployment, status checks |
| Documentation | 15%    | README, CONTRIBUTING, issue templates |
| Code Quality   | 10%    | Linters, code owners |
| Maintenance    | 10%    | Recent activity, description |
| Community      | 5%     | Community health files |

Sync a repository and use **Refresh grade** on its detail page to compute or update its score. The dashboard summary shows average score across repos.

## GitHub OAuth App Setup

This project uses GitHub OAuth App for user authentication (simpler than GitHub Apps).

### Step 1: Create OAuth App

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Click **OAuth Apps** → **New OAuth App**
3. Fill in the details:

   - **Application name**: `Snorlx Dashboard` (or your preferred name)
   - **Homepage URL**: `http://localhost:5173` (or your production URL)
   - **Authorization callback URL**: `http://localhost:8080/api/auth/callback`

4. Click **Register application**

### Step 2: Get Credentials

1. Copy the **Client ID**
2. Click **Generate a new client secret** and copy it

### Step 3: Configure Environment

Add to your `.env` file:

```bash
GITHUB_CLIENT_ID=your_client_id_here
GITHUB_CLIENT_SECRET=your_client_secret_here
```

### OAuth Scopes

The app requests these OAuth scopes:

- `read:user` - Read user profile information
- `user:email` - Access user email addresses
- `repo` - Access repositories (for workflow data)
- `read:org` - Read organization membership

### Optional: Webhooks

For real-time updates via webhooks, configure a webhook in your repository/organization settings:

1. Go to Repository → Settings → Webhooks → Add webhook
2. **Payload URL**: `https://your-domain.com/api/webhooks/github`
3. **Content type**: `application/json`
4. **Secret**: Generate a secure secret and add to `.env` as `GITHUB_WEBHOOK_SECRET`
5. Select events: `Workflow runs`, `Workflow jobs`, `Deployments`

## Configuration

### Environment Variables

#### Storage Configuration

| Variable            | Required                | Default  | Description                                    |
| ------------------- | ----------------------- | -------- | ---------------------------------------------- |
| `STORAGE_MODE`      | No                      | `memory` | `memory` or `database`                         |
| `DATABASE_URL`      | Only if `database` mode | -        | PostgreSQL connection string                   |
| `POSTGRES_PASSWORD` | Only if `database` mode | -        | PostgreSQL password (used by Docker Compose)   |

#### Server Configuration

| Variable         | Description                              | Default                 |
| ---------------- | ---------------------------------------- | ----------------------- |
| `PORT`           | Server port                              | `8080`                  |
| `LOG_LEVEL`      | Logging level (debug, info, warn, error) | `info`                  |
| `SESSION_SECRET` | Session encryption key                   | Required                |
| `FRONTEND_URL`   | Frontend URL for CORS                    | `http://localhost:5173` |
| `VITE_API_URL`   | Backend API URL for the frontend         | `http://localhost:8080` |

#### GitHub OAuth Configuration

| Variable                | Description                    | Required               |
| ----------------------- | ------------------------------ | ---------------------- |
| `GITHUB_CLIENT_ID`      | GitHub OAuth App Client ID     | Yes                    |
| `GITHUB_CLIENT_SECRET`  | GitHub OAuth App Client Secret | Yes                    |
| `GITHUB_WEBHOOK_SECRET` | Webhook signature secret       | No (for webhooks only) |
| `DEV_MODE`              | Skip GitHub OAuth validation   | No                     |

#### Sync Configuration

| Variable     | Required | Default     | Description                                  |
| ------------ | -------- | ----------- | -------------------------------------------- |
| `SYNC_LIMIT` | No       | `0` (all)   | Limit number of repos to sync                |
| `SYNC_REPOS` | No       | -           | Comma-separated list of specific repos to sync |

## API Endpoints

### Health Check

- `GET /health` - Health check endpoint

### Authentication

- `GET /api/auth/login` - Initiate GitHub OAuth
- `GET /api/auth/callback` - OAuth callback
- `POST /api/auth/logout` - Logout
- `GET /api/auth/status` - Check auth status

### Organizations

- `GET /api/organizations` - List all organizations
- `GET /api/organizations/:id` - Get organization details

### Repositories

- `GET /api/repositories` - List all repositories
- `GET /api/repositories/:id` - Get repository details
- `GET /api/repositories/scores` - List latest repository scores (all repos)
- `GET /api/repositories/:id/score` - Get latest score for a repository
- `POST /api/repositories/sync` - Trigger repository sync

### Workflows

- `GET /api/workflows` - List all workflows
- `GET /api/workflows/:id` - Get workflow details
- `PATCH /api/workflows/:id` - Update workflow
- `GET /api/workflows/:id/runs` - Get workflow runs

### Runs

- `GET /api/runs` - List all runs (with filters)
- `GET /api/runs/:id` - Get run details
- `GET /api/runs/:id/jobs` - Get run jobs
- `GET /api/runs/:id/logs` - Get run logs
- `GET /api/runs/:id/annotations` - Get run annotations
- `GET /api/runs/:id/workflow-definition` - Get workflow YAML definition
- `POST /api/runs/:id/rerun` - Rerun a workflow
- `POST /api/runs/:id/cancel` - Cancel a running workflow

### Jobs

- `GET /api/jobs/:id/logs` - Get job logs

### Dashboard

- `GET /api/dashboard/summary` - Get dashboard summary
- `GET /api/dashboard/trends` - Get trend data

### Real-time

- `GET /ws` - WebSocket endpoint for real-time updates

### Webhooks

- `POST /api/webhooks/github` - GitHub webhook receiver

## Project Structure

```
├── .github/workflows/      # CI and Security GitHub Actions
├── frontend/               # React frontend
│   ├── src/
│   │   ├── components/     # UI components (layout, protected routes)
│   │   ├── context/        # React contexts (auth, theme, socket, sync, sidebar)
│   │   ├── lib/            # Utility functions
│   │   ├── pages/          # Page components
│   │   ├── services/       # API client
│   │   ├── styles/         # Global styles
│   │   ├── test/           # Test setup
│   │   └── types/          # TypeScript types
│   └── ...
├── backend/                # Go backend
│   ├── cmd/server/         # Main entry point
│   ├── internal/
│   │   ├── config/         # Configuration
│   │   ├── database/       # Database migrations and setup
│   │   ├── github/         # GitHub client
│   │   ├── handlers/       # HTTP handlers
│   │   ├── models/         # Data models
│   │   ├── scorer/         # Repository scoring (gold/silver/bronze)
│   │   ├── storage/        # Storage layer (memory/database)
│   │   └── websocket/      # WebSocket hub for real-time updates
│   └── ...
├── helm/                   # Kubernetes Helm charts
└── docker-compose.yml      # Docker configuration
```

## Troubleshooting

### Common Issues

**Backend won't start: GitHub OAuth credentials required**

- Set `DEV_MODE=true` in your `.env` file for local development (skips OAuth)
- Or provide `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` from a [GitHub OAuth App](#github-oauth-app-setup)

**Database connection errors**

- Verify PostgreSQL is running: `psql -U postgres -d snorlx`
- Check `DATABASE_URL` in `.env` matches your database credentials
- Ensure TimescaleDB extension is installed

**Port already in use (EADDRINUSE)**

- Backend (8080) or Frontend (5173) port is already taken
- Find process: `lsof -i :8080` or `lsof -i :5173`
- Kill process: `kill -9 <PID>`
- Or change port in `.env`

**Memory mode data is lost**

- This is expected! Memory mode doesn't persist data between restarts
- Use `STORAGE_MODE=database` for persistent storage

**"Too many requests" error**

- Rate limiting is active. Wait and retry
- In production, this prevents abuse

## CI/CD

The project includes two GitHub Actions workflows:

### CI (`ci.yml`)

Runs on push and pull requests to `main`:

- **Backend**: Go vet, tests with race detector and coverage, binary build
- **Frontend**: Lint, tests with coverage, production build

### Security (`security.yml`)

Runs on push, pull requests to `main`, and weekly (Monday 08:00 UTC):

- **CodeQL Analysis**: Static analysis for Go and JavaScript/TypeScript
- **Trivy Scans**: Filesystem vulnerability scan plus Docker image scans for both backend and frontend
- **Dependency Review**: Flags newly introduced vulnerable dependencies on PRs
- **Go Security**: govulncheck and gosec for Go-specific vulnerabilities
- **npm Audit**: Checks for known vulnerabilities in Node packages

## 12-Factor App Compliance

This application follows the [12-Factor methodology](https://12factor.net/):

1. **Codebase**: Single repo tracked in Git
2. **Dependencies**: Explicitly declared in `go.mod` and `package.json`
3. **Config**: Environment variables for all configuration
4. **Backing services**: Database as attached resource via URL
5. **Build, release, run**: Docker images with semantic versioning
6. **Processes**: Stateless; sessions in storage
7. **Port binding**: Self-contained HTTP server
8. **Concurrency**: Horizontal scaling via replicas
9. **Disposability**: Graceful shutdown on SIGTERM
10. **Dev/prod parity**: Docker Compose mirrors production
11. **Logs**: JSON to stdout, collected by platform
12. **Admin processes**: Migrations as part of startup

## License

MIT License - see [LICENSE](LICENSE) for details.
