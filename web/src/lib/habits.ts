import { apiFetch } from "./api";

export type Habit = {
  id: string;
  name: string;
  target_days: number | null;
  current_streak: number;
  best_streak: number;
  done_today: boolean;
  done_yesterday: boolean;
  archived_at: string | null;
  created_at: string;
};

export type HabitInput = {
  name: string;
  target_days: number | null;
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// yesterdayString es el día anterior a la fecha dada (ventana de gracia).
export function yesterdayString(date = new Date()): string {
  const d = new Date(date);
  d.setDate(d.getDate() - 1);
  return todayString(d);
}

export function listHabits(archived = false): Promise<Habit[]> {
  const params = new URLSearchParams();
  params.set("today", todayString());
  if (archived) params.set("archived", "true");
  return apiFetch<Habit[]>(`/api/v1/habits?${params.toString()}`);
}

export function createHabit(input: HabitInput): Promise<Habit> {
  return apiFetch<Habit>(`/api/v1/habits?today=${todayString()}`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function checkHabit(id: string, day: string, done: boolean): Promise<Habit> {
  return apiFetch<Habit>(`/api/v1/habits/${id}/check?today=${todayString()}`, {
    method: "POST",
    body: JSON.stringify({ day, done }),
  });
}

export function archiveHabit(id: string): Promise<Habit> {
  return apiFetch<Habit>(`/api/v1/habits/${id}/archive?today=${todayString()}`, {
    method: "POST",
  });
}

export function removeHabit(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/habits/${id}`, { method: "DELETE" });
}
