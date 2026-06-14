import { apiFetch } from "./api";

export type Commitment = {
  id: string;
  target_date: string;
  text: string;
  done: boolean;
};

export function getDue(date: string): Promise<Commitment[]> {
  return apiFetch<{ commitments: Commitment[] }>(
    `/api/v1/commitments/due?date=${encodeURIComponent(date)}`
  ).then((r) => r.commitments);
}

export function toggle(id: string): Promise<Commitment> {
  return apiFetch<{ commitment: Commitment }>(
    `/api/v1/commitments/${id}/toggle`,
    { method: "POST" }
  ).then((r) => r.commitment);
}
