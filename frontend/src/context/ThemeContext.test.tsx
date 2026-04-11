import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ThemeProvider, useTheme } from "./ThemeContext";

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

function ThemeConsumer() {
  const { theme, isDark, setTheme } = useTheme();

  return (
    <div>
      <p data-testid="theme">{theme}</p>
      <p data-testid="is-dark">{String(isDark)}</p>
      <button onClick={() => setTheme("light")}>Set Light</button>
      <button onClick={() => setTheme("dark")}>Set Dark</button>
      <button onClick={() => setTheme("system")}>Set System</button>
    </div>
  );
}

function setMatchMedia(matches: boolean) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: () => ({
      matches,
      media: "(prefers-color-scheme: dark)",
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

describe("ThemeContext", () => {
  beforeEach(() => {
    mockLocalStorage();
    document.documentElement.classList.remove("dark");
  });

  it("initializes from localStorage and applies dark mode", async () => {
    mockLocalStorage({ theme: "dark" });
    setMatchMedia(false);

    render(
      <ThemeProvider>
        <ThemeConsumer />
      </ThemeProvider>
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("dark");

    await waitFor(() => {
      expect(screen.getByTestId("is-dark")).toHaveTextContent("true");
      expect(document.documentElement.classList.contains("dark")).toBe(true);
    });
  });

  it("uses system preference when theme is system", async () => {
    setMatchMedia(true);

    render(
      <ThemeProvider>
        <ThemeConsumer />
      </ThemeProvider>
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("system");

    await waitFor(() => {
      expect(screen.getByTestId("is-dark")).toHaveTextContent("true");
      expect(document.documentElement.classList.contains("dark")).toBe(true);
    });
  });

  it("updates theme and localStorage when setTheme is called", async () => {
    setMatchMedia(true);

    render(
      <ThemeProvider>
        <ThemeConsumer />
      </ThemeProvider>
    );

    fireEvent.click(screen.getByRole("button", { name: "Set Light" }));

    expect(localStorage.getItem("theme")).toBe("light");
    expect(screen.getByTestId("theme")).toHaveTextContent("light");

    await waitFor(() => {
      expect(screen.getByTestId("is-dark")).toHaveTextContent("false");
      expect(document.documentElement.classList.contains("dark")).toBe(false);
    });
  });
});
