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

  it("subir un archivo muestra las tarjetas extraídas", async () => {
    const movimiento = {
      id: "a1",
      kind: "movimiento",
      payload: { type: "expense", amount_centavos: 12500, category: "comida" },
      status: "proposed",
    };
    let pending: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, opts?: RequestInit) => {
        if (url === "/api/v1/ai/import" && opts?.method === "POST") {
          pending = [movimiento];
          return Promise.resolve(
            new Response(
              JSON.stringify({ created: [movimiento], dropped: 0, truncated: false }),
              { status: 200 }
            )
          );
        }
        if (url === "/api/v1/ai/import/pending") {
          return Promise.resolve(
            new Response(JSON.stringify({ actions: pending }), { status: 200 })
          );
        }
        if (url.includes("/summary")) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                cycle: "2026-06", income: 0, expense: 0, net: 0, status: "pendiente",
              }),
              { status: 200 }
            )
          );
        }
        if (url.includes("/cycles")) {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }));
        }
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }));
      })
    );

    renderPage();
    const input = await screen.findByLabelText("Subir comprobante");
    const file = new File(["x,y\n1,2\n"], "ticket.csv", { type: "text/csv" });
    await userEvent.upload(input as HTMLInputElement, file);

    expect(await screen.findByText("Movimiento")).toBeInTheDocument();
    expect((await screen.findAllByText("Confirmar"))[0]).toBeInTheDocument();
  });

  it("Confirmar todos confirma cada pendiente", async () => {
    const m1 = {
      id: "a1", kind: "movimiento",
      payload: { type: "expense", amount_centavos: 12500, category: "comida" },
      status: "proposed",
    };
    const m2 = {
      id: "a2", kind: "movimiento",
      payload: { type: "income", amount_centavos: 50000, category: "sueldo" },
      status: "proposed",
    };
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, _opts?: RequestInit) => {
        if (url === "/api/v1/ai/import/pending") {
          return Promise.resolve(
            new Response(JSON.stringify({ actions: [m1, m2] }), { status: 200 })
          );
        }
        if (/\/ai\/actions\/.+\/confirm$/.test(url)) {
          return Promise.resolve(
            new Response(JSON.stringify({ action: { ...m1, status: "done" } }), { status: 200 })
          );
        }
        if (url.includes("/summary")) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ cycle: "2026-06", income: 0, expense: 0, net: 0, status: "pendiente" }),
              { status: 200 }
            )
          );
        }
        if (url.includes("/cycles")) {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }));
        }
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }));
      })
    );

    renderPage();
    const btn = await screen.findByRole("button", { name: "Confirmar todos" });
    await userEvent.click(btn);

    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const confirmed = calls.filter(([url]) =>
        /\/ai\/actions\/.+\/confirm$/.test(url as string)
      );
      expect(confirmed.length).toBe(2);
    });
  });
});
