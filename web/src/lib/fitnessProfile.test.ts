import { describe, it, expect, vi, afterEach } from "vitest";
import { getProfile, saveProfile } from "./fitnessProfile";

afterEach(() => vi.restoreAllMocks());

describe("fitnessProfile", () => {
  it("getProfile devuelve null cuando el backend responde null", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response("null", { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(await getProfile()).toBeNull();
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/training/profile");
  });

  it("getProfile devuelve el perfil", async () => {
    const prof = { birthdate: "1990-05-01", sex: "masculino", height_cm: 178, weight_grams: 80500, objective: "hipertrofia", location: "casa", level: "intermedio", weekly_days: 4, equipment: ["mancuernas"], limitations: "", updated_at: "" };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify(prof), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    const p = await getProfile();
    expect(p?.objective).toBe("hipertrofia");
  });

  it("saveProfile hace PUT con el body", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ equipment: [], limitations: "", updated_at: "" }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    await saveProfile({ objective: "fuerza", weekly_days: 5, equipment: ["barra"], limitations: "" });
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("PUT");
    expect(String(opts.body)).toContain("fuerza");
  });
});
