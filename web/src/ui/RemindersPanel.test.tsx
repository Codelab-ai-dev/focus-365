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
import { todayString } from "@/lib/dashboard";

const toggleSpy = vi.fn((_id: string) =>
  Promise.resolve({ id: "h1", target_date: todayString(), text: "De hoy", done: true })
);
const getPendingSpy = vi.fn();

vi.mock("@/lib/commitments", () => ({
  getPendingCommitments: () => getPendingSpy(),
  toggle: (id: string) => toggleSpy(id),
}));

import { RemindersPanel } from "./RemindersPanel";

const TODAY = todayString();
const AYER = todayString(new Date(Date.now() - 24 * 60 * 60 * 1000));

function renderPanel() {
  const rootRoute = createRootRoute({ component: RemindersPanel });
  const checkinRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/check-in",
    component: () => <div>check-in</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([checkinRoute]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("RemindersPanel", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("muestra grupos de vencidos y hoy", async () => {
    getPendingSpy.mockResolvedValue([
      { id: "v1", target_date: AYER, text: "Vencido", done: false },
      { id: "h1", target_date: TODAY, text: "De hoy", done: false },
    ]);
    renderPanel();
    expect(await screen.findByText("Vencido")).toBeInTheDocument();
    expect(screen.getByText("De hoy")).toBeInTheDocument();
    expect(screen.getByText("Vencidos (1)")).toBeInTheDocument();
    expect(screen.getByText("Hoy")).toBeInTheDocument();
  });

  it("no renderiza nada si no hay pendientes", async () => {
    getPendingSpy.mockResolvedValue([]);
    renderPanel();
    await waitFor(() => {
      expect(screen.queryByText(/Recordatorios/i)).not.toBeInTheDocument();
    });
  });

  it("al marcar el check dispara toggle", async () => {
    getPendingSpy.mockResolvedValue([
      { id: "h1", target_date: TODAY, text: "De hoy", done: false },
    ]);
    renderPanel();
    const check = await screen.findByRole("checkbox", { name: /De hoy/i });
    await userEvent.click(check);
    await waitFor(() => expect(toggleSpy).toHaveBeenCalledWith("h1"));
  });
});
