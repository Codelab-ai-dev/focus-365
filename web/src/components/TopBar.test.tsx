import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";
import { ThemeProvider } from "@/ui/theme";

const mockAuth = { user: null as null | { id: string; email: string; name: string } };
vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: mockAuth.user,
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { TopBar } from "./TopBar";

function renderBar() {
  const rootRoute = createRootRoute({ component: TopBar });
  const home = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([home]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  // @ts-ignore router de prueba
  render(
    <ThemeProvider>
      <RouterProvider router={router} />
    </ThemeProvider>
  );
}

describe("TopBar", () => {
  afterEach(() => vi.restoreAllMocks());

  it("no muestra nada sin usuario", () => {
    mockAuth.user = null;
    renderBar();
    expect(screen.queryByText("Focus 365")).not.toBeInTheDocument();
  });

  it("muestra links con usuario", async () => {
    mockAuth.user = { id: "u1", email: "a@b.com", name: "Ana" };
    renderBar();
    // el router de TanStack monta el componente raíz de forma asíncrona
    expect(await screen.findByText("Focus 365")).toBeInTheDocument();
    expect(screen.getByText("Finanzas")).toBeInTheDocument();
    expect(screen.getByText("Salir")).toBeInTheDocument();
  });

  it("incluye el toggle de tema", async () => {
    mockAuth.user = { id: "u1", email: "a@b.com", name: "Ana" };
    renderBar();
    expect(
      await screen.findByRole("button", { name: /Cambiar a tema/ })
    ).toBeInTheDocument();
  });
});
