import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { setAccessToken } from "./api";
import { getToday, list, upsert, todayString } from "./checkins";

describe("lib checkins", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("getToday llama a GET /checkins/today con la fecha y el Bearer", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response("null", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getToday("2026-06-10");

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/checkins/today?date=2026-06-10");
    expect((opts.headers as Record<string, string>)["Authorization"]).toBe("Bearer tok");
    expect(res).toBeNull();
  });

  it("list llama a GET /checkins con limit", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await list(15);

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/checkins?limit=15");
  });

  it("upsert hace POST /checkins con el body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "c1" }), { status: 200 })
    );
    vi.stubGlobal("fetch", fetchMock);

    await upsert({
      date: "2026-06-10", mood: 7, energy: 6,
      espiritual: "oración", emocional: "calma", fisica: "gym",
      financiera: "ahorro", win: "cerré el deal", avoided: "redes",
      commitments: ["llamar a mamá", "leer 20 min"],
    });

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/checkins");
    expect(opts.method).toBe("POST");
    expect(JSON.parse(opts.body as string)).toEqual({
      date: "2026-06-10", mood: 7, energy: 6,
      espiritual: "oración", emocional: "calma", fisica: "gym",
      financiera: "ahorro", win: "cerré el deal", avoided: "redes",
      commitments: ["llamar a mamá", "leer 20 min"],
    });
  });

  it("todayString formatea la fecha local como YYYY-MM-DD", () => {
    const d = new Date(2026, 5, 9); // 9 de junio de 2026 (mes 0-indexado)
    expect(todayString(d)).toBe("2026-06-09");
  });
});
