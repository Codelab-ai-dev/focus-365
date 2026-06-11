import { describe, it, expect, vi, afterEach } from "vitest";
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

import { Route as AsistenteRoute } from "./asistente";

function renderPage() {
  const rootRoute = createRootRoute();
  const asistenteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente",
    component: AsistenteRoute.options.component,
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
    routeTree: rootRoute.addChildren([asistenteRoute, loginRoute, homeRoute]),
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

describe("AsistentePage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("renderiza el historial existente", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              messages: [
                { role: "user", content: "¿cómo voy?", created_at: "2026-06-11T10:00:00Z" },
                { role: "assistant", content: "Vas verde.", created_at: "2026-06-11T10:00:01Z" },
              ],
            }),
            { status: 200 }
          )
        )
      )
    );
    renderPage();
    expect(await screen.findByText("¿cómo voy?")).toBeInTheDocument();
    expect(screen.getByText("Vas verde.")).toBeInTheDocument();
  });

  it("al enviar dispara un POST con el mensaje", async () => {
    const fetchMock = vi.fn((_url: string, opts?: RequestInit) => {
      if (opts?.method === "POST") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              reply: { role: "assistant", content: "Respuesta.", created_at: "2026-06-11T10:00:02Z" },
            }),
            { status: 200 }
          )
        );
      }
      return Promise.resolve(new Response(JSON.stringify({ messages: [] }), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    const input = await screen.findByLabelText("Mensaje");
    await userEvent.type(input, "hola");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.some(
        ([url, opts]) => url === "/api/v1/ai/chat" && opts?.method === "POST"
      );
      expect(posted).toBe(true);
    });
  });

  it("muestra error inline sin romper la página cuando el POST falla", async () => {
    const fetchMock = vi.fn((_url: string, opts?: RequestInit) => {
      if (opts?.method === "POST") {
        return Promise.resolve(
          new Response(JSON.stringify({ error: "asistente no disponible por ahora" }), { status: 503 })
        );
      }
      return Promise.resolve(new Response(JSON.stringify({ messages: [] }), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    const input = (await screen.findByLabelText("Mensaje")) as HTMLInputElement;
    await userEvent.type(input, "hola");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    expect(await screen.findByText(/no disponible/i)).toBeInTheDocument();
    // El texto tecleado no se pierde (permite reintentar).
    expect(input.value).toBe("hola");
  });
});
