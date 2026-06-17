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
  getThreadMessages: vi.fn(),
  sendMessageStream: vi.fn(),
  renameThread: vi.fn(),
  deleteThread: vi.fn(),
  confirmAction: vi.fn(),
  cancelAction: vi.fn(),
  undoAction: vi.fn(),
}));

import { getThreadMessages, sendMessageStream } from "@/lib/ai";
import { Route as ThreadRoute } from "./asistente.$threadId";

let router: ReturnType<typeof createRouter>;

function renderPage() {
  const rootRoute = createRootRoute();
  const threadRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente/$threadId",
    component: ThreadRoute.options.component,
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
    routeTree: rootRoute.addChildren([threadRoute, listRoute, loginRoute]),
    history: createMemoryHistory({ initialEntries: ["/asistente/t1"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("ThreadChatPage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("renderiza los mensajes del hilo", async () => {
    vi.mocked(getThreadMessages).mockResolvedValue([
      { id: "m1", role: "user", content: "¿cómo voy?", created_at: "2026-06-11T10:00:00Z" },
      { id: "m2", role: "assistant", content: "Vas verde.", created_at: "2026-06-11T10:00:01Z" },
    ]);
    renderPage();
    expect(await screen.findByText("¿cómo voy?")).toBeInTheDocument();
    expect(screen.getByText("Vas verde.")).toBeInTheDocument();
  });

  it("al enviar agrega la burbuja con la respuesta", async () => {
    vi.mocked(getThreadMessages).mockResolvedValue([]);
    vi.mocked(sendMessageStream).mockImplementation(async (_msg, _tid, onDelta) => {
      onDelta("Vas ");
      onDelta("verde.");
      return {
        reply: { id: "m2", role: "assistant", content: "Vas verde.", created_at: "2026-06-11T10:00:02Z" },
        threadId: "t1",
      };
    });
    renderPage();
    const input = await screen.findByLabelText("Mensaje");
    await userEvent.type(input, "¿cómo voy?");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    await waitFor(() => {
      expect(vi.mocked(sendMessageStream)).toHaveBeenCalledWith("¿cómo voy?", "t1", expect.any(Function));
    });
    expect(await screen.findByText("Vas verde.")).toBeInTheDocument();
    expect(screen.getByText("¿cómo voy?")).toBeInTheDocument();
  });

  it("redirige a la lista si el hilo no existe", async () => {
    vi.mocked(getThreadMessages).mockRejectedValue(new Error("Error 404"));
    renderPage();
    expect(await screen.findByText("lista de hilos")).toBeInTheDocument();
  });
});
