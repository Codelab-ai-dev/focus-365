import { apiFetch, getAccessToken, ApiError } from "./api";
import { todayString } from "./dashboard";

export type Insight = {
  content: string | null;
  available: boolean;
  generated_at: string | null;
};

export function getInsight(): Promise<Insight> {
  return apiFetch<Insight>(`/api/v1/ai/insight?today=${todayString()}`);
}

export type Message = {
  role: string;
  content: string;
  created_at: string;
};

export function getMessages(): Promise<Message[]> {
  return apiFetch<{ messages: Message[] }>("/api/v1/ai/messages").then(
    (r) => r.messages
  );
}

export function sendMessage(message: string): Promise<Message> {
  return apiFetch<{ reply: Message }>("/api/v1/ai/chat", {
    method: "POST",
    body: JSON.stringify({ message }),
  }).then((r) => r.reply);
}

// sendMessageStream envía el mensaje al endpoint SSE y entrega los deltas vía
// onDelta a medida que llegan. Resuelve con el reply persistido (evento done)
// o rechaza con ApiError (HTTP no-ok, evento error, o stream cortado) — en
// cuyo caso nada quedó persistido y el caller debe descartar el parcial.
export async function sendMessageStream(
  message: string,
  onDelta: (text: string) => void
): Promise<Message> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch("/api/v1/ai/chat/stream", {
    method: "POST",
    headers,
    body: JSON.stringify({ message }),
    credentials: "include",
  });

  if (!res.ok) {
    let msg = `Error ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* respuesta sin JSON */
    }
    throw new ApiError(msg, res.status);
  }
  if (!res.body) throw new ApiError("streaming no soportado", 500);

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let reply: Message | null = null;

  const handleEvent = (raw: string) => {
    let event = "";
    let data = "";
    for (const line of raw.split("\n")) {
      if (line.startsWith("event: ")) event = line.slice(7).trim();
      else if (line.startsWith("data: ")) data += line.slice(6);
    }
    if (!event || !data) return;
    if (event === "delta") {
      onDelta((JSON.parse(data) as { text: string }).text);
    } else if (event === "done") {
      reply = (JSON.parse(data) as { reply: Message }).reply;
    } else if (event === "error") {
      throw new ApiError((JSON.parse(data) as { error: string }).error, 503);
    }
  };

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const raw = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      handleEvent(raw);
    }
  }

  if (!reply) throw new ApiError("la respuesta se cortó, intenta de nuevo", 502);
  return reply;
}
