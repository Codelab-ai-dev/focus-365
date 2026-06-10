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
});
