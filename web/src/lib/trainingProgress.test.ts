import { describe, it, expect } from "vitest";
import {
  weeklyVolume,
  weeklyFrequency,
  exerciseNames,
  exerciseProgression,
  personalRecords,
} from "./trainingProgress";
import type { Workout } from "./training";

function w(date: string, sets: { exercise: string; reps: number | null; weight_grams: number | null }[]): Workout {
  return {
    id: date + Math.random(),
    date,
    type: "",
    note: "",
    created_at: "",
    sets: sets.map((s) => ({ ...s, note: "" })),
  };
}

// Lun 2026-06-15 .. Dom 2026-06-21 es una semana; 2026-06-08 la anterior.
const data: Workout[] = [
  w("2026-06-16", [{ exercise: "Sentadilla", reps: 5, weight_grams: 100000 }]), // semana del 15
  w("2026-06-18", [{ exercise: "Sentadilla", reps: 5, weight_grams: 105000 }]), // misma semana
  w("2026-06-09", [{ exercise: "Sentadilla", reps: 5, weight_grams: 95000 }]),  // semana del 08
  w("2026-06-10", [{ exercise: "Banca", reps: 8, weight_grams: 60000 }]),       // semana del 08
];

describe("trainingProgress", () => {
  it("weeklyVolume suma reps×kg por semana", () => {
    const v = weeklyVolume(data, 3);
    // 3 semanas terminando en la del 15: [01,08,15]
    expect(v).toHaveLength(3);
    // semana del 15: 5*100 + 5*105 = 1025
    expect(v[2].value).toBe(1025);
    // semana del 08: 5*95 + 8*60 = 475 + 480 = 955
    expect(v[1].value).toBe(955);
    // semana del 01: sin entrenos -> 0
    expect(v[0].value).toBe(0);
  });

  it("weeklyFrequency cuenta sesiones por semana", () => {
    const f = weeklyFrequency(data, 3);
    expect(f[2].value).toBe(2); // 16 y 18
    expect(f[1].value).toBe(2); // 09 y 10
    expect(f[0].value).toBe(0);
  });

  it("exerciseNames devuelve los ejercicios distintos ordenados", () => {
    expect(exerciseNames(data)).toEqual(["Banca", "Sentadilla"]);
  });

  it("exerciseProgression toma el máximo por sesión del ejercicio", () => {
    const p = exerciseProgression(data, "Sentadilla", 12);
    // sesiones con Sentadilla, cronológicas: 09 (95), 16 (100), 18 (105)
    expect(p.map((x) => x.value)).toEqual([95, 100, 105]);
  });

  it("personalRecords da el mejor peso por ejercicio", () => {
    const prs = personalRecords(data);
    expect(prs).toEqual([
      { exercise: "Sentadilla", weightKg: 105 },
      { exercise: "Banca", weightKg: 60 },
    ]);
  });

  it("casos vacíos", () => {
    expect(weeklyVolume([], 12)).toEqual([]);
    expect(weeklyFrequency([], 12)).toEqual([]);
    expect(exerciseNames([])).toEqual([]);
    expect(exerciseProgression([], "x", 12)).toEqual([]);
    expect(personalRecords([])).toEqual([]);
  });
});
