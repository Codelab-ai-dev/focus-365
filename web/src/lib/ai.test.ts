import { describe, it, expect, vi, afterEach } from "vitest";
import { getInsight, getMessages, sendMessage, type Insight, type Message } from "./ai";

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
