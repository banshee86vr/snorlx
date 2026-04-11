import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { SidebarProvider, useSidebar } from "./SidebarContext";

function mockLocalStorage(initial: Record<string, string> = {}) {
  const store: Record<string, string> = { ...initial };
  const localStorageMock = {
    getItem: (key: string) => (key in store ? store[key] : null),
    setItem: (key: string, value: string) => {
      store[key] = String(value);
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      for (const key of Object.keys(store)) {
        delete store[key];
      }
    },
  };

  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: localStorageMock,
  });
}

function SidebarConsumer() {
  const { isCollapsed, toggleSidebar, setCollapsed } = useSidebar();

  return (
    <div>
      <p data-testid="collapsed">{String(isCollapsed)}</p>
      <button onClick={toggleSidebar}>Toggle</button>
      <button onClick={() => setCollapsed(true)}>Set True</button>
      <button onClick={() => setCollapsed(false)}>Set False</button>
    </div>
  );
}

describe("SidebarContext", () => {
  beforeEach(() => {
    mockLocalStorage();
  });

  it("initializes from localStorage", () => {
    mockLocalStorage({ "sidebar-collapsed": "true" });

    render(
      <SidebarProvider>
        <SidebarConsumer />
      </SidebarProvider>
    );

    expect(screen.getByTestId("collapsed")).toHaveTextContent("true");
  });

  it("toggles collapsed state and persists to localStorage", () => {
    render(
      <SidebarProvider>
        <SidebarConsumer />
      </SidebarProvider>
    );

    expect(screen.getByTestId("collapsed")).toHaveTextContent("false");

    fireEvent.click(screen.getByRole("button", { name: "Toggle" }));

    expect(screen.getByTestId("collapsed")).toHaveTextContent("true");
    expect(localStorage.getItem("sidebar-collapsed")).toBe("true");
  });

  it("sets collapsed explicitly and persists value", () => {
    render(
      <SidebarProvider>
        <SidebarConsumer />
      </SidebarProvider>
    );

    fireEvent.click(screen.getByRole("button", { name: "Set True" }));
    expect(screen.getByTestId("collapsed")).toHaveTextContent("true");
    expect(localStorage.getItem("sidebar-collapsed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Set False" }));
    expect(screen.getByTestId("collapsed")).toHaveTextContent("false");
    expect(localStorage.getItem("sidebar-collapsed")).toBe("false");
  });
});
