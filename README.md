# Snorlx - CI/CD Dashboard for GitHub Actions

A comprehensive, self-hosted dashboard that provides centralized visibility over GitHub Actions pipelines, performance metrics, and costs distributed across multiple repositories and organizations.

![Dashboard Preview](docs/preview.png)

## Features

- **Centralized Visibility**: Single pane of glass for all workflow runs across repositories
- **DevOps Metrics**: Deployment Frequency, Lead Time, Change Failure Rate, MTTR
- **Real-time Updates**: Live pipeline status via WebSocket
- **GitHub App Authentication**: Granular permissions, repository-level access
- **Cost Tracking**: Per-workflow and per-repository cost analysis
- **Multi-repository Support**: Monitor workflows across multiple repos and organizations
- **Beautiful UI**: Modern, responsive design with dark/light mode
- **Two Storage Modes**: Quick start with memory storage or persistent PostgreSQL database

## Architecture

```mermaid
graph TB
    subgraph Browser
        subgraph Frontend["Frontend &middot; React + TypeScript &middot; :5173 / :5174"]
            Pages["Pages<br/>Dashboard &middot; Workflows &middot; Runs<br/>Repositories &middot; Settings"]
            Contexts["Contexts<br/>Auth &middot; Theme &middot; Socket &middot; Sync &middot; Sidebar"]
            TQ["TanStack Query<br/>Cache + auto-invalidation on WS events"]
            Pages --> Contexts
            Contexts --> TQ
        end
    end

    subgraph Server["Backend &middot; Go + Chi &middot; :8080 / :3001"]
        MW["Middleware<br/>CORS &middot; Rate Limit &middot; Auth<br/>CSRF &middot; Security Headers"]
        REST["REST Handlers<br/>/api/auth &middot; /api/organizations<br/>/api/repositories &middot; /api/workflows<br/>/api/runs &middot; /api/jobs<br/>/api/metrics &middot; /api/dashboard"]
        WS["WebSocket Hub<br/>Client registry &middot; Broadcast<br/>workflow_run &middot; workflow_job<br/>deployment &middot; sync:*"]
        WH["Webhook Handler<br/>POST /api/webhooks/github<br/>Signature validation"]
        GH["GitHub Client<br/>OAuth flow &middot; REST API v3<br/>Repo + workflow sync"]
        ST["Storage Interface"]
        MEM["Memory Store<br/>dev"]
        DB["PostgreSQL Store<br/>production"]

        MW --> REST
        MW --> WS
        MW --> WH
        REST --> GH
        REST --> ST
        WH --> ST
        WH --> WS
        ST --> MEM
        ST --> DB
    end

    TQ -- "REST /api/*" --> MW
    Contexts -- "WebSocket /ws" --> WS
    GH -- "OAuth + API calls" --> GitHub["GitHub API<br/>OAuth &middot; Orgs &middot; Repos<br/>Workflows &middot; Runs &middot; Logs"]
    GitHub -- "Webhook events" --> WH
    DB --> PG["PostgreSQL + TimescaleDB<br/>Core entities &middot; Hypertables<br/>Continuous aggregates"]

    style Frontend fill:#1e293b,stroke:#3b82f6,color:#e2e8f0
    style Server fill:#1e293b,stroke:#10b981,color:#e2e8f0
    style GitHub fill:#1e293b,stroke:#f59e0b,color:#e2e8f0
    style PG fill:#1e293b,stroke:#8b5cf6,color:#e2e8f0
```

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
- GitHub App (see [Setup Guide](#github-app-setup))

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
# Development mode (skip GitHub App validation)
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
- `POST /api/repositories/sync` - Trigger repository sync

### Workflows

- `GET /api/workflows` - List all workflows
- `GET /api/workflows/:id` - Get workflow details
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

### DevOps Metrics

- `GET /api/metrics/devops` - Get DevOps metrics summary
- `GET /api/metrics/devops/deployment-frequency` - Deployment frequency
- `GET /api/metrics/devops/lead-time` - Lead time for changes
- `GET /api/metrics/devops/change-failure-rate` - Change failure rate
- `GET /api/metrics/devops/mttr` - Mean time to recovery

### Dashboard

- `GET /api/dashboard/summary` - Get dashboard summary
- `GET /api/dashboard/trends` - Get trend data

### Real-time

- `GET /ws` - WebSocket endpoint for real-time updates

### Webhooks

- `POST /api/webhooks/github` - GitHub webhook receiver

## DevOps Metrics

The dashboard calculates DevOps metrics based on GitHub Actions data:

| Metric                   | Calculation                    | Elite Benchmark |
| ------------------------ | ------------------------------ | --------------- |
| **Deployment Frequency** | Production deploys per period  | Multiple/day    |
| **Lead Time**            | Commit → Production deployment | < 1 hour        |
| **Change Failure Rate**  | Failed deploys / Total deploys | 0-15%           |
| **MTTR**                 | Incident → Resolution time     | < 1 hour        |

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
│   │   ├── storage/        # Storage layer (memory/database)
│   │   └── websocket/      # WebSocket hub for real-time updates
│   └── ...
├── helm/                   # Kubernetes Helm charts
└── docker-compose.yml      # Docker configuration
```

## Troubleshooting

### Common Issues

**Backend won't start: "GITHUB_APP_ID is required"**

- Set `DEV_MODE=true` in your `.env` file for development
- Or provide valid GitHub App credentials

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
