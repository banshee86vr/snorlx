import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  authApi,
  organizationsApi,
  repositoriesApi,
  workflowsApi,
  runsApi,
  dashboardApi,
  api,
} from "./api";

// ===== fetch mock setup =====

function mockFetch(body: unknown, status = 200) {
  global.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(String(body)),
  });
}

beforeEach(() => {
  vi.restoreAllMocks();
});

// ===== fetchApi error handling =====

describe("fetchApi error handling", () => {
  it("throws on non-ok response", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({}),
      text: () => Promise.resolve("Unauthorized"),
    });

    await expect(authApi.getStatus()).rejects.toThrow("Unauthorized");
  });

  it("throws with HTTP error message when body is empty", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({}),
      text: () => Promise.resolve(""),
    });

    await expect(authApi.getStatus()).rejects.toThrow("HTTP error 500");
  });

  it("passes credentials: include on every request", async () => {
    mockFetch({ authenticated: false });

    await authApi.getStatus();

    const [, options] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(options.credentials).toBe("include");
  });

  it("sends Content-Type: application/json header", async () => {
    mockFetch({ authenticated: false });

    await authApi.getStatus();

    const [, options] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(options.headers["Content-Type"]).toBe("application/json");
  });
});

// ===== authApi =====

describe("authApi", () => {
  it("getStatus calls /api/auth/status", async () => {
    mockFetch({ authenticated: true, user: { id: 1, login: "octocat" } });

    const result = await authApi.getStatus();

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/auth/status"),
      expect.any(Object)
    );
    expect(result.authenticated).toBe(true);
  });

  it("logout calls /api/auth/logout with POST", async () => {
    mockFetch({});

    await authApi.logout();

    const [url, options] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/auth/logout");
    expect(options.method).toBe("POST");
  });
});

// ===== organizationsApi =====

describe("organizationsApi", () => {
  it("list calls /api/organizations", async () => {
    mockFetch([{ id: 1, login: "my-org" }]);

    const result = await organizationsApi.list();

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/organizations"),
      expect.any(Object)
    );
    expect(result).toHaveLength(1);
  });

  it("get calls /api/organizations/:id", async () => {
    mockFetch({ id: 5, login: "test-org" });

    const result = await organizationsApi.get(5);

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/organizations/5"),
      expect.any(Object)
    );
    expect(result.id).toBe(5);
  });
});

// ===== repositoriesApi =====

describe("repositoriesApi", () => {
  it("list includes page param", async () => {
    mockFetch({ data: [], pagination: { page: 1, page_size: 20, total: 0 } });

    await repositoriesApi.list(2);

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("page=2");
  });

  it("list includes search param when provided", async () => {
    mockFetch({ data: [], pagination: { page: 1, page_size: 20, total: 0 } });

    await repositoriesApi.list(1, "my-repo");

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("search=my-repo");
  });

  it("list does not include search param when not provided", async () => {
    mockFetch({ data: [], pagination: { page: 1, page_size: 20, total: 0 } });

    await repositoriesApi.list(1);

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).not.toContain("search=");
  });

  it("get calls /api/repositories/:id", async () => {
    mockFetch({ id: 42, name: "my-repo" });

    const result = await repositoriesApi.get(42);

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/repositories/42"),
      expect.any(Object)
    );
    expect(result.id).toBe(42);
  });

  it("sync calls POST /api/repositories/sync", async () => {
    mockFetch({ status: "started" });

    await repositoriesApi.sync();

    const [url, options] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/repositories/sync");
    expect(options.method).toBe("POST");
  });
});

// ===== workflowsApi =====

describe("workflowsApi", () => {
  it("list calls /api/workflows", async () => {
    mockFetch([{ id: 1, name: "CI" }]);

    await workflowsApi.list();

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/workflows"),
      expect.any(Object)
    );
  });

  it("list includes repo_id when provided", async () => {
    mockFetch([]);

    await workflowsApi.list(10);

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("repo_id=10");
  });

  it("get calls /api/workflows/:id", async () => {
    mockFetch({ id: 7, name: "Deploy" });

    await workflowsApi.get(7);

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/workflows/7"),
      expect.any(Object)
    );
  });

  it("getRuns includes page param", async () => {
    mockFetch({ data: [], pagination: {} });

    await workflowsApi.getRuns(3, 2);

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/workflows/3/runs");
    expect(url).toContain("page=2");
  });
});

// ===== runsApi =====

describe("runsApi", () => {
  it("list calls /api/runs with page param", async () => {
    mockFetch({ data: [], pagination: {} });

    await runsApi.list(undefined, 1);

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/runs");
    expect(url).toContain("page=1");
  });

  it("list includes filter params when provided", async () => {
    mockFetch({ data: [], pagination: {} });

    await runsApi.list({ status: "completed", branch: "main" });

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("status=completed");
    expect(url).toContain("branch=main");
  });

  it("get calls /api/runs/:id", async () => {
    mockFetch({ id: 100, status: "completed" });

    await runsApi.get(100);

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/runs/100"),
      expect.any(Object)
    );
  });

  it("getJobs calls /api/runs/:id/jobs", async () => {
    mockFetch([{ id: 1, name: "build" }]);

    await runsApi.getJobs(55);

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/runs/55/jobs"),
      expect.any(Object)
    );
  });

  it("rerun calls POST /api/runs/:id/rerun", async () => {
    mockFetch({ status: "queued" });

    await runsApi.rerun(77);

    const [url, options] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/runs/77/rerun");
    expect(options.method).toBe("POST");
  });

  it("cancel calls POST /api/runs/:id/cancel", async () => {
    mockFetch({ status: "cancelled" });

    await runsApi.cancel(88);

    const [url, options] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/runs/88/cancel");
    expect(options.method).toBe("POST");
  });
});

// ===== dashboardApi =====

describe("dashboardApi", () => {
  it("getSummary calls /api/dashboard/summary", async () => {
    mockFetch({ repositories: { total: 5 } });

    await dashboardApi.getSummary();

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/dashboard/summary"),
      expect.any(Object)
    );
  });

  it("getTrends calls /api/dashboard/trends with days param", async () => {
    mockFetch({ trends: [] });

    await dashboardApi.getTrends(14);

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("/api/dashboard/trends");
    expect(url).toContain("days=14");
  });

  it("getTrends defaults to 30 days", async () => {
    mockFetch({ trends: [] });

    await dashboardApi.getTrends();

    const [url] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain("days=30");
  });
});

// ===== api convenience export =====

describe("api convenience export", () => {
  it("syncRepositories delegates to repositoriesApi.sync", async () => {
    mockFetch({ status: "started" });

    const result = await api.syncRepositories();

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/repositories/sync"),
      expect.any(Object)
    );
    expect(result.status).toBe("started");
  });
});
