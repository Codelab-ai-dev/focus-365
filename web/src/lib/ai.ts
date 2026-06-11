import { apiFetch } from "./api";
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
