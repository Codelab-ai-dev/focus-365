import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { setAccessToken } from "./api";
import {
  create,
  listByCycle,
  remove,
  summary,
  cycles,
  pesosToCents,
  centsToPesos,
} from "./finances";

describe("lib finances", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("create hace POST /finances/transactions con el body y el Bearer", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ id: "t1" }), { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);

    await create({
      type: "expense", amount: 12345, occurred_on: "2026-06-10",
      category: "luz", remark: "",
    });

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/finances/transactions");
    expect(opts.method).toBe("POST");
    expect((opts.headers as Record<string, string>)["Authorization"]).toBe("Bearer tok");
    expect(JSON.parse(opts.body as string).amount).toBe(12345);
  });

  it("listByCycle pide el ciclo dado y manda today", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await listByCycle("2026-06");

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/finances/transactions?cycle=2026-06");
    expect(url).toContain("today=");
  });

  it("listByCycle sin ciclo solo manda today (ciclo actual)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await listByCycle();

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/finances/transactions?today=");
    expect(url).not.toContain("cycle=");
  });

  it("remove hace DELETE con el id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await remove("t1");

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/finances/transactions/t1");
    expect(opts.method).toBe("DELETE");
  });

  it("summary y cycles pegan a sus rutas con today", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(() => Promise.resolve(new Response(JSON.stringify({}), { status: 200 })));
    vi.stubGlobal("fetch", fetchMock);

    await summary("2026-06");
    await cycles();

    expect(fetchMock.mock.calls[0][0]).toContain("/api/v1/finances/summary?cycle=2026-06");
    expect(fetchMock.mock.calls[1][0]).toContain("/api/v1/finances/cycles?today=");
  });

  it("convierte pesos a centavos y viceversa", () => {
    expect(pesosToCents(123.45)).toBe(12345);
    expect(centsToPesos(12345)).toBe(123.45);
  });
});
