import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  listHabits,
  createHabit,
  checkHabit,
  archiveHabit,
  removeHabit,
  todayString,
  yesterdayString,
} from "./habits";
import { setAccessToken } from "./api";

function okJson(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), { status }));
}

describe("lib/habits", () => {
  beforeEach(() => setAccessToken(null));
  afterEach(() => vi.restoreAllMocks());

  it("yesterdayString es el día anterior a today", () => {
    const base = new Date(2026, 5, 12); // 2026-06-12 local
    expect(todayString(base)).toBe("2026-06-12");
    expect(yesterdayString(base)).toBe("2026-06-11");
  });

  it("listHabits pega a GET /habits con today y sin archived", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listHabits();
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url.startsWith("/api/v1/habits?")).toBe(true);
    expect(url).toContain(`today=${todayString()}`);
    expect(url).not.toContain("archived");
  });

  it("listHabits(true) agrega archived=true", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listHabits(true);
    expect(fetchMock.mock.calls[0][0] as string).toContain("archived=true");
  });

  it("createHabit hace POST con el body y manda Bearer si hay token", async () => {
    setAccessToken("tok123");
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "h1" }, 201)
    );
    vi.stubGlobal("fetch", fetchMock);
    await createHabit({ name: "Leer", target_days: 21 });
    const [url, opts] = fetchMock.mock.calls[0];
    expect((url as string).startsWith("/api/v1/habits?today=")).toBe(true);
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ name: "Leer", target_days: 21 });
    const headers = opts?.headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok123");
  });

  it("checkHabit arma el body con day y done", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "h1" }, 200)
    );
    vi.stubGlobal("fetch", fetchMock);
    await checkHabit("h1", "2026-06-12", true);
    const [url, opts] = fetchMock.mock.calls[0];
    expect((url as string).startsWith("/api/v1/habits/h1/check?today=")).toBe(true);
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ day: "2026-06-12", done: true });
  });

  it("archiveHabit hace POST a /archive", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "h1" }, 200)
    );
    vi.stubGlobal("fetch", fetchMock);
    await archiveHabit("h1");
    const [url, opts] = fetchMock.mock.calls[0];
    expect((url as string).startsWith("/api/v1/habits/h1/archive?today=")).toBe(true);
    expect(opts?.method).toBe("POST");
  });

  it("removeHabit hace DELETE al hábito", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(null, { status: 204 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    await removeHabit("h9");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/habits/h9");
    expect(opts?.method).toBe("DELETE");
  });
});
