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

import { Route as EntrenamientoRoute } from "./entrenamiento";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "w9" }), { status: 201 }));
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/exercises")) {
      return Promise.resolve(
        new Response(
          JSON.stringify([{ id: "e1", name: "Sentadilla", created_at: "" }]),
          { status: 200 }
        )
      );
    }
    // GET /workouts
    return Promise.resolve(
      new Response(
        JSON.stringify([
          {
            id: "w1",
            date: "2026-06-11",
            type: "Fuerza",
            note: "",
            sets: [{ exercise: "Sentadilla", reps: 8, weight_grams: 80000 }],
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
    path: "/entrenamiento",
    component: EntrenamientoRoute.options.component,
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
    history: createMemoryHistory({ initialEntries: ["/entrenamiento"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("EntrenamientoPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el historial de sesiones", async () => {
    renderPage();
    // El nombre del ejercicio se renderiza junto a las reps y el peso
    // como nodos de texto hermanos, por eso usamos una coincidencia flexible.
    expect(
      await screen.findByText((_, el) => el?.tagName === "LI" && el.textContent?.startsWith("Sentadilla") === true)
    ).toBeInTheDocument();
  });

  it("al guardar dispara un POST con el peso en gramos", async () => {
    renderPage();
    await userEvent.type(await screen.findByLabelText("Ejercicio 1"), "Sentadilla");
    await userEvent.type(screen.getByLabelText("Reps 1"), "8");
    await userEvent.type(screen.getByLabelText("Peso 1"), "80");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.find(
        ([url, opts]) =>
          url === "/api/v1/training/workouts" && opts?.method === "POST"
      );
      expect(post).toBeTruthy();
      const body = JSON.parse(post![1].body as string);
      expect(body.sets[0].weight_grams).toBe(80000);
      expect(body.sets[0].reps).toBe(8);
    });
  });

  it("al borrar una sesión dispara un DELETE", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Borrar sesión 2026-06-11" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const del = calls.some(
        ([url, opts]) =>
          url === "/api/v1/training/workouts/w1" && opts?.method === "DELETE"
      );
      expect(del).toBe(true);
    });
  });
});
