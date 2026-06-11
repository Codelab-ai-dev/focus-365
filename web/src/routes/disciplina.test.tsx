import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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

import { Route as DisciplinaRoute } from "./disciplina";

function fetchMock() {
  return vi.fn((_url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "h9" }), { status: 201 }));
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    // GET /habits
    return Promise.resolve(
      new Response(
        JSON.stringify([
          {
            id: "h1",
            name: "Leer 20 min",
            target_days: 21,
            current_streak: 5,
            best_streak: 8,
            done_today: false,
            done_yesterday: true,
            archived_at: null,
            created_at: "",
          },
        ]),
        { status: 200 }
      )
    );
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: "/disciplina",
    component: DisciplinaRoute.options.component,
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
    history: createMemoryHistory({ initialEntries: ["/disciplina"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("DisciplinaPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el hábito con su racha", async () => {
    renderPage();
    expect(await screen.findByText("Leer 20 min")).toBeInTheDocument();
    expect(screen.getByText("🔥 5 días")).toBeInTheDocument();
  });

  it("marcar hoy dispara un POST a /check con done:true", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Marcar hoy Leer 20 min" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.find(
        ([url, opts]) =>
          (url as string).startsWith("/api/v1/habits/h1/check") && opts?.method === "POST"
      );
      expect(post).toBeTruthy();
      const body = JSON.parse(post![1].body as string);
      expect(body.done).toBe(true);
    });
  });

  it("crear hábito dispara un POST a /habits", async () => {
    renderPage();
    await userEvent.type(await screen.findByLabelText("Nombre del hábito"), "Meditar");
    await userEvent.click(screen.getByRole("button", { name: "Crear" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.find(
        ([url, opts]) =>
          (url as string).startsWith("/api/v1/habits?today=") && opts?.method === "POST"
      );
      expect(post).toBeTruthy();
      const body = JSON.parse(post![1].body as string);
      expect(body.name).toBe("Meditar");
    });
  });

  it("archivar dispara un POST a /archive", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Archivar Leer 20 min" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.some(
        ([url, opts]) =>
          (url as string).startsWith("/api/v1/habits/h1/archive") && opts?.method === "POST"
      );
      expect(post).toBe(true);
    });
  });
});
