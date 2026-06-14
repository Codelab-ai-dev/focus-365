import { apiFetch } from "./api";

export type CheckIn = {
  id: string;
  date: string;
  mood: number;
  energy: number;
  espiritual: string;
  emocional: string;
  fisica: string;
  financiera: string;
  win: string;
  avoided: string;
  commitments: string[];
  created_at: string;
  updated_at: string;
};

export type CheckInInput = {
  date: string;
  mood: number;
  energy: number;
  espiritual: string;
  emocional: string;
  fisica: string;
  financiera: string;
  win: string;
  avoided: string;
  commitments: string[];
};

// getToday devuelve el check-in del día o null si no existe.
export function getToday(date: string): Promise<CheckIn | null> {
  return apiFetch<CheckIn | null>(
    `/api/v1/checkins/today?date=${encodeURIComponent(date)}`
  );
}

export function list(limit = 30): Promise<CheckIn[]> {
  return apiFetch<CheckIn[]>(`/api/v1/checkins?limit=${limit}`);
}

export function upsert(input: CheckInInput): Promise<CheckIn> {
  return apiFetch<CheckIn>("/api/v1/checkins", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(d = new Date()): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}
