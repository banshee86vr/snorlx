import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ProtectedRoute } from "./ProtectedRoute";
import * as AuthContext from "../context/AuthContext";

// Helper to render ProtectedRoute inside a router
function renderProtectedRoute(children: React.ReactNode) {
  return render(
    <MemoryRouter initialEntries={["/dashboard"]}>
      <ProtectedRoute>{children}</ProtectedRoute>
    </MemoryRouter>
  );
}

// Mock useAuth with a controllable return value
function mockUseAuth(value: Partial<AuthContext.AuthContextType>) {
  vi.spyOn(AuthContext, "useAuth").mockReturnValue({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    logout: vi.fn(),
    checkAuth: vi.fn(),
    ...value,
  });
}

const testUser = {
  id: 1,
  login: "octocat",
  name: "The Octocat",
  email: null,
  avatar_url: null,
};

describe("ProtectedRoute", () => {
  it("renders children when authenticated", () => {
    mockUseAuth({
      isAuthenticated: true,
      isLoading: false,
      user: testUser,
    });

    renderProtectedRoute(<div>Protected Content</div>);

    expect(screen.getByText("Protected Content")).toBeInTheDocument();
  });

  it("redirects to /login when not authenticated", () => {
    mockUseAuth({ isAuthenticated: false, isLoading: false, user: null });

    renderProtectedRoute(<div>Protected Content</div>);

    // Navigate component renders nothing visible; check that protected content is absent
    expect(screen.queryByText("Protected Content")).not.toBeInTheDocument();
  });

  it("shows loading spinner while authentication is being checked", () => {
    mockUseAuth({ isAuthenticated: false, isLoading: true, user: null });

    renderProtectedRoute(<div>Protected Content</div>);

    expect(screen.getByText("Loading...")).toBeInTheDocument();
    expect(screen.queryByText("Protected Content")).not.toBeInTheDocument();
  });

  it("does not show loading spinner when authentication is resolved", () => {
    mockUseAuth({ isAuthenticated: true, isLoading: false, user: testUser });

    renderProtectedRoute(<div>Protected Content</div>);

    expect(screen.queryByText("Loading...")).not.toBeInTheDocument();
  });

  it("renders multiple children when authenticated", () => {
    mockUseAuth({ isAuthenticated: true, isLoading: false, user: testUser });

    renderProtectedRoute(
      <>
        <div>Child One</div>
        <div>Child Two</div>
      </>
    );

    expect(screen.getByText("Child One")).toBeInTheDocument();
    expect(screen.getByText("Child Two")).toBeInTheDocument();
  });
});
