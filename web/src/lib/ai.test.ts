import { describe, it, expect, vi, afterEach } from "vitest";
import { getInsight, getMessages, sendMessage, sendMessageStream, confirmAction, cancelAction, undoAction, importFile, getPendingUploads, type Insight, type Message, type Action } from "./ai";
import { ApiError } from "./api";

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

describe("getMessages", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/ai/messages y devuelve el array", async () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "hola", created_at: "2026-06-11T10:00:00Z" },
      { id: "m2", role: "assistant", content: "qué tal", created_at: "2026-06-11T10:00:01Z" },
    ];
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ messages })
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await getMessages();
    expect(got).toEqual(messages);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/ai/messages");
    const opts = fetchMock.mock.calls[0][1];
    expect(opts?.method ?? "GET").toBe("GET");
  });
});

describe("sendMessage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace POST a /api/v1/ai/chat con el mensaje y devuelve el reply", async () => {
    const reply: Message = {
      id: "m3",
      role: "assistant",
      content: "Vas verde este ciclo.",
      created_at: "2026-06-11T10:00:02Z",
    };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ reply })
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await sendMessage("¿cómo voy?");
    expect(got).toEqual(reply);

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/ai/chat");
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ message: "¿cómo voy?" });
  });
});

function sseResponse(chunks: string[], status = 200) {
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
  return Promise.resolve(
    new Response(stream, { status, headers: { "Content-Type": "text/event-stream" } })
  );
}

describe("sendMessageStream", () => {
  afterEach(() => vi.restoreAllMocks());

  const doneEvent =
    'event: done\ndata: {"reply":{"role":"assistant","content":"Vas bien.","created_at":"2026-06-11T10:00:02Z"}}\n\n';

  it("acumula deltas vía onDelta y resuelve con el reply del done", async () => {
    const fetchMock = vi.fn(() =>
      sseResponse([
        'event: delta\ndata: {"text":"Vas "}\n\n',
        'event: delta\ndata: {"text":"bien."}\n\n',
        doneEvent,
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const deltas: string[] = [];
    const reply = await sendMessageStream("¿cómo voy?", (d) => deltas.push(d));

    expect(deltas).toEqual(["Vas ", "bien."]);
    expect(reply.content).toBe("Vas bien.");
    const [url, opts] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/v1/ai/chat/stream");
    expect(opts.method).toBe("POST");
    expect(JSON.parse(opts.body as string)).toEqual({ message: "¿cómo voy?" });
  });

  it("reensambla un evento partido entre dos chunks", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        sseResponse(['event: delta\ndata: {"te', 'xt":"Hola"}\n\n', doneEvent])
      )
    );

    const deltas: string[] = [];
    await sendMessageStream("hola", (d) => deltas.push(d));
    expect(deltas).toEqual(["Hola"]);
  });

  it("rechaza con el mensaje del evento error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        sseResponse([
          'event: delta\ndata: {"text":"Vas "}\n\n',
          'event: error\ndata: {"error":"asistente no disponible por ahora"}\n\n',
        ])
      )
    );

    await expect(sendMessageStream("hola", () => {})).rejects.toThrowError(
      "asistente no disponible por ahora"
    );
  });

  it("rechaza con ApiError en HTTP no-ok (sin stream)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "asistente no disponible por ahora" }), {
            status: 503,
          })
        )
      )
    );

    const p = sendMessageStream("hola", () => {});
    await expect(p).rejects.toBeInstanceOf(ApiError);
    await expect(sendMessageStream("hola", () => {})).rejects.toThrowError(/no disponible/);
  });

  it("rechaza si el stream se cierra sin done ni error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => sseResponse(['event: delta\ndata: {"text":"Vas "}\n\n']))
    );

    await expect(sendMessageStream("hola", () => {})).rejects.toThrowError(/cortó/);
  });
});

describe("confirmAction / cancelAction / undoAction", () => {
  afterEach(() => vi.restoreAllMocks());

  const done: Action = {
    id: "a1",
    kind: "checkin",
    payload: { mood: 8, energy: 6 },
    status: "done",
  };

  it("confirmAction hace POST al endpoint y devuelve la acción", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ action: done }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await confirmAction("a1");
    expect(got).toEqual(done);
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/ai/actions/a1/confirm");
    expect(opts?.method).toBe("POST");
  });

  it("cancelAction hace POST al endpoint de cancelar", async () => {
    const cancelled: Action = { ...done, status: "cancelled" };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ action: cancelled }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await cancelAction("a1");
    expect(got.status).toBe("cancelled");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/ai/actions/a1/cancel");
  });

  it("undoAction hace POST al endpoint de deshacer y devuelve la acción undone", async () => {
    const undone: Action = { ...done, status: "undone" };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ action: undone }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await undoAction("a1");
    expect(got.status).toBe("undone");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/ai/actions/a1/undo");
    expect(opts?.method).toBe("POST");
  });

  it("confirmAction propaga el error del backend (409)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "la acción ya fue resuelta" }), { status: 409 })
        )
      )
    );
    await expect(confirmAction("m1")).rejects.toThrowError("la acción ya fue resuelta");
  });
});

describe("importFile", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace POST multipart a /api/v1/ai/import y devuelve {created, dropped, truncated}", async () => {
    const created: Action[] = [
      { id: "a1", kind: "movimiento", payload: { type: "expense", amount_centavos: 12500, category: "comida" }, status: "proposed" },
    ];
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ created, dropped: 0, truncated: false })
    );
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["x,y\n1,2\n"], "ticket.csv", { type: "text/csv" });
    const got = await importFile(file);
    expect(got).toEqual({ created, dropped: 0, truncated: false });

    const [url, opts] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/v1/ai/import");
    expect(opts.method).toBe("POST");
    expect(opts.body).toBeInstanceOf(FormData);
  });

  it("propaga 422 como ApiError con el mensaje del backend", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "no se pudo leer el archivo" }), { status: 422 })
        )
      )
    );
    const file = new File(["x"], "raro.pdf", { type: "application/pdf" });
    const p = importFile(file);
    await expect(p).rejects.toBeInstanceOf(ApiError);
    await expect(importFile(file)).rejects.toThrowError("no se pudo leer el archivo");
  });
});

describe("getPendingUploads", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/ai/import/pending y devuelve el array", async () => {
    const actions: Action[] = [
      { id: "a1", kind: "movimiento", payload: { type: "expense", amount_centavos: 12500, category: "comida" }, status: "proposed" },
      { id: "a2", kind: "movimiento", payload: { type: "income", amount_centavos: 50000, category: "sueldo" }, status: "proposed" },
    ];
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson({ actions }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getPendingUploads();
    expect(got).toEqual(actions);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/ai/import/pending");
    const opts = fetchMock.mock.calls[0][1];
    expect(opts?.method ?? "GET").toBe("GET");
  });
});
