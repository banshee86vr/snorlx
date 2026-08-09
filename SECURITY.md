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

- **Session CSRF**: Cookie-session mutating requests check the `Origin` header against `FRONTEND_URL`. Empty Origin is allowed for same-origin / CLI clients.
- **Personal API tokens**: Mint under Settings for MCP/automation. Tokens are stored as SHA-256 hashes and returned in plaintext only once. Validated `Authorization: Bearer snorlx_…` authenticates API clients and skips Origin CSRF checks; invalid Bearer tokens do not bypass CSRF or fall through to session auth. Prefer read-only scopes when write is not needed; revoke leaked tokens immediately.
- **MCP**: Use a user-minted API token in `SNORLX_API_TOKEN`. For Streamable HTTP, require `MCP_HTTP_TOKEN` on `/mcp` and keep `MCP_HOST=127.0.0.1` unless a reverse proxy provides auth. Never put OAuth client secrets or stored GitHub user tokens into MCP configuration. See [docs/mcp.md](docs/mcp.md).

## Resources

- [GitHub Security Policy](https://docs.github.com/en/code-security/security-policy)
- [Security Advisories](https://github.com/banshee86vr/snorlx/security/advisories)
