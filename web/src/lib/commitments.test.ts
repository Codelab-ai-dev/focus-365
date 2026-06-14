import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { setAccessToken } from "./api";
import { getDue, toggle } from "./commitments";

describe("lib commitments", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("getDue hace GET con la fecha y devuelve el array de compromisos", async () => {
    const due = [
      { id: "c1", target_date: "2026-06-14", text: "tender la cama", done: false },
      { id: "c2", target_date: "2026-06-14", text: "pasear a Ruffo", done: true },
    ];
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ commitments: due }), { status: 200 })
      );
    vi.stubGlobal("fetch", fetchMock);

    const res = await getDue("2026-06-14");

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/commitments/due?date=2026-06-14");
    expect((opts.headers as Record<string, string>)["Authorization"]).toBe("Bearer tok");
    expect(res).toEqual(due);
  });

  it("toggle hace POST a /{id}/toggle y devuelve el compromiso", async () => {
    const commitment = {
      id: "c1",
      target_date: "2026-06-14",
      text: "tender la cama",
      done: true,
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ commitment }), { status: 200 })
      );
    vi.stubGlobal("fetch", fetchMock);

    const res = await toggle("c1");

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/commitments/c1/toggle");
    expect(opts.method).toBe("POST");
    expect(res).toEqual(commitment);
  });
});
