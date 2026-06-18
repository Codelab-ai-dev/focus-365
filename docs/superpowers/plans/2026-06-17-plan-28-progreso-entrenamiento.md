# Evolución / progreso — Plan de implementación (Entrenamiento slice D, Rebanada 28)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Una sección "Progreso" en `/entrenamiento` con volumen y frecuencia semanales, progresión de peso por ejercicio (con selector) y records (PRs), calculados en el frontend desde el historial de entrenos.

**Architecture:** Frontend-only, sin backend. Funciones puras de agregación (`lib/trainingProgress.ts`) sobre la lista de `Workout` que ya carga la página; un componente de barras SVG hecho a mano (`ui/BarChart.tsx`, sin dependencias nuevas); una sección "Progreso" en `entrenamiento.tsx` que cablea todo.

**Tech Stack:** React + Vite + TanStack Query + Vitest. Sin librería de gráficos.

**Contexto del repo (leer antes de empezar):**
- Comandos web: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run <archivo>` para un test; `npx vitest run && npm run build` para suite + typecheck.
- `lib/training.ts` exporta `type Workout = { id; date: string /* YYYY-MM-DD */; type; note; sets: WorkoutSet[]; created_at }`, `type WorkoutSet = { exercise: string; reps: number | null; weight_grams: number | null; note: string }`, y `gramsToKg(grams: number): number` (= grams/1000). `listWorkouts()` devuelve los workouts **ordenados date DESC**.
- `entrenamiento.tsx` ya tiene `historyQuery` (`useQuery(["training","workouts"], ...)` o similar; revisá la queryKey real) cuya `data` es `Workout[]`. La sección "Historial" termina con `</section>` (hoy ~línea 416); la nueva sección "Progreso" va **después** de ese `</section>` y **antes** del cierre del `<div>` contenedor.
- Tokens del design system: `border-2 border-ink`, `shadow-brutal-sm`, `bg-surface`, `fill-accent`/`bg-accent`, `stroke-ink`, `text-muted`, `font-display`. `@/ui/Card` ya se importa en la página. Estilo de import de React: named (`import { ... } from "react"`), `jsx: react-jsx`.
- Helpers de fecha: NO usar `new Date("YYYY-MM-DD")` (UTC). Parsear local con split (`new Date(y, m-1, d)`).

---

## Estructura de archivos

- Crear `web/src/lib/trainingProgress.ts` — funciones puras de agregación + tipos.
- Crear `web/src/lib/trainingProgress.test.ts` — unit tests (el núcleo).
- Crear `web/src/ui/BarChart.tsx` — barras SVG reutilizables.
- Crear `web/src/ui/BarChart.test.tsx` — test del componente.
- Modificar `web/src/routes/entrenamiento.tsx` — sección "Progreso".
- Modificar `web/src/routes/entrenamiento.test.tsx` — test de la sección.

---

## Task 1: `lib/trainingProgress.ts` (agregaciones puras)

**Files:**
- Create: `web/src/lib/trainingProgress.ts`
- Create: `web/src/lib/trainingProgress.test.ts`

- [ ] **Step 1: Tests (que fallan)**

Crear `web/src/lib/trainingProgress.test.ts`:

```ts
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
```

- [ ] **Step 2: Verlos fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/trainingProgress.test.ts`
Expected: FAIL (módulo no existe).

- [ ] **Step 3: Implementar `web/src/lib/trainingProgress.ts`**

```ts
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
```

- [ ] **Step 4: Verde + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/trainingProgress.test.ts && npm run build`
Expected: tests PASS y build OK.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/trainingProgress.ts web/src/lib/trainingProgress.test.ts
git commit -m "feat(web): agregaciones de progreso de entrenamiento (puras)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `ui/BarChart.tsx` (barras SVG a mano)

**Files:**
- Create: `web/src/ui/BarChart.tsx`
- Create: `web/src/ui/BarChart.test.tsx`

- [ ] **Step 1: Test (que falla)**

Crear `web/src/ui/BarChart.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { BarChart } from "./BarChart";

