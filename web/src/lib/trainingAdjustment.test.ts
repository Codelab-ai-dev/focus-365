import { describe, it, expect, vi, afterEach } from "vitest";
import { getAdjustment, generateAdjustment } from "./trainingAdjustment";

afterEach(() => vi.restoreAllMocks());

describe("trainingAdjustment", () => {
  it("getAdjustment devuelve null cuando no hay", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response("null", { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(await getAdjustment()).toBeNull();
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/training/adjustment");
  });

  it("generateAdjustment hace POST con el scope", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ scope: "week", content: "ajustá", created_at: "" }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    const a = await generateAdjustment("week");
    expect(a.content).toBe("ajustá");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain("week");
  });
});
