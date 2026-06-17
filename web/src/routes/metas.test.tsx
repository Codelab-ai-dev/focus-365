import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
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

vi.mock("@/lib/goalNotes", () => ({
  listGoalNotes: vi.fn(async () => [
    { id: "n1", goal_id: "1", note_date: "2026-06-17", body: "5k hoy", created_at: "" },
  ]),
  createGoalNote: vi.fn(async () => ({
    id: "n2",
    goal_id: "1",
    note_date: "2026-06-18",
    body: "nueva",
    created_at: "",
  })),
  deleteGoalNote: vi.fn(async () => undefined),
}));

import { createGoalNote } from "@/lib/goalNotes";
import { Route as MetasRoute } from "./metas";

type GoalOverride = {
  id?: string;
  title?: string;
  dimension?: string;
  status?: string;
  progress?: number;
  deadline?: string | null;
  overdue?: boolean;
  created_at?: string;
};

function makeGoal(overrides: GoalOverride = {}) {
  return {
    id: "1",
    title: "Correr 10k",
    dimension: "fisica",
    status: "active",
    progress: 30,
    deadline: null,
    overdue: false,
    created_at: "2026-06-11T00:00:00Z",
    ...overrides,
  };
}

function fetchMock(goals: ReturnType<typeof makeGoal>[]) {
  return vi.fn((_url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "g9" }), { status: 201 }));
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (opts?.method === "PATCH") {
      return Promise.resolve(new Response(JSON.stringify(goals[0]), { status: 200 }));
    }
    // GET /goals
    return Promise.resolve(
      new Response(JSON.stringify(goals), { status: 200 })
    );
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: "/metas",
    component: MetasRoute.options.component,
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
    history: createMemoryHistory({ initialEntries: ["/metas"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("MetasPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock([makeGoal()])));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el título de la meta", async () => {
    renderPage();
    expect(await screen.findByText("Correr 10k")).toBeInTheDocument();
  });

  it("cambiar el slider dispara un PATCH con progress:80", async () => {
    renderPage();
    const slider = await screen.findByRole("slider", { name: "Progreso Correr 10k" });
    await userEvent.type(slider, "{arrowRight}");
    // Disparamos el cambio con fireEvent para simular onChange con valor 80
    fireEvent.change(slider, { target: { value: "80" } });
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const patch = calls.find(
        ([url, opts]) =>
          (url as string).includes("/api/v1/goals/1") && opts?.method === "PATCH"
      );
      expect(patch).toBeTruthy();
      const body = JSON.parse(patch![1].body as string);
      expect(body.progress).toBe(80);
    });
  });

  it("muestra 'Vencida' cuando overdue es true", async () => {
    vi.stubGlobal(
      "fetch",
      fetchMock([makeGoal({ deadline: "2026-01-01", overdue: true })])
    );
    renderPage();
    expect(await screen.findByText(/Vencida/)).toBeInTheDocument();
  });

  it("el selector de dimensión ofrece las 4D y NO las viejas", async () => {
    renderPage();
    // Esperar a que la página cargue
    expect(await screen.findByText("Correr 10k")).toBeInTheDocument();
    // Las 4 opciones nuevas deben estar presentes
    expect(screen.getByRole("option", { name: "Espiritual" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Emocional" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Física" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Financiera" })).toBeInTheDocument();
    // Las dimensiones viejas NO deben aparecer como opciones
    expect(screen.queryByRole("option", { name: "general" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "checkin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "entrenamiento" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "finanzas" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "mente" })).not.toBeInTheDocument();
  });

  it("al crear una meta envía el valor en minúscula (sin acento)", async () => {
    let capturedBody: Record<string, unknown> | null = null;
    vi.stubGlobal("fetch", vi.fn((_url: string, opts?: RequestInit) => {
      if (opts?.method === "POST") {
        capturedBody = JSON.parse(opts.body as string);
        return Promise.resolve(new Response(JSON.stringify({ id: "g9" }), { status: 201 }));
      }
      return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }));
    }));
    renderPage();
    const select = await screen.findByLabelText("Dimensión");
    await userEvent.selectOptions(select, "fisica");
    await userEvent.type(screen.getByLabelText("Título de la meta"), "Correr 5k");
    await userEvent.click(screen.getByRole("button", { name: /crear meta/i }));
    await waitFor(() => expect(capturedBody).not.toBeNull());
    expect(capturedBody!.dimension).toBe("fisica");
  });

  it("el chip de una meta con dimension 'financiera' muestra la etiqueta 'Financiera'", async () => {
    vi.stubGlobal(
      "fetch",
      fetchMock([makeGoal({ dimension: "financiera", title: "Ahorrar" })])
    );
    renderPage();
    expect(await screen.findByText("Ahorrar")).toBeInTheDocument();
    // El chip debe mostrar la etiqueta capitalizada, no el valor en crudo.
    // Usamos getAllByText porque también existe como opción en el <select>.
    const matches = screen.getAllByText("Financiera");
    expect(matches.length).toBeGreaterThanOrEqual(1);
    // Al menos uno de los elementos debe ser el chip (span con role implícito)
    const chip = matches.find((el) => el.tagName === "SPAN");
    expect(chip).toBeTruthy();
  });

  it("abre el modal de notas y lista las notas de la meta", async () => {
    renderPage();
    await userEvent.click(await screen.findByLabelText("Notas de Correr 10k"));
    expect(await screen.findByText("5k hoy")).toBeInTheDocument();
  });

  it("agregar una nota llama a createGoalNote", async () => {
    renderPage();
    await userEvent.click(await screen.findByLabelText("Notas de Correr 10k"));
    await screen.findByText("5k hoy");
    await userEvent.type(screen.getByLabelText("Nueva nota"), "avance");
    await userEvent.click(screen.getByRole("button", { name: /agregar/i }));
    await waitFor(() => expect(createGoalNote).toHaveBeenCalled());
  });
});
