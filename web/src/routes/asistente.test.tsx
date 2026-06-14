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

function sseBody(chunks: string[]) {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
}

function proposedMessages() {
  return {
    messages: [
      { id: "u1", role: "user", content: "registra mi check-in", created_at: "2026-06-11T10:00:00Z" },
      {
        id: "m1",
        role: "assistant",
        content: "Propongo registrar tu check-in de hoy: ánimo 8, energía 6, disciplina 9.",
        actions: [
          { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "proposed" },
        ],
        created_at: "2026-06-11T10:00:01Z",
      },
    ],
  };
}

it("una acción proposed muestra botones y confirmar la pasa a hecha", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/actions/a1/confirm" && opts?.method === "POST") {
      const action = { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "done" };
      return Promise.resolve(new Response(JSON.stringify({ action }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify(proposedMessages()), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  expect(await screen.findByRole("button", { name: "Confirmar" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "Confirmar" }));

  expect(await screen.findByText(/Hecha/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Confirmar" })).toBeNull();
});

it("cancelar deja la tarjeta como cancelada sin ejecutar", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/actions/a1/cancel" && opts?.method === "POST") {
      const action = { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "cancelled" };
      return Promise.resolve(new Response(JSON.stringify({ action }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify(proposedMessages()), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: "Cancelar" }));

  expect(await screen.findByText(/Cancelada/)).toBeInTheDocument();
  const confirmCalls = fetchMock.mock.calls.filter(([u]) => String(u).includes("/confirm"));
  expect(confirmCalls).toHaveLength(0);
});

it("un mensaje con dos acciones muestra dos tarjetas independientes", async () => {
  const twoActions = {
    messages: [
      {
        id: "m1",
        role: "assistant",
        content: "Propongo dos cosas.",
        actions: [
          { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "proposed" },
          { id: "a2", kind: "habito", payload: { habit_id: "h1" }, status: "proposed" },
        ],
        created_at: "2026-06-12T10:00:01Z",
      },
    ],
  };
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/actions/a1/confirm" && opts?.method === "POST") {
      const action = { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "done" };
      return Promise.resolve(new Response(JSON.stringify({ action }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify(twoActions), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  // Dos tarjetas: dos botones Confirmar al inicio.
  await waitFor(() => {
    expect(screen.getAllByRole("button", { name: "Confirmar" })).toHaveLength(2);
  });

  // Confirmar la primera.
  await userEvent.click(screen.getAllByRole("button", { name: "Confirmar" })[0]);

  // La primera pasa a Hecha, la segunda sigue con botones.
  expect(await screen.findByText(/Hecha/)).toBeInTheDocument();
  expect(screen.getAllByRole("button", { name: "Confirmar" })).toHaveLength(1);
});

it("una tarjeta done se puede deshacer y queda Deshecha", async () => {
  const doneMessages = {
    messages: [
      {
        id: "m1",
        role: "assistant",
        content: "Hecho.",
        actions: [
          { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "done" },
        ],
        created_at: "2026-06-12T10:00:01Z",
      },
    ],
  };
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/actions/a1/undo" && opts?.method === "POST") {
      const action = { id: "a1", kind: "checkin", payload: { mood: 8, energy: 6 }, status: "undone" };
      return Promise.resolve(new Response(JSON.stringify({ action }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify(doneMessages), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: "Deshacer" }));

  expect(await screen.findByText(/Deshecha/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Deshacer" })).toBeNull();
});

it("mensajes sin acción no muestran tarjeta ni botones", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            messages: [
              { id: "m2", role: "assistant", content: "Vas bien.", created_at: "2026-06-11T10:00:01Z" },
            ],
          }),
          { status: 200 }
        )
      )
    )
  );
  renderPage();
  expect(await screen.findByText("Vas bien.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Confirmar" })).toBeNull();
});

