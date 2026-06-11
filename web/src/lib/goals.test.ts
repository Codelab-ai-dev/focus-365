import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { listGoals, createGoal, patchGoal, deleteGoal } from "./goals";
import { setAccessToken } from "./api";

const okJson = (data: unknown) =>
  Promise.resolve(
    new Response(JSON.stringify(data), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );

describe("goals lib", () => {
  beforeEach(() => {
    setAccessToken("tok");
  });
  afterEach(() => {
    vi.restoreAllMocks();
    setAccessToken(null);
  });

  it("listGoals arma ?status= y ?today=", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listGoals("done");
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/goals");
    expect(url).toContain("status=done");
    expect(url).toContain("today=");
  });

  it("createGoal hace POST con body", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "1" }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await createGoal({ title: "X", dimension: "general", deadline: null });
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain('"title":"X"');
  });

  it("patchGoal hace PATCH con subset", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "1" }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await patchGoal("1", { progress: 50 });
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("PATCH");
    expect(String(opts.body)).toContain('"progress":50');
  });

  it("deleteGoal hace DELETE", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(null, { status: 204 })),
    );
    vi.stubGlobal("fetch", fetchMock);
    await deleteGoal("1");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("DELETE");
  });
});