describe("BarChart", () => {
  it("dibuja una barra por dato", () => {
    const { container } = render(
      <BarChart data={[{ label: "16/6", value: 10 }, { label: "23/6", value: 20 }]} unit="kg" />
    );
    expect(container.querySelectorAll("rect")).toHaveLength(2);
    const svg = container.querySelector("svg");
    expect(svg?.getAttribute("aria-label")).toContain("16/6");
  });

  it("lista vacía muestra 'sin datos'", () => {
    const { getByText } = render(<BarChart data={[]} />);
    expect(getByText("sin datos")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/ui/BarChart.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implementar `web/src/ui/BarChart.tsx`**

```tsx
export type ChartPoint = { label: string; value: number };

// BarChart dibuja barras SVG (neo-brutalista: relleno accent, borde ink) a partir
// de una serie {label, value}. Alto proporcional al máximo de la serie.
export function BarChart({
  data,
  unit = "",
  className = "",
}: {
  data: ChartPoint[];
  unit?: string;
  className?: string;
}) {
  if (data.length === 0) {
    return <p className={`text-xs text-muted ${className}`}>sin datos</p>;
  }
  const max = Math.max(...data.map((d) => d.value), 1);
  const W = 100;
  const H = 60;
  const gap = data.length > 1 ? 2 : 0;
  const barW = (W - gap * (data.length - 1)) / data.length;
  const aria =
    "Gráfico de barras: " +
    data.map((d) => `${d.label} ${Math.round(d.value)}${unit}`).join(", ");

  return (
    <div className={className}>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={aria} className="h-24 w-full">
        {data.map((d, i) => {
          const h = (d.value / max) * (H - 1);
          const x = i * (barW + gap);
          return (
            <rect
              key={i}
              x={x}
              y={H - h}
              width={barW}
              height={h}
              className="fill-accent stroke-ink"
              strokeWidth={0.5}
            >
              <title>{`${d.label}: ${Math.round(d.value)}${unit}`}</title>
            </rect>
          );
        })}
      </svg>
      <div className="mt-1 flex gap-[2px] text-[8px] text-muted">
        {data.map((d, i) => (
          <span key={i} className="flex-1 truncate text-center">
            {d.label}
          </span>
        ))}
      </div>
    </div>
  );
}
```

> `fill-accent`/`stroke-ink` son utilidades de Tailwind sobre los colores del tema (`accent`/`ink`); si el proyecto no las genera para SVG, usá `style={{ fill: "var(--color-accent)", stroke: "var(--color-ink)" }}` — revisá cómo el repo expone los colores del tema (`tailwind.config`/CSS vars) y adaptá. La barra debe verse con relleno de acento y borde de tinta.

- [ ] **Step 4: Verde + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/ui/BarChart.test.tsx && npm run build`
Expected: PASS y build OK.

- [ ] **Step 5: Commit**

```bash
git add web/src/ui/BarChart.tsx web/src/ui/BarChart.test.tsx
git commit -m "feat(web): componente BarChart (barras SVG)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Sección "Progreso" en `entrenamiento.tsx`

**Files:**
- Modify: `web/src/routes/entrenamiento.tsx`
- Modify: `web/src/routes/entrenamiento.test.tsx`

- [ ] **Step 1: Test (que falla)**

En `web/src/routes/entrenamiento.test.tsx`, mirá el `fetchMock()` y cómo siembra `/training/workouts`. Hacé que devuelva (al menos en un test nuevo) un par de workouts con series con peso, y agregá un test: la sección "Progreso" aparece, hay barras (`rect`) y la lista de PRs muestra un ejercicio. Ejemplo de asserts (adaptá al harness real):

```tsx
// Con el fetchMock devolviendo workouts con sets (exercise/reps/weight_grams):
// renderPage();
// expect(await screen.findByText("Progreso")).toBeInTheDocument();
// expect(document.querySelectorAll("svg rect").length).toBeGreaterThan(0);
// expect(screen.getByText(/Sentadilla/)).toBeInTheDocument(); // en PRs o selector
```
Si el `fetchMock` actual devuelve un workout sin peso, sumá uno con `weight_grams`. Watch fail (no existe "Progreso").

- [ ] **Step 2: Imports + estado en `entrenamiento.tsx`**

```tsx
import { BarChart } from "@/ui/BarChart";
import {
  weeklyVolume,
  weeklyFrequency,
  exerciseNames,
  exerciseProgression,
  personalRecords,
} from "@/lib/trainingProgress";
```
(`useMemo` de `react` — agregalo al import existente de `react` si no está.)

Dentro del componente, junto a los otros `useState`:

```tsx
  const [progressExercise, setProgressExercise] = useState("");
```

- [ ] **Step 3: Series derivadas (useMemo)**

Después de obtener `historyQuery.data`, derivar las series. Buscá la línea donde se usa `historyQuery.data` (p. ej. `const workouts = historyQuery.data ?? []` — si no existe, definila) y agregá:

```tsx
  const workouts = historyQuery.data ?? [];
  const volume = useMemo(() => weeklyVolume(workouts, 12), [workouts]);
  const frequency = useMemo(() => weeklyFrequency(workouts, 12), [workouts]);
  const names = useMemo(() => exerciseNames(workouts), [workouts]);
  const selectedExercise = progressExercise && names.includes(progressExercise) ? progressExercise : names[0] ?? "";
  const progression = useMemo(
    () => (selectedExercise ? exerciseProgression(workouts, selectedExercise, 12) : []),
    [workouts, selectedExercise]
  );
  const prs = useMemo(() => personalRecords(workouts), [workouts]);
```

> Si la página ya define `const workouts = ...` o usa `historyQuery.data` directo en el `.map` del historial, reusá esa variable en lugar de redefinirla (evitá duplicar). Adaptá el `.map` del historial a `workouts` si hace falta.

- [ ] **Step 4: Sección "Progreso"**

Agregar **después** del `</section>` del Historial y antes del cierre del `<div>` contenedor:

```tsx
        <section className="mt-8">
          <h2 className="font-display text-lg font-bold tracking-tight">Progreso</h2>
          {workouts.length === 0 ? (
            <p className="mt-3 text-sm text-muted">Registrá entrenos para ver tu progreso.</p>
          ) : (
            <div className="mt-3 space-y-4">
              <Card className="p-4">
                <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Volumen por semana (kg·reps)</h3>
                <BarChart data={volume} className="mt-2" />
              </Card>
              <Card className="p-4">
                <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Frecuencia por semana</h3>
                <BarChart data={frequency} className="mt-2" />
              </Card>
              {names.length > 0 && (
                <Card className="p-4 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Progresión (peso máx.)</h3>
                    <select
                      aria-label="Ejercicio"
                      value={selectedExercise}
                      onChange={(e) => setProgressExercise(e.target.value)}
                      className="rounded-lg border-2 border-ink bg-surface px-2 py-1 text-xs"
                    >
                      {names.map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                    </select>
                  </div>
                  <BarChart data={progression} unit="kg" />
                </Card>
              )}
              {prs.length > 0 && (
                <Card className="p-4">
                  <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Records</h3>
                  <ul className="mt-2 space-y-1 text-sm">
                    {prs.map((p) => (
                      <li key={p.exercise} className="flex justify-between">
                        <span>{p.exercise}</span>
                        <span className="font-bold">{p.weightKg} kg</span>
                      </li>
                    ))}
                  </ul>
                </Card>
              )}
            </div>
          )}
        </section>
```

- [ ] **Step 5: Verde + suite + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido).

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/entrenamiento.tsx web/src/routes/entrenamiento.test.tsx
git commit -m "feat(web): sección Progreso en entrenamiento (volumen, frecuencia, progresión, PRs)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Cierre — review, merge y verificación

**Files:** verificación + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-28-progreso-entrenamiento-design.md`. Aplicar nits.

- [ ] **Step 2: Suite verde + build:** `cd web && npx vitest run && npm run build`.

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`. Mensaje de merge describiendo la rebanada 28.

- [ ] **Step 4: Deploy manual (Coolify) + verificación visual.** Es solo-frontend: no hay smoke con curl. Tras el deploy, en `/entrenamiento` con entrenos registrados, ver la sección "Progreso" con los gráficos y el selector. (Recordatorio: si el Deploy falla en el build con el código compilando local, sospechar disco lleno en el VPS → `docker system prune -af`.)

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-28-progreso-entrenamiento.md` — **cierra la expansión de entrenamiento (A→D)**.

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 `trainingProgress.ts` (weeklyVolume/weeklyFrequency/exerciseNames/
  exerciseProgression/personalRecords, agrupado por semana=lunes, máximos) → Task 1. ✓
- §2 `BarChart.tsx` (SVG a mano, rect por dato, vacío, aria) → Task 2. ✓
- §2 sección "Progreso" (reusa historyQuery, charts + selector + PRs, estado vacío) → Task 3. ✓
- §3 ventanas (12 semanas / 12 sesiones) → Task 1 (defaults) + Task 3 (llamadas). ✓
- §4 bordes (sin workouts, semanas en 0, ejercicio sin peso) → Task 1 (lógica) + Task 3 (estado vacío). ✓
- §5 testing → Tasks 1–3; visual → Task 4. ✓
- §6 aceptación → Task 4 (verificación visual). ✓

**Placeholders:** la nota sobre `fill-accent`/CSS vars es una adaptación determinista al sistema de colores del repo (con instrucción de qué inspeccionar). Sin TODOs de diseño.

**Consistencia de tipos/firmas:** `ChartPoint{label,value}` se define en `trainingProgress.ts` y también en `BarChart.tsx` (mismo shape estructural; `BarChart` recibe `ChartPoint[]` por estructura, no requiere importar el tipo). `weeklyVolume/weeklyFrequency(workouts, weeks=12) → ChartPoint[]`, `exerciseNames(workouts) → string[]`, `exerciseProgression(workouts, exercise, sessions=12) → ChartPoint[]`, `personalRecords(workouts) → {exercise, weightKg}[]` ↔ uso en `entrenamiento.tsx`. `gramsToKg` reutilizado. ✓

**Lección aplicada:** las tasks 1 y 2 (lib/ui) corren `npm run build` además del test.
