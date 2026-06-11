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

// Inyectamos un usuario autenticado falso para evitar el redirect a /login.
vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as CheckInRoute } from "./check-in";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            id: "c1", date: "2026-06-10", mood: 5, energy: 5,
            discipline: 5, note: "", created_at: "", updated_at: "",
          }),
          { status: 200 }
        )
      );
    }
    if (url.includes("/today")) {
      return Promise.resolve(new Response("null", { status: 200 }));
    }
    return Promise.resolve(new Response("[]", { status: 200 }));
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const checkinRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/check-in",
    component: CheckInRoute.options.component,
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
    routeTree: rootRoute.addChildren([checkinRoute, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/check-in"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("CheckInPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("renderiza los 3 sliders y la nota", async () => {
    renderPage();
    expect(await screen.findByLabelText("Ánimo")).toBeInTheDocument();
    expect(screen.getByLabelText("Energía")).toBeInTheDocument();
    expect(screen.getByLabelText("Disciplina")).toBeInTheDocument();
    expect(screen.getByLabelText("Nota")).toBeInTheDocument();
  });

  it("al Guardar dispara un POST", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Guardar" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const posted = calls.some(
        ([url, opts]) => url === "/api/v1/checkins" && opts?.method === "POST"
      );
      expect(posted).toBe(true);
    });
  });

  it("pre-rellena el formulario con el check-in de hoy", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url.includes("/today")) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: "c1", date: "2026-06-10", mood: 8, energy: 7,
                discipline: 9, note: "gran día", created_at: "", updated_at: "",
              }),
              { status: 200 }
            )
          );
        }
        return Promise.resolve(new Response("[]", { status: 200 }));
      })
    );

    renderPage();

    const mood = (await screen.findByLabelText("Ánimo")) as HTMLInputElement;
    await waitFor(() => expect(mood.value).toBe("8"));
    expect((screen.getByLabelText("Energía") as HTMLInputElement).value).toBe("7");
    expect((screen.getByLabelText("Disciplina") as HTMLInputElement).value).toBe("9");
    expect((screen.getByLabelText("Nota") as HTMLTextAreaElement).value).toBe("gran día");
  });

  it("muestra el historial de check-ins", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        if (url.includes("/today")) {
          return Promise.resolve(new Response("null", { status: 200 }));
        }
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: "c1", date: "2026-06-09", mood: 4, energy: 5,
                discipline: 6, note: "", created_at: "", updated_at: "",
              },
            ]),
            { status: 200 }
          )
        );
      })
    );

    renderPage();

    expect(await screen.findByText("2026-06-09")).toBeInTheDocument();
    expect(screen.getByText("Á4 · E5 · D6")).toBeInTheDocument();
  });
});
