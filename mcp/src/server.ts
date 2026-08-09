import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';

import { SnorlxApiClient } from './client.js';

function textResult(data: unknown) {
  return {
    content: [{ type: 'text' as const, text: typeof data === 'string' ? data : JSON.stringify(data, null, 2) }],
  };
}

function errorResult(err: unknown) {
  const message = err instanceof Error ? err.message : String(err);
  return {
    content: [{ type: 'text' as const, text: message }],
    isError: true,
  };
}

export function createSnorlxMcpServer(client: SnorlxApiClient): McpServer {
  const server = new McpServer({
    name: 'snorlx',
    version: '1.0.0',
  });

  server.tool('health', 'Check Snorlx backend health.', {}, async () => {
    try {
      return textResult(await client.request('GET', '/health'));
    } catch (err) {
      return errorResult(err);
    }
  });

  server.tool('dashboard_summary', 'Fleet CI snapshot: repos, workflows, recent/failed runs.', {}, async () => {
    try {
      return textResult(await client.request('GET', '/api/dashboard/summary'));
    } catch (err) {
      return errorResult(err);
    }
  });

  server.tool(
    'dashboard_trends',
    'Historical CI trends.',
    { days: z.number().int().min(1).max(365).optional().describe('Number of days (default 30)') },
    async ({ days }) => {
      try {
        return textResult(await client.request('GET', '/api/dashboard/trends', { query: { days: days ?? 30 } }));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool('list_active_pipelines', 'List in-progress and queued workflow runs.', {}, async () => {
    try {
      return textResult(await client.request('GET', '/api/pipelines/active'));
    } catch (err) {
      return errorResult(err);
    }
  });

  server.tool('list_organizations', 'List organizations known to Snorlx.', {}, async () => {
    try {
      return textResult(await client.request('GET', '/api/organizations'));
    } catch (err) {
      return errorResult(err);
    }
  });

  server.tool(
    'get_organization',
    'Get one organization by Snorlx id.',
    { id: z.number().int().describe('Organization id') },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/organizations/${id}`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'list_repositories',
    'List repositories with optional search.',
    {
      page: z.number().int().min(1).optional(),
      per_page: z.number().int().min(1).max(100).optional(),
      search: z.string().optional(),
    },
    async ({ page, per_page, search }) => {
      try {
        return textResult(
          await client.request('GET', '/api/repositories', {
            query: { page: page ?? 1, per_page, search },
          }),
        );
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_repository',
    'Get repository details by Snorlx id.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/repositories/${id}`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'list_repository_scores',
    'List latest repository health scores (gold/silver/bronze).',
    {},
    async () => {
      try {
        return textResult(await client.request('GET', '/api/repositories/scores'));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_repository_score',
    'Get latest health score for one repository.',
    { id: z.number().int().describe('Repository id') },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/repositories/${id}/score`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'list_workflows',
    'List workflows, optionally filtered by repository.',
    { repo_id: z.number().int().optional() },
    async ({ repo_id }) => {
      try {
        return textResult(await client.request('GET', '/api/workflows', { query: { repo_id } }));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_workflow',
    'Get one workflow by id.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/workflows/${id}`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'list_runs',
    'List workflow runs with filters.',
    {
      page: z.number().int().min(1).optional(),
      status: z.string().optional(),
      conclusion: z.string().optional(),
      branch: z.string().optional(),
      repo_id: z.number().int().optional(),
      workflow_id: z.number().int().optional(),
    },
    async (args) => {
      try {
        return textResult(await client.request('GET', '/api/runs', { query: args }));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_run',
    'Get one workflow run.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/runs/${id}`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_run_jobs',
    'List jobs for a workflow run.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/runs/${id}/jobs`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_run_annotations',
    'Get check annotations for a run (prefer before full logs).',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/runs/${id}/annotations`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_run_workflow_definition',
    'Get workflow job graph / needs definition for a run.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('GET', `/api/runs/${id}/workflow-definition`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_run_logs',
    'Fetch run logs (truncated for MCP context). Prefer annotations first.',
    {
      id: z.number().int(),
      max_chars: z.number().int().min(1000).max(100_000).optional(),
    },
    async ({ id, max_chars }) => {
      try {
        const data = await client.request('GET', `/api/runs/${id}/logs`);
        return textResult(client.truncateLogs(data, max_chars));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'get_job_logs',
    'Fetch job logs (truncated for MCP context).',
    {
      id: z.number().int(),
      max_chars: z.number().int().min(1000).max(100_000).optional(),
    },
    async ({ id, max_chars }) => {
      try {
        const data = await client.request('GET', `/api/jobs/${id}/logs`);
        return textResult(client.truncateLogs(data, max_chars));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'sync_repositories',
    'CONFIRM WITH THE USER before calling. Sync all repositories from GitHub into Snorlx (rate-limit heavy).',
    {},
    async () => {
      try {
        return textResult(await client.request('POST', '/api/repositories/sync'));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'sync_repository',
    'CONFIRM WITH THE USER before calling. Sync one repository from GitHub.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('POST', `/api/repositories/${id}/sync`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'backfill_deployment_runs',
    'CONFIRM WITH THE USER before calling. Backfill deployment linkage for historical runs.',
    {},
    async () => {
      try {
        return textResult(await client.request('POST', '/api/repositories/backfill-deployment-runs'));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'update_workflow',
    'CONFIRM WITH THE USER before calling. Update workflow metadata (e.g. is_deployment_workflow).',
    {
      id: z.number().int(),
      is_deployment_workflow: z.boolean().optional(),
    },
    async ({ id, is_deployment_workflow }) => {
      try {
        return textResult(
          await client.request('PATCH', `/api/workflows/${id}`, {
            body: { is_deployment_workflow },
          }),
        );
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'rerun_workflow',
    'CONFIRM WITH THE USER before calling. Rerun a workflow run (consumes GitHub Actions minutes).',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('POST', `/api/runs/${id}/rerun`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  server.tool(
    'cancel_run',
    'CONFIRM WITH THE USER before calling. Cancel an in-progress workflow run.',
    { id: z.number().int() },
    async ({ id }) => {
      try {
        return textResult(await client.request('POST', `/api/runs/${id}/cancel`));
      } catch (err) {
        return errorResult(err);
      }
    },
  );

  return server;
}
