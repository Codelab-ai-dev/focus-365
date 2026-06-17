import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";

// Usuario autenticado falso para evitar el redirect a /login.
vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/lib/ai", () => ({
  getThreads: vi.fn(),
  searchChat: vi.fn(),
}));

import { getThreads, searchChat } from "@/lib/ai";
import { Route as ListRoute } from "./asistente.index";

function renderPage() {
  const rootRoute = createRootRoute();
  const listRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente",
    component: ListRoute.options.component,
  });
  const threadRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente/$threadId",
    component: () => <div>thread</div>,
  });
  const newRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente/new",
    component: () => <div>new</div>,
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
    routeTree: rootRoute.addChildren([listRoute, threadRoute, newRoute, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/asistente"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("ThreadListPage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("renderiza los hilos con título y preview", async () => {
    vi.mocked(getThreads).mockResolvedValue([
      { id: "t1", title: "Finanzas", preview: "cómo voy de plata", updated_at: "2026-06-11T10:00:00Z" },
      { id: "t2", title: "Hábitos", preview: "racha de lectura", updated_at: "2026-06-10T10:00:00Z" },
    ]);
    renderPage();
    expect(await screen.findByText("Finanzas")).toBeInTheDocument();
    expect(screen.getByText("cómo voy de plata")).toBeInTheDocument();
    expect(screen.getByText("Hábitos")).toBeInTheDocument();
    expect(screen.getByText("racha de lectura")).toBeInTheDocument();
  });

  it("enlaza al hilo nuevo", async () => {
    vi.mocked(getThreads).mockResolvedValue([]);
    renderPage();
    const link = await screen.findByRole("link", { name: "Nuevo hilo" });
    expect(link).toHaveAttribute("href", "/asistente/new");
  });

  it("muestra estado vacío cuando no hay hilos", async () => {
    vi.mocked(getThreads).mockResolvedValue([]);
    renderPage();
    expect(await screen.findByText(/Todavía no tenés hilos/)).toBeInTheDocument();
  });

  it("buscar (≥2 chars) muestra resultados y oculta la lista normal", async () => {
    vi.mocked(getThreads).mockResolvedValue([
      { id: "t1", title: "Hilo viejo", preview: "p", updated_at: "" },
    ]);
    vi.mocked(searchChat).mockResolvedValue({
      threads: [{ id: "t9", title: "Finanzas", preview: "hola", updated_at: "" }],
      messages: [
        {
          id: "m1",
          thread_id: "t9",
          thread_title: "Finanzas",
          role: "user",
          content: "gasté 200",
          created_at: "",
        },
      ],
    });
    renderPage();
    expect(await screen.findByText("Hilo viejo")).toBeInTheDocument();

    const input = await screen.findByPlaceholderText("Buscar…");
    await userEvent.type(input, "gaste");

    expect((await screen.findAllByText("Finanzas")).length).toBeGreaterThan(0);
    expect(screen.getByText(/gasté 200/)).toBeInTheDocument();
    expect(screen.queryByText("Hilo viejo")).not.toBeInTheDocument();
  });

  it("input vacío muestra la lista de hilos normal", async () => {
    vi.mocked(getThreads).mockResolvedValue([
      { id: "t1", title: "Hilo viejo", preview: "p", updated_at: "" },
    ]);
    renderPage();
    expect(await screen.findByText("Hilo viejo")).toBeInTheDocument();
    expect(searchChat).not.toHaveBeenCalled();
  });

  it("búsqueda sin resultados muestra 'Sin resultados'", async () => {
    vi.mocked(getThreads).mockResolvedValue([
      { id: "t1", title: "Hilo viejo", preview: "p", updated_at: "" },
    ]);
    vi.mocked(searchChat).mockResolvedValue({ threads: [], messages: [] });
    renderPage();
    const input = await screen.findByPlaceholderText("Buscar…");
    await userEvent.type(input, "zzz");
    expect(await screen.findByText(/Sin resultados/)).toBeInTheDocument();
  });
});
