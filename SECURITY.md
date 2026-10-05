# Security Policy

## Supported Versions

We release security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| 1.x     | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly:

1. **Preferred:** Open a [private security advisory](https://github.com/banshee86vr/snorlx/security/advisories/new) on GitHub. This allows us to discuss and fix the issue before it is disclosed.
2. **Alternative:** Contact the maintainers (see repository owners) with a description of the issue and steps to reproduce.

Please do not open public issues for security vulnerabilities.

We will acknowledge your report and work on a fix. We appreciate your help in keeping this project secure.

## Operator notes

- **Who can sign in**: any GitHub account by default. Set `ALLOWED_GITHUB_USERS` and/or `ALLOWED_GITHUB_ORGS` in production so only your people can create a session.
- **What a user sees**: only repositories they synced with their own GitHub token (recorded in `user_repositories`). Lists, dashboards, trends, scores and WebSocket events are filtered per user; a run or workflow of another user's repository answers `404`.
- **GitHub tokens at rest**: encrypted with AES-256-GCM using a key derived from `SESSION_SECRET` (see [ADR 0004](docs/adr/0004-encrypt-github-tokens-at-rest.md)). Keep the secret at least 32 random characters and treat a rotation as a forced re-login for every user.
- **Background use of tokens**: the live poller (see [ADR 0007](docs/adr/0007-server-side-live-run-polling.md)) reads GitHub with the stored token of a user who has a visible dashboard page (or an API token client active in the last 45 seconds) and has access to the repository, only for repositories that user synced. It runs only while somebody is looking, uses conditional requests, and rests a token that GitHub rejects or rate limits until the next pass. Set `RUN_POLL_INTERVAL=0` to disable it.
- **Public origin**: `FRONTEND_URL` is the only origin accepted for CORS, CSRF (`Origin` header on mutating cookie-session requests) and WebSocket upgrades. Serve the SPA and the API from that one origin (the frontend image proxies `/api` and `/ws`). Outside `DEV_MODE` plain `http` is accepted only for localhost.
- **Cookies**: `HttpOnly`, `SameSite=Lax`, `Secure` when `FRONTEND_URL` is https (override with `COOKIE_SECURE`), and the `__Host-session` name when secure.
- **Proxy headers**: `X-Forwarded-For` and `X-Real-IP` are honoured only when the peer is inside `TRUSTED_PROXY_CIDRS`; otherwise the direct peer address is used for rate limits and logs.
- **Readiness**: `/health` is process liveness, `/health/ready` also pings the storage backend. In database mode the backend refuses to start when the database is unreachable instead of falling back to memory.
- **Personal API tokens**: mint under Settings for MCP/automation. Tokens are stored as SHA-256 hashes and returned in plaintext only once. Validated `Authorization: Bearer snorlx_…` authenticates API clients and skips the Origin CSRF check; invalid Bearer tokens do not bypass CSRF or fall through to session auth. Prefer read-only scopes when write is not needed; revoke leaked tokens immediately.
- **Webhooks**: deliveries must be signed with `GITHUB_WEBHOOK_SECRET`; events for repositories nobody synced are ignored.
- **MCP**: use a user-minted API token in `SNORLX_API_TOKEN` (read-only unless write tools are needed). For Streamable HTTP, require `MCP_HTTP_TOKEN` on `/mcp`, keep `MCP_HOST=127.0.0.1` unless a reverse proxy provides auth, and set `MCP_ALLOWED_HOSTS` when binding elsewhere. Never put OAuth client secrets or stored GitHub user tokens into MCP configuration. See [docs/mcp.md](docs/mcp.md).
- **Release artifacts**: images and the Helm chart are signed with keyless cosign and ship SLSA provenance and an SBOM. Verify with `cosign verify --certificate-identity-regexp 'https://github.com/banshee86vr/snorlx/' --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/banshee86vr/snorlx-backend:<version>`.

## Resources

- [GitHub Security Policy](https://docs.github.com/en/code-security/security-policy)
- [Security Advisories](https://github.com/banshee86vr/snorlx/security/advisories)
