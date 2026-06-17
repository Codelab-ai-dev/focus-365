import { describe, it, expect, vi, afterEach } from "vitest";
import { listGoalNotes, createGoalNote, deleteGoalNote } from "./goalNotes";

function okJson(data: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(data), { status }));
}

describe("goalNotes", () => {
  afterEach(() => vi.restoreAllMocks());

  it("listGoalNotes pega al endpoint de la meta", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ notes: [{ id: "n1", goal_id: "g1", note_date: "2026-06-17", body: "x", created_at: "" }] })
    );
    vi.stubGlobal("fetch", fetchMock);
    const notes = await listGoalNotes("g1");
    expect(notes).toHaveLength(1);
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/goals/g1/notes");
  });

  it("createGoalNote hace POST con note_date y body", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ note: { id: "n1", goal_id: "g1", note_date: "2026-06-17", body: "5k", created_at: "" } }, 201)
    );
    vi.stubGlobal("fetch", fetchMock);
    const note = await createGoalNote("g1", { note_date: "2026-06-17", body: "5k" });
    expect(note.body).toBe("5k");
    const opts = fetchMock.mock.calls[0][1]!;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain("2026-06-17");
  });

  it("deleteGoalNote hace DELETE a la nota", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(null, { status: 204 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    await deleteGoalNote("g1", "n1");
    const opts = fetchMock.mock.calls[0][1]!;
    expect(opts.method).toBe("DELETE");
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/goals/g1/notes/n1");
  });
});
