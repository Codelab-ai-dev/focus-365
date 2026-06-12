import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthProvider, useAuth } from "./auth";
import { getAccessToken, setAccessToken } from "./api";

function Probe() {
  const { user, logout } = useAuth();
  return (
    <div>
      <span>{user ? `hola ${user.name}` : "anon"}</span>
      <button onClick={logout}>salir</button>
    </div>
  );
}

const userBody = {
  access_token: "tok-refrescado",
  user: { id: "u1", email: "g@focus.com", name: "Gus" },
};

describe("AuthProvider bootstrap de sesión", () => {
  beforeEach(() => setAccessToken(null));
  afterEach(() => vi.restoreAllMocks());

  it("restaura la sesión desde /auth/refresh al montar", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify(userBody), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    );

    expect(await screen.findByText("hola Gus")).toBeInTheDocument();
    expect(getAccessToken()).toBe("tok-refrescado");
    const [url, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/auth/refresh");
    expect(options.method).toBe("POST");
  });

  it("queda deslogueado si el refresh responde 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "sin refresh token" }), { status: 401 })
      )
    );

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    );

    expect(await screen.findByText("anon")).toBeInTheDocument();
    expect(getAccessToken()).toBeNull();
  });

  it("no renderiza children hasta que el bootstrap termina", async () => {
    let resolveFetch: (r: Response) => void = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn().mockReturnValue(new Promise<Response>((res) => (resolveFetch = res)))
    );

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    );

    expect(screen.queryByText("anon")).toBeNull();
    resolveFetch(new Response(JSON.stringify({ error: "x" }), { status: 401 }));
    expect(await screen.findByText("anon")).toBeInTheDocument();
  });

  it("logout llama a /auth/logout y limpia la sesión", async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(new Response(JSON.stringify(userBody), { status: 200 }));
      }
      return Promise.resolve(new Response(null, { status: 204 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    );
    await screen.findByText("hola Gus");

    await userEvent.click(screen.getByRole("button", { name: "salir" }));

    expect(await screen.findByText("anon")).toBeInTheDocument();
    expect(getAccessToken()).toBeNull();
    const logoutCall = fetchMock.mock.calls.find(([u]) => u === "/api/v1/auth/logout");
    expect(logoutCall).toBeDefined();
    expect((logoutCall![1] as RequestInit).method).toBe("POST");
  });
});
