import { describe, it, expect, vi, afterEach } from "vitest";
import { getInsight, getMessages, sendMessage, sendMessageStream, type Insight, type Message } from "./ai";
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
      { role: "user", content: "hola", created_at: "2026-06-11T10:00:00Z" },
      { role: "assistant", content: "qué tal", created_at: "2026-06-11T10:00:01Z" },
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
