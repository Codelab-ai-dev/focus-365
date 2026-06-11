import { apiFetch } from "./api";

export type StreakView = {
  best_current: number;
  done_today: number;
  total: number;
};

export type FinanceView = {
  cycle: string; // YYYY-MM
  net: number; // centavos
  status: "pendiente" | "verde" | "rojo";
};

export type CheckinView = {
  present: boolean;
  mood: number;
  energy: number;
  discipline: number;
};

export type TrainingView = {
  trained_today: boolean;
  type: string;
};

export type GoalsView = {
  active: number;
  avg_progress: number;
  overdue: number;
};

export type Snapshot = {
  streak: StreakView;
  finance: FinanceView;
  checkin: CheckinView | null;
  training: TrainingView;
  goals: GoalsView;
  dimensions_active: number;
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function getDashboard(): Promise<Snapshot> {
  return apiFetch<Snapshot>(`/api/v1/dashboard?today=${todayString()}`);
}
