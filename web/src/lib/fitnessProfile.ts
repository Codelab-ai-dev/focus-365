import { apiFetch } from "./api";

export type FitnessProfile = {
  birthdate: string | null;
  sex: string | null;
  height_cm: number | null;
  weight_grams: number | null;
  objective: string | null;
  location: string | null;
  level: string | null;
  weekly_days: number | null;
  equipment: string[];
  limitations: string;
  updated_at: string;
};

// Lo que se envía al guardar: todos los campos opcionales (null/ausente = limpiar).
export type FitnessProfileInput = {
  birthdate?: string | null;
  sex?: string | null;
  height_cm?: number | null;
  weight_grams?: number | null;
  objective?: string | null;
  location?: string | null;
  level?: string | null;
  weekly_days?: number | null;
  equipment?: string[];
  limitations?: string;
};

export function getProfile(): Promise<FitnessProfile | null> {
  return apiFetch<FitnessProfile | null>("/api/v1/training/profile");
}

export function saveProfile(input: FitnessProfileInput): Promise<FitnessProfile> {
  return apiFetch<FitnessProfile>("/api/v1/training/profile", {
    method: "PUT",
    body: JSON.stringify(input),
  });
}
