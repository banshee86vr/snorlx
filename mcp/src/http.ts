import { randomUUID, timingSafeEqual } from 'node:crypto';

import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import express from 'express';

import { SnorlxApiClient } from './client.js';
import { createSnorlxMcpServer } from './server.js';

function safeEqual(a: string, b: string): boolean {
  const left = Buffer.from(a);
  const right = Buffer.from(b);
  if (left.length !== right.length) {
    return false;
  }
  return timingSafeEqual(left, right);
}

function requireMcpHttpAuth(httpToken: string): express.RequestHandler {
  return (req, res, next) => {
    const header = req.get('authorization') || '';
    if (!header.startsWith('Bearer ')) {
      res.status(401).json({ error: 'Unauthorized', message: 'Bearer MCP_HTTP_TOKEN required' });
      return;
    }
    const presented = header.slice('Bearer '.length).trim();
    if (!presented || !safeEqual(presented, httpToken)) {
      res.status(401).json({ error: 'Unauthorized', message: 'Invalid MCP HTTP token' });
      return;
    }
    next();
  };
}

export async function startHttpServer(opts: {
  client: SnorlxApiClient;
  port: number;
  host: string;
  httpToken: string;
}): Promise<void> {
  const app = express();
  app.use(express.json({ limit: '4mb' }));

  const sessions = new Map<string, StreamableHTTPServerTransport>();
  const auth = requireMcpHttpAuth(opts.httpToken);

  const handle = async (req: express.Request, res: express.Response) => {
    const sessionId = req.headers['mcp-session-id'] as string | undefined;

    try {
      if (sessionId && sessions.has(sessionId)) {
        const transport = sessions.get(sessionId)!;
        await transport.handleRequest(req, res, req.body);
        return;
      }

      if (req.method === 'POST') {
        const transport = new StreamableHTTPServerTransport({
          sessionIdGenerator: () => randomUUID(),
          onsessioninitialized: (id) => {
            sessions.set(id, transport);
          },
        });
        transport.onclose = () => {
          if (transport.sessionId) {
            sessions.delete(transport.sessionId);
          }
        };
        const server = createSnorlxMcpServer(opts.client);
        await server.connect(transport);
        await transport.handleRequest(req, res, req.body);
        return;
      }

      res.status(400).json({ error: 'Invalid or missing MCP session' });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (!res.headersSent) {
        res.status(500).json({ error: message });
      }
    }
  };

  app.post('/mcp', auth, handle);
  app.get('/mcp', auth, handle);
  app.delete('/mcp', auth, handle);

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', service: 'snorlx-mcp' });
  });

  await new Promise<void>((resolve) => {
    app.listen(opts.port, opts.host, () => {
      console.error(`snorlx-mcp Streamable HTTP listening on http://${opts.host}:${opts.port}/mcp`);
      resolve();
    });
  });
}
