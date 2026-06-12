import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";
import { AuthProvider } from "@/lib/auth";
import { Route as LoginRoute } from "./login";

function renderLogin() {
  const rootRoute = createRootRoute();
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: LoginRoute.options.component,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const registerRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/register",
    component: () => <div>register</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([loginRoute, homeRoute, registerRoute]),
    history: createMemoryHistory({ initialEntries: ["/login"] }),
  });
  render(
    <AuthProvider>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </AuthProvider>
  );
}

describe("LoginPage", () => {
  it("muestra los campos de email y contraseña", async () => {
    renderLogin();
    expect(await screen.findByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Contraseña")).toBeInTheDocument();
  });

  it("muestra error cuando el login falla", async () => {
    // Response fresca por llamada: el bootstrap de AuthProvider también
    // hace fetch y consumiría el body de una instancia compartida.
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "credenciales inválidas" }), { status: 401 })
        )
      )
    );
    renderLogin();
    await userEvent.type(await screen.findByLabelText("Email"), "x@y.com");
    await userEvent.type(screen.getByLabelText("Contraseña"), "bad");
    await userEvent.click(screen.getByRole("button", { name: "Entrar" }));
    expect(await screen.findByText("credenciales inválidas")).toBeInTheDocument();
    vi.restoreAllMocks();
  });
});
