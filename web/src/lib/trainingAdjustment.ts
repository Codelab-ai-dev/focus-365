import { apiFetch } from "./api";

export type TrainingAdjustment = {
  scope: string;
  content: string;
  created_at: string;
};

export function getAdjustment(): Promise<TrainingAdjustment | null> {
  return apiFetch<TrainingAdjustment | null>("/api/v1/training/adjustment");
}

export function generateAdjustment(scope: "last" | "week"): Promise<TrainingAdjustment> {
  return apiFetch<TrainingAdjustment>("/api/v1/training/adjustment", {
    method: "POST",
    body: JSON.stringify({ scope }),
  });
}
