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
