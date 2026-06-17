import { describe, it, expect, vi, afterEach } from "vitest";
import { getSuggestion, generateSuggestion } from "./trainingSuggestion";

afterEach(() => vi.restoreAllMocks());

describe("trainingSuggestion", () => {
  it("getSuggestion devuelve null cuando no hay", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response("null", { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(await getSuggestion()).toBeNull();
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/training/suggestion");
  });

  it("generateSuggestion hace POST con el enfoque", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ focus: "pierna", content: "rutina", created_at: "" }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    const s = await generateSuggestion("pierna");
    expect(s.content).toBe("rutina");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain("pierna");
  });
});
