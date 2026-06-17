import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
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

vi.mock("@/lib/fitnessProfile", () => ({
  getProfile: vi.fn(async () => ({
    birthdate: null,
    sex: null,
    height_cm: null,
    weight_grams: null,
    objective: "hipertrofia",
    location: "casa",
    level: null,
    weekly_days: 4,
    equipment: ["mancuernas"],
    limitations: "",
    updated_at: "",
  })),
  saveProfile: vi.fn(async () => ({ equipment: [], limitations: "", updated_at: "" })),
}));

vi.mock("@/lib/trainingSuggestion", () => ({
  getSuggestion: vi.fn(async () => null),
  generateSuggestion: vi.fn(async () => ({
    focus: "",
    content: "Hacé sentadillas 4x8",
    created_at: "",
  })),
}));

vi.mock("@/lib/trainingAdjustment", () => ({
  getAdjustment: vi.fn(async () => null),
  generateAdjustment: vi.fn(async () => ({
    scope: "last",
    content: "Subí 2.5 kg en sentadilla",
    created_at: "",
  })),
}));

import { Route as EntrenamientoRoute } from "./entrenamiento";
import { saveProfile } from "@/lib/fitnessProfile";

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
            sets: [{ exercise: "Sentadilla", reps: 8, weight_grams: 80000, note: "molestia rodilla" }],
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

  it("el form muestra un input de nota por serie", async () => {
    renderPage();
    expect(await screen.findByLabelText("Nota serie 1")).toBeInTheDocument();
  });

  it("el historial muestra la nota de una serie", async () => {
    renderPage();
    expect(await screen.findByText("molestia rodilla")).toBeInTheDocument();
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

  it("abre Mi perfil y precarga el objetivo", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Mi perfil" }));
    expect(await screen.findByLabelText("Objetivo")).toHaveValue("hipertrofia");
  });

  it("guardar llama saveProfile", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Mi perfil" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      expect(saveProfile).toHaveBeenCalled();
    });
  });

  it("Sugerir genera y muestra la sugerencia", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Sugerir" }));
    expect(await screen.findByText(/Hacé sentadillas 4x8/)).toBeInTheDocument();
  });

  it("Analizar genera y muestra el análisis del agente", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Analizar" }));
    expect(await screen.findByText(/Subí 2.5 kg en sentadilla/)).toBeInTheDocument();
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