it("la tarjeta de entrenamiento muestra tipo y series", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            messages: [
              {
                id: "w1",
                role: "assistant",
                content: "Propongo registrar un entrenamiento.",
                actions: [
                  {
                    id: "a1",
                    kind: "entrenamiento",
                    payload: {
                      type: "fuerza",
                      sets: [
                        { exercise: "press banca", reps: 8, weight_kg: 60 },
                        { exercise: "plancha" },
                      ],
                    },
                    status: "proposed",
                  },
                ],
                created_at: "2026-06-12T10:00:01Z",
              },
            ],
          }),
          { status: 200 }
        )
      )
    )
  );
  renderPage();
  expect(await screen.findByText("Entrenamiento")).toBeInTheDocument();
  expect(screen.getByText(/fuerza/)).toBeInTheDocument();
  expect(screen.getByText(/press banca ×8 @60kg/)).toBeInTheDocument();
});

it("la tarjeta de hábito nuevo y meta nueva muestran sus datos", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            messages: [
              {
                id: "h1", role: "assistant", content: "x",
                actions: [{ id: "a1", kind: "habito_nuevo", payload: { name: "Leer 30 min", target_days: 21 }, status: "proposed" }],
                created_at: "2026-06-12T10:00:01Z",
              },
              {
                id: "g1", role: "assistant", content: "y",
                actions: [{ id: "a2", kind: "meta_nueva", payload: { title: "Ahorrar 50k", dimension: "finanzas", deadline: "2026-12-01" }, status: "proposed" }],
                created_at: "2026-06-12T10:00:02Z",
              },
            ],
          }),
          { status: 200 }
        )
      )
    )
  );
  renderPage();
  expect(await screen.findByText("Nuevo hábito")).toBeInTheDocument();
  expect(screen.getByText(/Leer 30 min · objetivo 21 días/)).toBeInTheDocument();
  expect(screen.getByText("Nueva meta")).toBeInTheDocument();
  expect(screen.getByText(/Ahorrar 50k · finanzas · para 2026-12-01/)).toBeInTheDocument();
});

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
                { id: "m1", role: "user", content: "¿cómo voy?", created_at: "2026-06-11T10:00:00Z" },
                { id: "m2", role: "assistant", content: "Vas verde.", created_at: "2026-06-11T10:00:01Z" },
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

  it("al enviar streamea la respuesta y muestra la burbuja creciendo", async () => {
    const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
      if (url === "/api/v1/ai/chat/stream" && opts?.method === "POST") {
        return Promise.resolve(
          new Response(
            sseBody([
              'event: delta\ndata: {"text":"Vas "}\n\n',
              'event: delta\ndata: {"text":"verde."}\n\n',
              'event: done\ndata: {"reply":{"role":"assistant","content":"Vas verde.","created_at":"2026-06-11T10:00:02Z"}}\n\n',
            ]),
            { status: 200, headers: { "Content-Type": "text/event-stream" } }
          )
        );
      }
      return Promise.resolve(new Response(JSON.stringify({ messages: [] }), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    const input = await screen.findByLabelText("Mensaje");
    await userEvent.type(input, "¿cómo voy?");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    // El POST fue al endpoint de streaming.
    await waitFor(() => {
      const posted = fetchMock.mock.calls.some(
        ([url, opts]) => url === "/api/v1/ai/chat/stream" && (opts as RequestInit)?.method === "POST"
      );
      expect(posted).toBe(true);
    });
    // La respuesta completa queda visible (burbuja streameada).
    expect(await screen.findByText("Vas verde.")).toBeInTheDocument();
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

  it("descarta el parcial y muestra error si el stream se corta", async () => {
    const fetchMock = vi.fn((_url: string, opts?: RequestInit) => {
      if (opts?.method === "POST") {
        return Promise.resolve(
          new Response(
            sseBody([
              'event: delta\ndata: {"text":"Vas "}\n\n',
              'event: error\ndata: {"error":"asistente no disponible por ahora"}\n\n',
            ]),
            { status: 200, headers: { "Content-Type": "text/event-stream" } }
          )
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
    // El parcial "Vas " no queda como burbuja fantasma.
    expect(screen.queryByText(/^Vas /)).toBeNull();
    // El input conserva el texto para reintentar.
    expect(input.value).toBe("hola");
  });
});
