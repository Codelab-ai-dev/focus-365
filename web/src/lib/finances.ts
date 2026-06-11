import { apiFetch } from "./api";

export type TxType = "income" | "expense" | "transfer";

export type Transaction = {
  id: string;
  type: TxType;
  amount: number; // centavos
  occurred_on: string; // YYYY-MM-DD
  cycle: string; // YYYY-MM
  category: string;
  remark: string;
  source: string;
  created_at: string;
  updated_at: string;
};

export type TransactionInput = {
  type: TxType;
  amount: number; // centavos
  occurred_on: string;
  category: string;
  remark: string;
};

export type CycleSummary = {
  cycle: string; // YYYY-MM
  income: number;
  expense: number;
  net: number;
  status: "pendiente" | "verde" | "rojo";
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function create(input: TransactionInput): Promise<Transaction> {
  return apiFetch<Transaction>("/api/v1/finances/transactions", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// listByCycle lista las transacciones de un ciclo (YYYY-MM); sin ciclo usa el
// actual (el servidor lo deriva de today).
export function listByCycle(cycle?: string): Promise<Transaction[]> {
  const params = new URLSearchParams();
  if (cycle) params.set("cycle", cycle);
  params.set("today", todayString());
  return apiFetch<Transaction[]>(`/api/v1/finances/transactions?${params.toString()}`);
}

export function remove(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/finances/transactions/${id}`, { method: "DELETE" });
}

export function summary(cycle?: string): Promise<CycleSummary> {
  const params = new URLSearchParams();
  if (cycle) params.set("cycle", cycle);
  params.set("today", todayString());
  return apiFetch<CycleSummary>(`/api/v1/finances/summary?${params.toString()}`);
}

export function cycles(): Promise<CycleSummary[]> {
  return apiFetch<CycleSummary[]>(`/api/v1/finances/cycles?today=${todayString()}`);
}

export function pesosToCents(pesos: number): number {
  return Math.round(pesos * 100);
}

export function centsToPesos(cents: number): number {
  return cents / 100;
}

// formatMXN muestra centavos como moneda mexicana ($1,234.50).
export function formatMXN(cents: number): string {
  return new Intl.NumberFormat("es-MX", {
    style: "currency",
    currency: "MXN",
  }).format(cents / 100);
}
