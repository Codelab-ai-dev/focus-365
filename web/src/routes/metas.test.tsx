import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";

vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as MetasRoute } from "./metas";

type GoalOverride = {
  id?: string;
  title?: string;
  dimension?: string;
  status?: string;
  progress?: number;
  deadline?: string | null;
  overdue?: boolean;
  created_at?: string;
};

function makeGoal(overrides: GoalOverride = {}) {
  return {
    id: "1",
    title: "Correr 10k",
    dimension: "entrenamiento",
    status: "active",
    progress: 30,
    deadline: null,
    overdue: false,
    created_at: "2026-06-11T00:00:00Z",
    ...overrides,
  };
}

function fetchMock(goals: ReturnType<typeof makeGoal>[]) {
  return vi.fn((_url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "g9" }), { status: 201 }));
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (opts?.method === "PATCH") {
      return Promise.resolve(new Response(JSON.stringify(goals[0]), { status: 200 }));
    }
    // GET /goals
    return Promise.resolve(
      new Response(JSON.stringify(goals), { status: 200 })
    );
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: "/metas",
    component: MetasRoute.options.component,
  });
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/metas"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("MetasPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock([makeGoal()])));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el título de la meta", async () => {
    renderPage();
    expect(await screen.findByText("Correr 10k")).toBeInTheDocument();
  });

  it("cambiar el slider dispara un PATCH con progress:80", async () => {
    renderPage();
    const slider = await screen.findByRole("slider", { name: "Progreso Correr 10k" });
    await userEvent.type(slider, "{arrowRight}");
    // Disparamos el cambio con fireEvent para simular onChange con valor 80
    fireEvent.change(slider, { target: { value: "80" } });
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const patch = calls.find(
        ([url, opts]) =>
          (url as string).includes("/api/v1/goals/1") && opts?.method === "PATCH"
      );
      expect(patch).toBeTruthy();
      const body = JSON.parse(patch![1].body as string);
      expect(body.progress).toBe(80);
    });
  });

  it("muestra 'Vencida' cuando overdue es true", async () => {
    vi.stubGlobal(
      "fetch",
      fetchMock([makeGoal({ deadline: "2026-01-01", overdue: true })])
    );
    renderPage();
    expect(await screen.findByText(/Vencida/)).toBeInTheDocument();
  });
});
