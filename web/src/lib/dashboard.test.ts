import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { getDashboard, type Snapshot } from "./dashboard";
import { setAccessToken } from "./api";

const snap: Snapshot = {
  streak: { best_current: 12, done_today: 2, total: 4 },
  finance: { cycle: "2026-06", net: 320000, status: "verde" },
  checkin: { present: true, mood: 8, energy: 6, discipline: 9 },
  training: { trained_today: true, type: "Fuerza" },
  goals: { active: 3, avg_progress: 40, overdue: 1 },
  dimensions_active: 5,
};

function okJson(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(data), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })
  );
}

describe("getDashboard", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/dashboard con ?today=", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson(snap));
    vi.stubGlobal("fetch", fetchMock);

    const result = await getDashboard();

    expect(result.dimensions_active).toBe(5);
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toMatch(/^\/api\/v1\/dashboard\?today=\d{4}-\d{2}-\d{2}$/);
    const opts = fetchMock.mock.calls[0][1] as RequestInit | undefined;
    expect(opts?.method ?? "GET").toBe("GET");
  });

  it("acepta checkin null", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ ...snap, checkin: null, dimensions_active: 4 })
    );
    vi.stubGlobal("fetch", fetchMock);
    const result = await getDashboard();
    expect(result.checkin).toBeNull();
  });
});
