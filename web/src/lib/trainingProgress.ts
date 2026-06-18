import { gramsToKg } from "./training";
import type { Workout } from "./training";

export type ChartPoint = { label: string; value: number };
export type PersonalRecord = { exercise: string; weightKg: number };

// parseLocalDate interpreta "YYYY-MM-DD" como fecha LOCAL (sin corrimiento UTC).
function parseLocalDate(iso: string): Date {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, m - 1, d);
}

// mondayOf devuelve el lunes (medianoche local) de la semana que contiene d.
function mondayOf(d: Date): Date {
  const day = d.getDay(); // 0=Dom .. 6=Sáb
  const shift = day === 0 ? -6 : 1 - day;
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + shift);
}

// shortDate: "16/6".
function shortDate(d: Date): string {
  return `${d.getDate()}/${d.getMonth() + 1}`;
}

// volumeOf: Σ reps × peso(kg) de las series de una sesión.
function volumeOf(workout: Workout): number {
  let total = 0;
  for (const s of workout.sets) {
    if (s.reps != null && s.weight_grams != null) {
      total += s.reps * gramsToKg(s.weight_grams);
    }
  }
  return total;
}

// weeklyAgg agrupa por semana (lunes) y arma las últimas `weeks` semanas
// (continuas, las vacías en 0) terminando en la semana del entreno más reciente.
function weeklyAgg(workouts: Workout[], weeks: number, valueOf: (w: Workout) => number): ChartPoint[] {
  if (workouts.length === 0) return [];
  const byWeek = new Map<number, number>();
  let maxTime = -Infinity;
  for (const wk of workouts) {
    const monday = mondayOf(parseLocalDate(wk.date));
    const t = monday.getTime();
    byWeek.set(t, (byWeek.get(t) ?? 0) + valueOf(wk));
    if (t > maxTime) maxTime = t;
  }
  const end = new Date(maxTime);
  const out: ChartPoint[] = [];
  for (let i = weeks - 1; i >= 0; i--) {
    const ws = new Date(end.getFullYear(), end.getMonth(), end.getDate() - i * 7);
    out.push({ label: shortDate(ws), value: byWeek.get(ws.getTime()) ?? 0 });
  }
  return out;
}

export function weeklyVolume(workouts: Workout[], weeks = 12): ChartPoint[] {
  return weeklyAgg(workouts, weeks, volumeOf);
}

export function weeklyFrequency(workouts: Workout[], weeks = 12): ChartPoint[] {
  return weeklyAgg(workouts, weeks, () => 1);
}

export function exerciseNames(workouts: Workout[]): string[] {
  const set = new Set<string>();
  for (const w of workouts) {
    for (const s of w.sets) set.add(s.exercise);
  }
  return [...set].sort((a, b) => a.localeCompare(b));
}

export function exerciseProgression(workouts: Workout[], exercise: string, sessions = 12): ChartPoint[] {
  const points: { time: number; label: string; value: number }[] = [];
  for (const w of workouts) {
    let max = -Infinity;
    for (const s of w.sets) {
      if (s.exercise === exercise && s.weight_grams != null) {
        const kg = gramsToKg(s.weight_grams);
        if (kg > max) max = kg;
      }
    }
    if (max > -Infinity) {
      const d = parseLocalDate(w.date);
      points.push({ time: d.getTime(), label: shortDate(d), value: max });
    }
  }
  points.sort((a, b) => a.time - b.time);
  return points.slice(-sessions).map((p) => ({ label: p.label, value: p.value }));
}

export function personalRecords(workouts: Workout[]): PersonalRecord[] {
  const best = new Map<string, number>();
  for (const w of workouts) {
    for (const s of w.sets) {
      if (s.weight_grams != null) {
        const kg = gramsToKg(s.weight_grams);
        const cur = best.get(s.exercise);
        if (cur == null || kg > cur) best.set(s.exercise, kg);
      }
    }
  }
  return [...best.entries()]
    .map(([exercise, weightKg]) => ({ exercise, weightKg }))
    .sort((a, b) => b.weightKg - a.weightKg);
}
