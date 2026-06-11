import { apiFetch } from "./api";

export type GoalStatus = "active" | "done" | "paused";

export type Goal = {
  id: string;
  title: string;
  dimension: string;
  status: GoalStatus;
  progress: number;
  deadline: string | null;
  overdue: boolean;
  created_at: string;
};

export type GoalInput = {
  title: string;
  dimension: string;
  deadline: string | null;
};

export type GoalPatch = {
  title?: string;
  dimension?: string;
  status?: GoalStatus;
  progress?: number;
  deadline?: string | null;
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function listGoals(status: GoalStatus = "active"): Promise<Goal[]> {
  const params = new URLSearchParams();
  params.set("status", status);
  params.set("today", todayString());
  return apiFetch<Goal[]>(`/api/v1/goals?${params.toString()}`);
}

export function createGoal(input: GoalInput): Promise<Goal> {
  return apiFetch<Goal>(`/api/v1/goals?today=${todayString()}`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function patchGoal(id: string, patch: GoalPatch): Promise<Goal> {
  return apiFetch<Goal>(`/api/v1/goals/${id}?today=${todayString()}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function deleteGoal(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/goals/${id}`, { method: "DELETE" });
}
