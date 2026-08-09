#!/usr/bin/env node
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';

import { requireEnv, SnorlxApiClient } from './client.js';
import { startHttpServer } from './http.js';
import { createSnorlxMcpServer } from './server.js';

async function main(): Promise<void> {
  const baseUrl = process.env.SNORLX_API_URL?.trim() || 'http://localhost:8080';
  const token = requireEnv('SNORLX_API_TOKEN');
  const client = new SnorlxApiClient(baseUrl, token);
  const httpMode = process.argv.includes('--http') || process.env.MCP_TRANSPORT === 'http';

  if (httpMode) {
    const port = Number(process.env.MCP_PORT || process.env.PORT || 3100);
    const host = process.env.MCP_HOST?.trim() || '127.0.0.1';
    const httpToken = requireEnv('MCP_HTTP_TOKEN');
    await startHttpServer({ client, port, host, httpToken });
    return;
  }

  const server = createSnorlxMcpServer(client);
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
