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
            espiritual: "", emocional: "", fisica: "", financiera: "",
            win: "", avoided: "", commitments: [],
            created_at: "", updated_at: "",
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

  it("renderiza los sliders, las 4 dimensiones, win y evité", async () => {
    renderPage();
    expect(await screen.findByLabelText("Ánimo")).toBeInTheDocument();
    expect(screen.getByLabelText("Energía")).toBeInTheDocument();
    expect(screen.getByLabelText("Espiritual")).toBeInTheDocument();
    expect(screen.getByLabelText("Emocional")).toBeInTheDocument();
    expect(screen.getByLabelText("Física")).toBeInTheDocument();
    expect(screen.getByLabelText("Financiera")).toBeInTheDocument();
    expect(screen.getByLabelText("Win del día")).toBeInTheDocument();
    expect(screen.getByLabelText("Qué evité")).toBeInTheDocument();
    // No quedan rastros del check-in viejo.
    expect(screen.queryByLabelText("Disciplina")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Nota")).not.toBeInTheDocument();
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

  it("guarda el check-in completo con las 4 dimensiones y compromisos", async () => {
    renderPage();
    await userEvent.type(await screen.findByLabelText("Espiritual"), "reto");
    await userEvent.type(screen.getByLabelText("Win del día"), "win!");
    // agregar un compromiso
    await userEvent.click(
      screen.getByRole("button", { name: /agregar compromiso/i })
    );
    await userEvent.type(screen.getByLabelText("Compromiso 1"), "uno");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const posted = calls.find(
        ([u, o]) => u === "/api/v1/checkins" && (o as RequestInit)?.method === "POST"
      );
      expect(posted).toBeTruthy();
      const body = JSON.parse((posted![1] as RequestInit).body as string);
      expect(body.espiritual).toBe("reto");
      expect(body.win).toBe("win!");
      expect(body.commitments).toEqual(["uno"]);
    });
  });

  it("filtra los compromisos vacíos al guardar", async () => {
    renderPage();
    const addBtn = await screen.findByRole("button", { name: /agregar compromiso/i });
    await userEvent.click(addBtn);
    await userEvent.click(addBtn);
    await userEvent.type(screen.getByLabelText("Compromiso 1"), "solo este");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const posted = calls.find(
        ([u, o]) => u === "/api/v1/checkins" && (o as RequestInit)?.method === "POST"
      );
      expect(posted).toBeTruthy();
      const body = JSON.parse((posted![1] as RequestInit).body as string);
      expect(body.commitments).toEqual(["solo este"]);
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
                espiritual: "oré", emocional: "calma", fisica: "gym",
                financiera: "ahorré", win: "gran día", avoided: "redes",
                commitments: ["leer", "correr"],
                created_at: "", updated_at: "",
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
    expect((screen.getByLabelText("Espiritual") as HTMLInputElement).value).toBe("oré");
    expect((screen.getByLabelText("Emocional") as HTMLInputElement).value).toBe("calma");
    expect((screen.getByLabelText("Física") as HTMLInputElement).value).toBe("gym");
    expect((screen.getByLabelText("Financiera") as HTMLInputElement).value).toBe("ahorré");
    expect((screen.getByLabelText("Win del día") as HTMLInputElement).value).toBe("gran día");
    expect((screen.getByLabelText("Qué evité") as HTMLInputElement).value).toBe("redes");
    expect((screen.getByLabelText("Compromiso 1") as HTMLInputElement).value).toBe("leer");
    expect((screen.getByLabelText("Compromiso 2") as HTMLInputElement).value).toBe("correr");
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
                espiritual: "", emocional: "", fisica: "", financiera: "",
                win: "", avoided: "", commitments: [],
                created_at: "", updated_at: "",
              },
            ]),
            { status: 200 }
          )
        );
      })
    );

    renderPage();

    expect(await screen.findByText("2026-06-09")).toBeInTheDocument();
    expect(screen.getByText("Á4 · E5")).toBeInTheDocument();
  });
});
