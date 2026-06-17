import { apiFetch } from "./api";

export type Exercise = {
  id: string;
  name: string;
  created_at: string;
};

export type WorkoutSet = {
  exercise: string;
  reps: number | null;
  weight_grams: number | null;
  note: string;
};

export type Workout = {
  id: string;
  date: string; // YYYY-MM-DD
  type: string;
  note: string;
  sets: WorkoutSet[];
  created_at: string;
};

export type SetInput = {
  exercise: string;
  reps: number | null;
  weight_grams: number | null;
  note: string;
};

export type WorkoutInput = {
  date: string;
  type: string;
  note: string;
  sets: SetInput[];
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// El peso se guarda en gramos (entero) para evitar líos de coma flotante.
export function kgToGrams(kg: number): number {
  return Math.round(kg * 1000);
}

export function gramsToKg(grams: number): number {
  return grams / 1000;
}

export function listExercises(): Promise<Exercise[]> {
  return apiFetch<Exercise[]>("/api/v1/training/exercises");
}

export function createExercise(name: string): Promise<Exercise> {
  return apiFetch<Exercise>("/api/v1/training/exercises", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function createWorkout(input: WorkoutInput): Promise<Workout> {
  return apiFetch<Workout>("/api/v1/training/workouts", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function listWorkouts(from?: string, to?: string): Promise<Workout[]> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const qs = params.toString();
  return apiFetch<Workout[]>(`/api/v1/training/workouts${qs ? `?${qs}` : ""}`);
}

export function getWorkout(id: string): Promise<Workout> {
  return apiFetch<Workout>(`/api/v1/training/workouts/${id}`);
}

export function removeWorkout(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/training/workouts/${id}`, { method: "DELETE" });
}
