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

import { Route as FinanzasRoute } from "./finanzas";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(
        new Response(JSON.stringify({ id: "t9" }), { status: 201 })
      );
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/summary")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            cycle: "2026-06", income: 500000, expense: 200000,
            net: 300000, status: "pendiente",
          }),
          { status: 200 }
        )
      );
    }
    if (url.includes("/cycles")) {
      return Promise.resolve(
        new Response(
          JSON.stringify([
            { cycle: "2026-06", income: 500000, expense: 200000, net: 300000, status: "pendiente" },
          ]),
          { status: 200 }
        )
      );
    }
    // GET /transactions
    return Promise.resolve(
      new Response(
        JSON.stringify([
          {
            id: "t1", type: "expense", amount: 200000, occurred_on: "2026-06-12",
            cycle: "2026-06", category: "renta", remark: "", source: "manual",
            created_at: "", updated_at: "",
          },
        ]),
        { status: 200 }
      )
    );
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const finanzasRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/finanzas",
    component: FinanzasRoute.options.component,
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
    routeTree: rootRoute.addChildren([finanzasRoute, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/finanzas"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("FinanzasPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el resumen del ciclo y una transacción", async () => {
    renderPage();
    expect((await screen.findAllByText(/pendiente/i))[0]).toBeInTheDocument();
    expect(await screen.findByText("renta")).toBeInTheDocument();
  });

  it("al guardar dispara un POST", async () => {
    renderPage();
    const monto = await screen.findByLabelText("Monto");
    await userEvent.type(monto, "150");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const posted = calls.some(
        ([url, opts]) =>
          url === "/api/v1/finances/transactions" && opts?.method === "POST"
      );
      expect(posted).toBe(true);
    });
  });

  it("al borrar una transacción dispara un DELETE", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Borrar renta" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const deleted = calls.some(
        ([url, opts]) =>
          url === "/api/v1/finances/transactions/t1" && opts?.method === "DELETE"
      );
      expect(deleted).toBe(true);
    });
  });
});
