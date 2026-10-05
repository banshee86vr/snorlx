# Snorlx - CI/CD Dashboard for GitHub Actions

[![CI](https://github.com/banshee86vr/snorlx/actions/workflows/ci.yml/badge.svg?branch=main&event=push)](https://github.com/banshee86vr/snorlx/actions/workflows/ci.yml) [![Security Scans](https://github.com/banshee86vr/snorlx/actions/workflows/security.yml/badge.svg?branch=main&event=push)](https://github.com/banshee86vr/snorlx/actions/workflows/security.yml) [![Release](https://github.com/banshee86vr/snorlx/actions/workflows/release.yml/badge.svg?branch=main)](https://github.com/banshee86vr/snorlx/actions/workflows/release.yml) [![Latest release](https://img.shields.io/github/v/release/banshee86vr/snorlx?sort=semver)](https://github.com/banshee86vr/snorlx/releases/latest) [![Go](https://img.shields.io/github/go-mod/go-version/banshee86vr/snorlx?filename=backend%2Fgo.mod)](backend/go.mod) [![License: MIT](https://img.shields.io/github/license/banshee86vr/snorlx)](LICENSE)

A comprehensive, self-hosted dashboard that provides centralized visibility over GitHub Actions pipelines, performance metrics, and costs distributed across multiple repositories and organizations.

![Dashboard Preview](docs/preview.png)

## Features

- **Centralized Visibility**: Single pane of glass for all workflow runs across repositories
- **Repository Scoring**: Grade repos with gold/silver/bronze tiers across Security, Testing, CI/CD, Documentation, Code Quality, Maintenance, and Community
- **Real-time Updates**: Live pipeline status via WebSocket, fed by webhooks and a server-side poller that uses conditional GitHub requests
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

# 2. Configure (edit .env with your GitHub OAuth credentials and a SESSION_SECRET)
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
# Storage Mode - Use memory for quick start (no database needed)
STORAGE_MODE=memory

# GitHub OAuth (create at: https://github.com/settings/developers)
GITHUB_CLIENT_ID=your_oauth_client_id
GITHUB_CLIENT_SECRET=your_oauth_client_secret

# Encrypts stored GitHub tokens (generate with: openssl rand -base64 32, minimum 32 characters)
SESSION_SECRET=your_random_32_character_secret_string

# URLs (the Vite dev server proxies /api and /ws to the backend)
PORT=8080
FRONTEND_URL=http://localhost:5173
```

`DEV_MODE=true` only relaxes startup validation (placeholder secret, missing OAuth credentials); you still need real OAuth credentials to sign in.

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

For persistent data storage. In this mode the backend refuses to start when the database is unreachable instead of silently falling back to memory.

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
# Storage Mode - Use database for persistence
STORAGE_MODE=database
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/snorlx?sslmode=disable

# GitHub Configuration
GITHUB_CLIENT_ID=your_oauth_client_id
GITHUB_CLIENT_SECRET=your_oauth_client_secret

# Encrypts stored GitHub tokens (openssl rand -base64 32)
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

A production-like stack with all services containerized. The browser talks only to the frontend: nginx serves the SPA and proxies `/api` and `/ws` to the backend, so one origin, first-party cookies and a strict CSP.

1. **Configure environment**

```bash
cp env.example .env
# Set POSTGRES_PASSWORD, SESSION_SECRET, GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET.
# Register the OAuth callback as http://localhost:5174/api/auth/callback.
# Optional: ALLOWED_GITHUB_USERS / ALLOWED_GITHUB_ORGS to restrict who can sign in.
```

2. **Start all services**

```bash
docker compose up -d
```

The dashboard is available at http://localhost:5174. The backend is not published on the host; it is reachable only through the frontend proxy, and the database listens on `127.0.0.1:5433` for local `psql` access.

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

1. Go to [GitHub Developer Settings](https://github.com/settings/developers). On GitHub Enterprise Server, open `https://<your-ghe-host>/settings/developers` on that instance instead.
2. Click **OAuth Apps** → **New OAuth App**
3. Fill in the details:

   - **Application name**: `Snorlx Dashboard` (or your preferred name)
   - **Homepage URL**: the value of `FRONTEND_URL` (`http://localhost:5173` in development)
   - **Authorization callback URL**: `<FRONTEND_URL>/api/auth/callback` (`http://localhost:5173/api/auth/callback` in development; the frontend proxies `/api` to the backend)

4. Click **Register application**

### Step 2: Get Credentials

1. Copy the **Client ID**
2. Click **Generate a new client secret** and copy it

### Step 3: Configure Environment

Add to your `.env` file:

```bash
GITHUB_CLIENT_ID=your_client_id_here
GITHUB_CLIENT_SECRET=your_client_secret_here
# Only for GitHub Enterprise Server. Hostname, or the same host with /api/v3.
# GITHUB_BASE_URL=https://github.mycompany.com
```

### OAuth Scopes

The app requests these OAuth scopes:

- `read:user` - Read user profile information
- `user:email` - Access user email addresses
- `repo` - Access repositories (for workflow data)
- `read:org` - Read organization membership

### Keeping runs up to date

Runs update on their own while somebody has a page visible. The browser reports its visibility over the WebSocket, and the backend runs a live poller for users with a visible page: every `RUN_POLL_INTERVAL` (default `10s`) it re-reads each queued or in-progress run and its jobs, and every six intervals it lists the newest runs of every visible repository to pick up runs started since the last sync. Every request is conditional (`If-None-Match`), so a resource that did not change answers `304`, costs no GitHub rate-limit budget and writes nothing. Changes are pushed to the browser over the WebSocket; a page that becomes visible again triggers a pass right away and refetches what it missed. While the socket is down, pages poll the backend (never GitHub) until it reconnects, and those requests count as presence for 45 seconds (the same applies to API token clients such as MCP). Background tabs and closed browsers mean no GitHub traffic. The **Refresh** buttons stay as an escape hatch: they run the same conditional passes right away. See [ADR 0007](docs/adr/0007-server-side-live-run-polling.md).

### Optional: Webhooks

Webhooks deliver changes the instant GitHub reports them and let the poller answer `304` for everything it checks. Configure one in your repository or organization settings:

1. Go to Repository → Settings → Webhooks → Add webhook
2. **Payload URL**: `https://your-domain.com/api/webhooks/github`
3. **Content type**: `application/json`
4. **Secret**: Generate a secure secret and add to `.env` as `GITHUB_WEBHOOK_SECRET`
5. Select events: `Workflow runs`, `Workflow jobs`, `Deployments`

With webhooks on every repository you can lengthen `RUN_POLL_INTERVAL` or set it to `0` to rely on webhooks and manual refresh only.

## Configuration

### Environment Variables

#### Storage Configuration

| Variable            | Required                | Default  | Description                                    |
| ------------------- | ----------------------- | -------- | ---------------------------------------------- |
| `STORAGE_MODE`      | No                      | `memory` | `memory` or `database`                         |
| `DATABASE_URL`      | Only if `database` mode | -        | PostgreSQL connection string                   |
| `POSTGRES_PASSWORD` | Only if `database` mode | -        | PostgreSQL password (used by Docker Compose)   |

#### Server Configuration

| Variable              | Description                                                                                                           | Default                               |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| `PORT`                | Server port                                                                                                           | `8080`                                |
| `LOG_LEVEL`           | Logging level (debug, info, warn, error)                                                                              | `info`                                |
| `LOG_FORMAT`          | `json` for structured logs, anything else for console output                                                          | console                               |
| `SESSION_SECRET`      | Derives the key that encrypts stored GitHub tokens. Required outside `DEV_MODE`, minimum 32 characters                | Required                              |
| `FRONTEND_URL`        | Public origin (`scheme://host`) used for CORS, CSRF, WebSocket origin checks and the OAuth redirect. Plain `http` is accepted only for localhost outside `DEV_MODE` | `http://localhost:5173`               |
| `COOKIE_SECURE`       | Force the `Secure` flag on auth cookies                                                                               | `true` when `FRONTEND_URL` is https   |
| `TRUSTED_PROXY_CIDRS` | Comma-separated networks whose `X-Forwarded-For` / `X-Real-IP` are trusted (ingress, reverse proxy)                   | none (peer address is used)           |
| `VITE_API_URL`        | Backend API URL baked into the frontend build. Leave empty: the frontend proxies `/api` and `/ws` same-origin         | empty                                 |

#### GitHub OAuth Configuration

| Variable                | Description                                                                              | Required                    |
| ----------------------- | ---------------------------------------------------------------------------------------- | --------------------------- |
| `GITHUB_CLIENT_ID`      | GitHub OAuth App Client ID                                                               | Yes                         |
| `GITHUB_CLIENT_SECRET`  | GitHub OAuth App Client Secret                                                           | Yes                         |
| `GITHUB_BASE_URL`       | Enterprise Server URL (`https://host` or `.../api/v3`)                                   | No (defaults to github.com) |
| `GITHUB_WEBHOOK_SECRET` | Webhook signature secret; unsigned deliveries are rejected                               | No (for webhooks only)      |
| `ALLOWED_GITHUB_USERS`  | Comma-separated GitHub logins allowed to sign in                                         | No (recommended)            |
| `ALLOWED_GITHUB_ORGS`   | Comma-separated GitHub organizations whose members may sign in                           | No (recommended)            |
| `DEV_MODE`              | Relax startup validation (placeholder secret, missing OAuth credentials). Local dev only | No                          |

#### Sync Configuration

| Variable            | Required | Default     | Description                                                                                                   |
| ------------------- | -------- | ----------- | ------------------------------------------------------------------------------------------------------------- |
| `SYNC_LIMIT`        | No       | `0` (all)   | Limit number of repos to sync                                                                                 |
| `SYNC_REPOS`        | No       | -           | Comma-separated list of specific repos to sync                                                                |
| `RUN_POLL_INTERVAL` | No       | `10s`       | How often active runs are refreshed from GitHub for watching users (conditional requests). Minimum `5s`, `0` disables the poller |

## API Endpoints

### Health Check

- `GET /health` - JSON `{"status":"ok","version":"<release>"}`. Liveness: process only.
- `GET /health/ready` - `200 {"status":"ready"}` or `503 {"status":"unavailable"}`. Readiness: also pings the storage backend.

All `/api` routes below (except `/api/auth/*` and the webhook receiver) require a session cookie or a personal API token, and every object is scoped to the repositories the caller synced. See [SECURITY.md](SECURITY.md) for the access model.

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

- `GET /ws` - WebSocket endpoint for real-time updates. Server events: `workflow_run` (a stored run changed or appeared), `workflow_job` (`{run_id, run_github_id}`: the jobs of that run changed), `deployment`, `sync:*`. The client sends `presence` (`{"active": bool}`) with its page visibility; the live poller works only for users with a visible page

### Webhooks

- `POST /api/webhooks/github` - GitHub webhook receiver

## Project Structure

```
├── .github/workflows/      # CI and Security GitHub Actions
├── frontend/               # React frontend
│   ├── nginx/              # nginx template (SPA + same-origin /api and /ws proxy)
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
│   │   ├── config/         # Configuration and validation
│   │   ├── github/         # GitHub client
│   │   ├── handlers/       # HTTP handlers (authn, per-user authz)
│   │   ├── httpmiddleware/ # Trusted-proxy client IP, request logging
│   │   ├── models/         # Data models
│   │   ├── scorer/         # Repository scoring (gold/silver/bronze)
│   │   ├── storage/        # Storage layer (memory/database) and migrations
│   │   ├── tokencrypt/     # Encryption of stored GitHub tokens
│   │   └── websocket/      # WebSocket hub for per-user real-time updates
│   └── ...
├── docs/adr/               # Architecture decision records
├── helm/                   # Kubernetes Helm charts
└── docker-compose.yml      # Docker configuration
```

## Troubleshooting

### Common Issues

**Backend won't start: GitHub OAuth credentials required**

- Provide `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` from a [GitHub OAuth App](#github-oauth-app-setup)
- `DEV_MODE=true` lets the process start without them for local work on non-auth code, but nobody can sign in

**Backend won't start: SESSION_SECRET or FRONTEND_URL rejected**

- `SESSION_SECRET` must be a random value of at least 32 characters (`openssl rand -base64 32`)
- `FRONTEND_URL` must be an origin such as `https://dash.example.com`; plain `http` is accepted only for localhost unless `DEV_MODE=true`

**Database connection errors**

- Verify PostgreSQL is running: `psql -U postgres -d snorlx`
- Check `DATABASE_URL` in `.env` matches your database credentials
- Ensure TimescaleDB extension is installed
- In database mode the backend exits on a connection failure by design; `/health/ready` returns 503 when the database goes away later

**Signed in but the dashboard is empty**

- Each user sees only the repositories they synced with their own token. Click **Sync** once after signing in (and once after upgrading from a release before 1.1)

**403 on every action or WebSocket disconnected**

- The browser origin must equal `FRONTEND_URL`. Serve the SPA and the API from that origin (the frontend image proxies `/api` and `/ws`); do not point the browser at the backend port directly

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

The project includes three GitHub Actions workflows:

### CI (`ci.yml`)

Runs on push and pull requests to `main`:

- **Backend**: Go vet, tests with race detector and coverage, binary build
- **Frontend**: Lint, tests with coverage, production build
- **Helm**: `helm lint` and `helm template`, including the chart `appVersion` image tag fallback

### Release (`release.yml`)

Runs on push to `main`. release-please opens or updates the release pull request, and publishes the release when that pull request merges. On publish it verifies the immutable release, builds the backend and frontend images, and pushes the Helm chart. See [Releases](#releases).

### Security (`security.yml`)

Runs on push, pull requests to `main`, and weekly (Monday 08:00 UTC):

- **CodeQL Analysis**: Static analysis for Go and JavaScript/TypeScript
- **Trivy Scans**: Filesystem vulnerability scan plus Docker image scans for both backend and frontend; fixable CRITICAL and HIGH findings fail the job
- **Dependency Review**: Flags newly introduced vulnerable dependencies on PRs
- **Go Security**: govulncheck and gosec (pinned versions) for Go-specific vulnerabilities
- **npm Audit**: Checks for known vulnerabilities in Node packages

All workflows pin actions to full commit SHAs and install dependencies with `--frozen-lockfile`; Dependabot keeps the pins, the image digests and the lockfile current. See [ADR 0006](docs/adr/0006-supply-chain-pinning-and-signing.md).

## 12-Factor App Compliance

This application follows the [12-Factor methodology](https://12factor.net/):

1. **Codebase**: Single repo tracked in Git
2. **Dependencies**: Explicitly declared in `go.mod` and `package.json`
3. **Config**: Environment variables for all configuration
4. **Backing services**: Database as attached resource via URL
5. **Build, release, run**: Docker images tagged with the product version
6. **Processes**: Stateless; sessions in storage
7. **Port binding**: Self-contained HTTP server
8. **Concurrency**: Horizontal scaling via replicas
9. **Disposability**: Graceful shutdown on SIGTERM
10. **Dev/prod parity**: Docker Compose mirrors production
11. **Logs**: JSON to stdout, collected by platform
12. **Admin processes**: Migrations as part of startup

## Releases

Releases are cut by [release-please](https://github.com/googleapis/release-please) from Conventional Commits on `main`. It opens a release pull request that bumps the product version in the root, frontend, and MCP `package.json` files, `backend/internal/version/version.go`, `mcp/src/server.ts`, and `helm/snorlx/Chart.yaml`, and writes `CHANGELOG.md`. Merging that pull request publishes tag `vX.Y.Z` and one GitHub Release.

The repository uses GitHub immutable releases. After publication the tag cannot be moved, deleted, or reused, and GitHub attaches a release attestation. Check it with `gh release verify vX.Y.Z`. A failed verify, image, or chart job can be re-run from Actions. A new run publishes the `vX.Y.Z` tag already on that commit, or the tag you pass to workflow_dispatch. A wrong release needs a new patch version.

Images are published only for that version, for `linux/amd64` and `linux/arm64`:

```bash
docker pull ghcr.io/banshee86vr/snorlx-backend:X.Y.Z
docker pull ghcr.io/banshee86vr/snorlx-frontend:X.Y.Z
```

The Helm chart is an OCI artifact. An empty image tag in the chart uses `appVersion`, which matches the image tag. The chart needs the public dashboard origin: enable the ingress with TLS (the origin becomes `https://<first host>`) or set `backend.frontendUrl` explicitly, for example when TLS terminates in front of the ingress or for port-forward access. Generated secrets (database password, `SESSION_SECRET`) are created on first install and kept across upgrades.

```bash
helm install snorlx oci://ghcr.io/banshee86vr/charts/snorlx --version X.Y.Z \
  --set ingress.enabled=true \
  --set 'ingress.hosts[0].host=snorlx.example.com' \
  --set 'ingress.tls[0].secretName=snorlx-tls' \
  --set 'ingress.tls[0].hosts[0]=snorlx.example.com' \
  --set github.clientId=... --set github.clientSecret=... \
  --set github.allowedOrgs=my-org
```

Images and the chart are signed with keyless cosign and carry SLSA provenance and an SBOM:

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/banshee86vr/snorlx/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/banshee86vr/snorlx-backend:X.Y.Z
```

release-please authenticates with a GitHub App. Create the app (Contents, Pull requests, and Issues write), install it on this repository, and store `RELEASE_PLEASE_APP_ID` and `RELEASE_PLEASE_APP_PRIVATE_KEY` as secrets. Enable immutable releases under Settings, General, Releases before the first release.

Any Conventional Commit, including `chore(deps)`, opens or updates a patch release pull request. Merge it when you want to ship.

## MCP

Snorlx exposes a Model Context Protocol server for agents (Cursor, etc.). Create a personal API token under **Settings → API tokens**, then see [docs/mcp.md](docs/mcp.md) for stdio and Streamable HTTP setup.

## License

MIT License - see [LICENSE](LICENSE) for details.
