import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { Layout } from "./Layout";

const mockUseSidebar = vi.fn();
const mockUseSync = vi.fn();

vi.mock("./Sidebar", () => ({
  Sidebar: () => <aside>Sidebar Stub</aside>,
}));

vi.mock("./Header", () => ({
  Header: () => <header>Header Stub</header>,
}));

vi.mock("../../context/SidebarContext", () => ({
  useSidebar: () => mockUseSidebar(),
}));

vi.mock("../../context/SyncContext", () => ({
  useSync: () => mockUseSync(),
}));

describe("Layout", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseSidebar.mockReturnValue({ isCollapsed: false });
    mockUseSync.mockReturnValue({
      sync: {
        isSyncing: false,
        syncedCount: 0,
        totalToSync: 0,
        progress: 0,
        currentRepo: null,
      },
    });
  });

  it("renders layout shell and children", () => {
    render(
      <Layout>
        <div>Page Content</div>
      </Layout>
    );

    expect(screen.getByText("Sidebar Stub")).toBeInTheDocument();
    expect(screen.getByText("Header Stub")).toBeInTheDocument();
    expect(screen.getByText("Page Content")).toBeInTheDocument();
  });

  it("applies collapsed sidebar spacing class", () => {
    mockUseSidebar.mockReturnValue({ isCollapsed: true });

    const { container } = render(
      <Layout>
        <div>Page Content</div>
      </Layout>
    );

    expect(container.querySelector(".lg\\:pl-20")).toBeInTheDocument();
  });

  it("renders sync progress overlay while syncing", () => {
    mockUseSync.mockReturnValue({
      sync: {
        isSyncing: true,
        syncedCount: 2,
        totalToSync: 5,
        progress: 40,
        currentRepo: "repo-a",
      },
    });

    render(
      <Layout>
        <div>Page Content</div>
      </Layout>
    );

    expect(screen.getByText("Syncing Repositories")).toBeInTheDocument();
    expect(screen.getByText("2 of 5")).toBeInTheDocument();
    expect(screen.getByText("repo-a")).toBeInTheDocument();
    expect(screen.getByText("40% complete")).toBeInTheDocument();
  });

  it("shows fetching message when syncing without current repo", () => {
    mockUseSync.mockReturnValue({
      sync: {
        isSyncing: true,
        syncedCount: 0,
        totalToSync: 10,
        progress: 0,
        currentRepo: null,
      },
    });

    render(
      <Layout>
        <div>Page Content</div>
      </Layout>
    );

    expect(screen.getByText("Fetching repositories from GitHub...")).toBeInTheDocument();
  });
});
