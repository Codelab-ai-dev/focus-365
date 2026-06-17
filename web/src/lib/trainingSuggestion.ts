import { apiFetch } from "./api";

export type TrainingSuggestion = {
  focus: string;
  content: string;
  created_at: string;
};

export function getSuggestion(): Promise<TrainingSuggestion | null> {
  return apiFetch<TrainingSuggestion | null>("/api/v1/training/suggestion");
}

export function generateSuggestion(focus: string): Promise<TrainingSuggestion> {
  return apiFetch<TrainingSuggestion>("/api/v1/training/suggestion", {
    method: "POST",
    body: JSON.stringify({ focus }),
  });
}
