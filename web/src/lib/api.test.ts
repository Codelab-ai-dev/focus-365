import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { setAccessToken, getAccessToken, apiFetch } from "./api";

describe("token store", () => {
  beforeEach(() => setAccessToken(null));

  it("guarda y lee el access token", () => {
    expect(getAccessToken()).toBeNull();
    setAccessToken("abc");
    expect(getAccessToken()).toBe("abc");
  });
});

describe("apiFetch", () => {
  afterEach(() => vi.restoreAllMocks());

  it("agrega el header Authorization cuando hay token", async () => {
    setAccessToken("tok123");
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/v1/health");

    const headers = (fetchMock.mock.calls[0][1] as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok123");
  });

  it("lanza ApiError en respuesta no-ok", async () => {
    setAccessToken(null);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "boom" }), { status: 400 })
      )
    );
    await expect(apiFetch("/x")).rejects.toThrowError("boom");
  });

  it("no agrega Authorization sin token", async () => {
    setAccessToken(null);
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/v1/data");

    const headers = (fetchMock.mock.calls[0][1] as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBeUndefined();
  });

  it("envía credentials: include para la cookie de refresh", async () => {
    setAccessToken(null);
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/v1/data");

    const options = fetchMock.mock.calls[0][1] as RequestInit;
    expect(options.credentials).toBe("include");
  });

  it("retorna undefined para 204 No Content", async () => {
    setAccessToken(null);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    );
    const result = await apiFetch("/api/v1/create");
    expect(result).toBeUndefined();
  });

  it("ante 401 refresca el token y reintenta una vez", async () => {
    setAccessToken("viejo");
    let recurso = 0;
    const fetchMock = vi.fn((url: string, _opts?: RequestInit) => {
      if (String(url).includes("/auth/refresh")) {
        return Promise.resolve(
          new Response(JSON.stringify({ access_token: "nuevo", user: { id: "u" } }), { status: 200 })
        );
      }
      recurso++;
      if (recurso === 1) {
        return Promise.resolve(new Response(JSON.stringify({ error: "no autorizado" }), { status: 401 }));
      }
      return Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await apiFetch<{ ok: boolean }>("/api/v1/habits", { method: "POST", body: "{}" });
    expect(result).toEqual({ ok: true });
    expect(getAccessToken()).toBe("nuevo");
    // hubo exactamente una llamada al refresh
    expect(fetchMock.mock.calls.filter((c) => String(c[0]).includes("/auth/refresh"))).toHaveLength(1);
    // el reintento al recurso llevó el token nuevo
    const recursoCalls = fetchMock.mock.calls.filter((c) => String(c[0]).includes("/habits"));
    expect(recursoCalls).toHaveLength(2);
    const retryHeaders = (recursoCalls[1][1] as RequestInit).headers as Record<string, string>;
    expect(retryHeaders["Authorization"]).toBe("Bearer nuevo");
  });

  it("no intenta refrescar ante un 401 de un endpoint de auth", async () => {
    setAccessToken(null);
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: "credenciales inválidas" }), { status: 401 })
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(apiFetch("/api/v1/auth/login", { method: "POST", body: "{}" })).rejects.toThrowError(
      "credenciales inválidas"
    );
    expect(fetchMock).toHaveBeenCalledTimes(1); // sin refresh, sin reintento
  });

  it("si el refresh falla, propaga el 401 original", async () => {
    setAccessToken("viejo");
    const fetchMock = vi.fn((url: string) => {
      if (String(url).includes("/auth/refresh")) {
        return Promise.resolve(new Response(JSON.stringify({ error: "refresh inválido" }), { status: 401 }));
      }
      return Promise.resolve(new Response(JSON.stringify({ error: "no autorizado" }), { status: 401 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      apiFetch("/api/v1/habits", { method: "POST", body: "{}" })
    ).rejects.toMatchObject({ status: 401 });
  });
});
