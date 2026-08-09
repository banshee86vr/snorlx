const DEFAULT_LOG_CHARS = 24_000;

export class SnorlxApiClient {
  constructor(
    private readonly baseUrl: string,
    private readonly token: string,
  ) {}

  async request<T = unknown>(
    method: string,
    path: string,
    options?: { query?: Record<string, string | number | boolean | undefined>; body?: unknown },
  ): Promise<T> {
    const url = new URL(path, this.baseUrl.replace(/\/$/, '') + '/');
    if (options?.query) {
      for (const [key, value] of Object.entries(options.query)) {
        if (value !== undefined && value !== '') {
          url.searchParams.set(key, String(value));
        }
      }
    }

    const headers: Record<string, string> = {
      Accept: 'application/json',
      Authorization: `Bearer ${this.token}`,
    };
    let body: string | undefined;
    if (options?.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(options.body);
    }

    const res = await fetch(url, { method, headers, body });
    const text = await res.text();
    if (!res.ok) {
      throw new Error(`Snorlx API ${method} ${path} failed (${res.status}): ${text || res.statusText}`);
    }
    if (!text || res.status === 204) {
      return undefined as T;
    }
    try {
      return JSON.parse(text) as T;
    } catch {
      return text as T;
    }
  }

  truncateLogs(payload: unknown, maxChars = DEFAULT_LOG_CHARS): unknown {
    if (typeof payload === 'string') {
      return truncateText(payload, maxChars);
    }
    if (payload && typeof payload === 'object') {
      const obj = { ...(payload as Record<string, unknown>) };
      for (const key of ['logs', 'content', 'log', 'body', 'text']) {
        if (typeof obj[key] === 'string') {
          obj[key] = truncateText(obj[key] as string, maxChars);
          obj.truncated = true;
          obj.truncated_note = `Log truncated to last ${maxChars} characters for MCP context limits.`;
        }
      }
      return obj;
    }
    return payload;
  }
}

function truncateText(text: string, maxChars: number): string {
  if (text.length <= maxChars) {
    return text;
  }
  return text.slice(text.length - maxChars);
}

export function requireEnv(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}
