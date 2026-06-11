import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";
import type { Snapshot } from "@/lib/dashboard";

vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as IndexRoute } from "./index";

function makeSnap(overrides: Partial<Snapshot> = {}): Snapshot {
  return {
    streak: { best_current: 12, done_today: 2, total: 4 },
    finance: { cycle: "2026-06", net: 320000, status: "verde" },
    checkin: { present: true, mood: 8, energy: 6, discipline: 9 },
    training: { trained_today: true, type: "Fuerza" },
    goals: { active: 3, avg_progress: 40, overdue: 1 },
    dimensions_active: 4,
    ...overrides,
  };
}

function okJson(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify(data), { status: 200 }));
}

function renderPage() {
  const rootRoute = createRootRoute();
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: IndexRoute.options.component,
  });
  const login = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  const module = createRoute({
    getParentRoute: () => rootRoute,
    path: "/disciplina",
    component: () => <div>disciplina</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, login, module]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("DashboardPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn(() => okJson(makeSnap()))));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el saludo con el nombre y las dimensiones", async () => {
    renderPage();
    // El nombre va en un <span> amber, así que "Hola, " y "Ana" quedan en nodos
    // distintos; usamos un matcher sobre el textContent combinado del <p>.
    expect(
      await screen.findByText((_content, el) =>
        el?.tagName === "P" && /Hola,\s*Ana/.test(el.textContent ?? "")
      )
    ).toBeInTheDocument();
    expect(screen.getByText(/4 dimensiones en marcha/)).toBeInTheDocument();
  });

  it("muestra la racha y el superávit en MXN", async () => {
    renderPage();
    expect(await screen.findByText(/12/)).toBeInTheDocument();
    expect(screen.getByText(/\$3,200\.00/)).toBeInTheDocument();
  });

  it("muestra la banda de IA placeholder", async () => {
    renderPage();
    expect(await screen.findByText(/Tu insight del día llega pronto/)).toBeInTheDocument();
  });

  it("muestra aviso de metas vencidas", async () => {
    renderPage();
    expect(await screen.findByText(/1 vencida/)).toBeInTheDocument();
  });

  it("muestra 'Sin check-in hoy' cuando checkin es null", async () => {
    vi.stubGlobal("fetch", vi.fn(() => okJson(makeSnap({ checkin: null, dimensions_active: 3 }))));
    renderPage();
    expect(await screen.findByText(/Sin check-in hoy/)).toBeInTheDocument();
  });

  it("cada tarjeta linkea a su módulo (racha → /disciplina)", async () => {
    renderPage();
    const link = await screen.findByRole("link", { name: /Racha/ });
    expect(link.getAttribute("href")).toBe("/disciplina");
  });
});
