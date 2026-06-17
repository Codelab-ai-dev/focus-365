import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  listExercises,
  createExercise,
  createWorkout,
  listWorkouts,
  removeWorkout,
  kgToGrams,
  gramsToKg,
} from "./training";
import { setAccessToken } from "./api";

function okJson(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), { status }));
}

describe("lib/training", () => {
  beforeEach(() => setAccessToken(null));
  afterEach(() => vi.restoreAllMocks());

  it("convierte kg↔gramos (incl. 0.5 kg)", () => {
    expect(kgToGrams(80)).toBe(80000);
    expect(kgToGrams(0.5)).toBe(500);
    expect(gramsToKg(80000)).toBe(80);
    expect(gramsToKg(500)).toBe(0.5);
  });

  it("listExercises pega a GET /exercises", async () => {
    const fetchMock = vi.fn(() => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listExercises();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/training/exercises",
      expect.objectContaining({ })
    );
  });

  it("createExercise hace POST con el nombre", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "e1", name: "Sentadilla" }, 201)
    );
    vi.stubGlobal("fetch", fetchMock);
    await createExercise("Sentadilla");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/training/exercises");
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ name: "Sentadilla" });
  });

  it("createWorkout hace POST con el body y manda Bearer si hay token", async () => {
    setAccessToken("tok123");
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "w1" }, 201)
    );
    vi.stubGlobal("fetch", fetchMock);
    await createWorkout({
      date: "2026-06-11",
      type: "Fuerza",
      note: "",
      sets: [{ exercise: "Sentadilla", reps: 8, weight_grams: 80000, note: "" }],
    });
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/training/workouts");
    expect(opts?.method).toBe("POST");
    const headers = opts?.headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok123");
  });

  it("listWorkouts arma el querystring de rango", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listWorkouts("2026-06-01", "2026-06-30");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/training/workouts?from=2026-06-01&to=2026-06-30"
    );
  });

  it("removeWorkout hace DELETE a la sesión", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(null, { status: 204 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    await removeWorkout("w9");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/training/workouts/w9");
    expect(opts?.method).toBe("DELETE");
  });
});
