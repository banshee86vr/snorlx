# 0001: Failed pipelines views

- Status: Accepted
- Date: 2026-09-01

## Context

Operators need a single place to find red GitHub Actions work across every repository Snorlx has synced. GitHub's REST API lists workflow runs per repository only, so a live org-wide scan would mean iterating every repo on each visit and would hit rate limits. Snorlx already stores runs from sync, webhooks, and the Active Pipelines refresh path. `GET /api/runs?conclusion=failure` exists, but it is a historical dump of every failed run, not a fleet health view.

## Decision

Snorlx exposes `GET /api/pipelines/failed` with two views over local storage:

- `current` (default): one row per active workflow whose latest completed run (any branch) has `conclusion=failure`. A newer in-progress or queued run does not hide the row. The workflow leaves the list only after a later completed success.
- `recent`: every stored run with `conclusion=failure` in the last 7 days, including workflows that later succeeded. Paginated.

An optional `refresh=true` reuses the existing `pullLatestRunsFromGitHub` helper. Search (`q`) matches repository `full_name` and workflow name in storage.

The Failures page, Dashboard teaser, and MCP `list_failed_pipelines` tool all use this API. Scope is all synced repositories. Cancelled, timed_out, and startup_failure runs are out of scope.

## Alternatives considered

- Filter the existing Runs page: already possible, but it cannot express "latest completed per workflow" and still feels like iterating a dump.
- Live GitHub scan on every visit: most complete if sync is stale, but rate-limit heavy and duplicates the Active Pipelines refresh path.
- Default-branch-only currently-broken: quieter, but hides a failing PR or release branch when that is the latest completed run.
- Org picker in v1: organization tables exist but sync does not populate them. All synced repos covers the stated pain without that wiring.

## Consequences

Fleet triage no longer requires opening each repository. Completeness is bounded by what is synced (about 50 runs per repo) and by the existing GitHub refresh cap (first 50 repos, 30 latest runs, 25s). Org-scoped filtering remains future work.
