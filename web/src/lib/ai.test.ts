import { describe, it, expect, vi, afterEach } from "vitest";
import { getInsight, type Insight } from "./ai";

function okJson(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify(data), { status: 200 }));
}

describe("getInsight", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/ai/insight con el día de hoy", async () => {
    const insight: Insight = {
      content: "Aprovecha tu energía hoy.",
      available: true,
      generated_at: "2026-06-11T10:00:00Z",
    };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson(insight));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getInsight();
    expect(got).toEqual(insight);

    const url = fetchMock.mock.calls[0][0];
    expect(url).toMatch(/^\/api\/v1\/ai\/insight\?today=\d{4}-\d{2}-\d{2}$/);
    const opts = fetchMock.mock.calls[0][1];
    expect(opts?.method ?? "GET").toBe("GET");
  });
});
