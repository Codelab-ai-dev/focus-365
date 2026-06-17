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
  sendMessageStream: vi.fn(),
}));

import { sendMessageStream } from "@/lib/ai";
import { Route as NewRoute } from "./asistente.new";

let router: ReturnType<typeof createRouter>;

function renderPage() {
  const rootRoute = createRootRoute();
  const newRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente/new",
    component: NewRoute.options.component,
  });
  const threadRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente/$threadId",
    component: () => {
      return <div>hilo abierto</div>;
    },
  });
  const listRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente",
    component: () => <div>lista de hilos</div>,
  });
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  router = createRouter({
    routeTree: rootRoute.addChildren([newRoute, threadRoute, listRoute, loginRoute]),
    history: createMemoryHistory({ initialEntries: ["/asistente/new"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("NewThreadPage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("al enviar navega al hilo creado", async () => {
    vi.mocked(sendMessageStream).mockImplementation(async (_msg, _tid, onDelta) => {
      onDelta("Hola.");
      return {
        reply: { id: "m1", role: "assistant", content: "Hola.", created_at: "2026-06-11T10:00:00Z" },
        threadId: "t9",
      };
    });
    renderPage();
    const input = await screen.findByLabelText("Mensaje");
    await userEvent.type(input, "empezá un hilo");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    await waitFor(() => {
      expect(vi.mocked(sendMessageStream)).toHaveBeenCalledWith(
        "empezá un hilo",
        undefined,
        expect.any(Function)
      );
    });
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/asistente/t9");
    });
    expect(await screen.findByText("hilo abierto")).toBeInTheDocument();
  });

  it("muestra error inline si el envío falla", async () => {
    vi.mocked(sendMessageStream).mockRejectedValue(new Error("asistente no disponible"));
    renderPage();
    const input = (await screen.findByLabelText("Mensaje")) as HTMLInputElement;
    await userEvent.type(input, "hola");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    expect(await screen.findByText(/no disponible/i)).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/asistente/new");
  });
});
