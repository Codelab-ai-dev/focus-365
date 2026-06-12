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
import { MotionConfig } from "framer-motion";
import type { Snapshot } from "@/lib/dashboard";
import type { Insight } from "@/lib/ai";

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

function makeInsight(overrides: Partial<Insight> = {}): Insight {
  return {
    content: "Aprovecha tu energía alta hoy.",
    available: true,
    generated_at: "2026-06-11T10:00:00Z",
    ...overrides,
  };
}

function routeFetch(snap = makeSnap(), insight: Insight | "error" = makeInsight()) {
  return vi.fn((url: string, _opts?: RequestInit) => {
    if (url.includes("/ai/insight")) {
      if (insight === "error") {
        return Promise.resolve(new Response("boom", { status: 500 }));
      }
      return okJson(insight);
    }
    return okJson(snap);
  });
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
    <MotionConfig reducedMotion="always">
      <QueryClientProvider client={qc}>
        {/* @ts-ignore router de prueba */}
        <RouterProvider router={router} />
      </QueryClientProvider>
    </MotionConfig>
  );
}

describe("DashboardPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", routeFetch()));
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
    expect(await screen.findByText(/12 días/)).toBeInTheDocument();
    expect(screen.getByText(/\$3,200\.00/)).toBeInTheDocument();
  });

  it("muestra el insight de IA cuando está disponible", async () => {
    renderPage();
    expect(await screen.findByText(/Aprovecha tu energía alta hoy/)).toBeInTheDocument();
  });

  it("muestra aviso de metas vencidas", async () => {
    renderPage();
    expect(await screen.findByText(/1 vencida/)).toBeInTheDocument();
  });

  it("muestra 'Sin check-in hoy' cuando checkin es null", async () => {
    vi.stubGlobal("fetch", routeFetch(makeSnap({ checkin: null, dimensions_active: 3 })));
    renderPage();
    expect(await screen.findByText(/Sin check-in hoy/)).toBeInTheDocument();
  });

  it("cada tarjeta linkea a su módulo (racha → /disciplina)", async () => {
    renderPage();
    const link = await screen.findByRole("link", { name: /Racha/ });
    expect(link.getAttribute("href")).toBe("/disciplina");
  });

  it("muestra el placeholder cuando la IA no está disponible", async () => {
    vi.stubGlobal("fetch", routeFetch(makeSnap(), makeInsight({ available: false, content: null })));
    renderPage();
    expect(await screen.findByText(/Tu insight del día llega pronto/)).toBeInTheDocument();
  });

  it("muestra el estado de carga del insight", async () => {
    const pending = new Promise<Response>(() => {});
    vi.stubGlobal("fetch", vi.fn((url: string) => {
      if (url.includes("/ai/insight")) return pending;
      return okJson(makeSnap());
    }));
    renderPage();
    expect(await screen.findByText(/Generando tu insight…/)).toBeInTheDocument();
  });

  it("un error de la IA no rompe el resto del dashboard", async () => {
    vi.stubGlobal("fetch", routeFetch(makeSnap(), "error"));
    renderPage();
    expect(await screen.findByText(/Tu insight del día llega pronto/)).toBeInTheDocument();
    // El resto del dashboard sigue visible (la racha llega igual).
    expect(screen.getByText(/12 días/)).toBeInTheDocument();
  });
});
