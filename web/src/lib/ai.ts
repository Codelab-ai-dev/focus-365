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

export type Action = {
  id: string;
  kind: string;
  payload: Record<string, unknown>;
  status: "proposed" | "done" | "cancelled" | "undone";
};

export type Message = {
  id: string;
  role: string;
  content: string;
  actions?: Action[];
  created_at: string;
};

export type Thread = {
  id: string;
  title: string;
  preview: string;
  updated_at: string;
};

export function getThreads(): Promise<Thread[]> {
  return apiFetch<{ threads: Thread[] }>("/api/v1/ai/threads").then((r) => r.threads);
}

export function getThreadMessages(threadId: string): Promise<Message[]> {
  return apiFetch<{ messages: Message[] }>(
    `/api/v1/ai/threads/${threadId}/messages`
  ).then((r) => r.messages);
}

export function renameThread(id: string, title: string): Promise<Thread> {
  return apiFetch<{ thread: Thread }>(`/api/v1/ai/threads/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ title }),
  }).then((r) => r.thread);
}

export function deleteThread(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/ai/threads/${id}`, { method: "DELETE" });
}

export type ImportResult = { created: Action[]; dropped: number; truncated: boolean };

export async function importFile(file: File): Promise<ImportResult> {
  const headers: Record<string, string> = {};
  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const form = new FormData();
  form.append("file", file);
  const res = await fetch("/api/v1/ai/import", {
    method: "POST",
    headers,
    body: form,
    credentials: "include",
  });
  if (!res.ok) {
    let msg = `Error ${res.status}`;
    try {
      const b = await res.json();
      if (b?.error) msg = b.error;
    } catch {
      /* respuesta sin JSON */
    }
    throw new ApiError(msg, res.status);
  }
  return (await res.json()) as ImportResult;
}

export function getPendingUploads(): Promise<Action[]> {
  return apiFetch<{ actions: Action[] }>("/api/v1/ai/import/pending").then(
    (r) => r.actions
  );
}

export function confirmAction(id: string): Promise<Action> {
  return apiFetch<{ action: Action }>(`/api/v1/ai/actions/${id}/confirm`, {
    method: "POST",
  }).then((r) => r.action);
}

export function cancelAction(id: string): Promise<Action> {
  return apiFetch<{ action: Action }>(`/api/v1/ai/actions/${id}/cancel`, {
    method: "POST",
  }).then((r) => r.action);
}

export function undoAction(id: string): Promise<Action> {
  return apiFetch<{ action: Action }>(`/api/v1/ai/actions/${id}/undo`, {
    method: "POST",
  }).then((r) => r.action);
}

// sendMessageStream envía el mensaje al endpoint SSE y entrega los deltas vía
// onDelta a medida que llegan. Resuelve con el reply persistido y el threadId
// (evento done) o rechaza con ApiError (HTTP no-ok, evento error, o stream
// cortado) — en cuyo caso nada quedó persistido y el caller debe descartar el
// parcial.
export async function sendMessageStream(
  message: string,
  threadId: string | undefined,
  onDelta: (text: string) => void
): Promise<{ reply: Message; threadId: string }> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch("/api/v1/ai/chat/stream", {
    method: "POST",
    headers,
    body: JSON.stringify(threadId ? { message, thread_id: threadId } : { message }),
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
  let doneThreadId = "";

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
      const d = JSON.parse(data) as { reply: Message; thread_id: string };
      reply = d.reply;
      doneThreadId = d.thread_id;
    } else if (event === "error") {
      throw new ApiError((JSON.parse(data) as { error: string }).error, 503);
    }
  };

  try {
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
  } finally {
    // Ante un throw temprano (evento error), cierra la conexión en vez de
    // dejar que el stream siga hasta que el GC lo recoja.
    await reader.cancel().catch(() => {});
  }

  if (!reply) throw new ApiError("la respuesta se cortó, intenta de nuevo", 502);
  return { reply, threadId: doneThreadId };
}
