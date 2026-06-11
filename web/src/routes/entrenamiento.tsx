import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  listExercises,
  listWorkouts,
  createWorkout,
  removeWorkout,
  kgToGrams,
  gramsToKg,
  todayString,
  type Exercise,
  type Workout,
} from "@/lib/training";

export const Route = createFileRoute("/entrenamiento")({ component: EntrenamientoPage });

type SetRow = { exercise: string; reps: string; weightKg: string };

function emptyRow(): SetRow {
  return { exercise: "", reps: "", weightKg: "" };
}

function EntrenamientoPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const exercisesQuery = useQuery({
    queryKey: ["training", "exercises"],
    queryFn: () => listExercises(),
    enabled: !!user,
  });
  const historyQuery = useQuery({
    queryKey: ["training", "workouts"],
    queryFn: () => listWorkouts(),
    enabled: !!user,
  });

  const [date, setDate] = useState(todayString());
  const [type, setType] = useState("");
  const [note, setNote] = useState("");
  const [rows, setRows] = useState<SetRow[]>([emptyRow()]);
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["training"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      createWorkout({
        date,
        type,
        note,
        sets: rows
          .filter((r) => r.exercise.trim() !== "")
          .map((r) => ({
            exercise: r.exercise.trim(),
            reps: r.reps === "" ? null : Number(r.reps),
            weight_grams: r.weightKg === "" ? null : kgToGrams(Number(r.weightKg)),
          })),
      }),
    onSuccess: () => {
      setError(null);
      setType("");
      setNote("");
      setRows([emptyRow()]);
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al guardar"),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => removeWorkout(id),
    onSuccess: invalidate,
  });

  function updateRow(i: number, patch: Partial<SetRow>) {
    setRows((rs) => rs.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
  }

  if (!user) return null;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Entrenamiento</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          createMutation.mutate();
        }}
        className="mt-6 space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6"
      >
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Fecha</span>
          <input
            type="date"
            aria-label="Fecha"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Tipo</span>
          <input
            type="text"
            aria-label="Tipo"
            placeholder="Fuerza, Pierna…"
            value={type}
            onChange={(e) => setType(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <div className="space-y-3">
          <span className="text-sm text-sand-400">Series</span>
          {rows.map((row, i) => (
            <div key={i} className="flex gap-2">
              <input
                type="text"
                aria-label={`Ejercicio ${i + 1}`}
                list="catalogo-ejercicios"
                placeholder="Ejercicio"
                value={row.exercise}
                onChange={(e) => updateRow(i, { exercise: e.target.value })}
                className="flex-1 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
              />
              <input
                type="number"
                aria-label={`Reps ${i + 1}`}
                placeholder="Reps"
                min="0"
                value={row.reps}
                onChange={(e) => updateRow(i, { reps: e.target.value })}
                className="w-20 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
              />
              <input
                type="number"
                aria-label={`Peso ${i + 1}`}
                placeholder="kg"
                min="0"
                step="0.5"
                value={row.weightKg}
                onChange={(e) => updateRow(i, { weightKg: e.target.value })}
                className="w-20 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
              />
            </div>
          ))}
          <datalist id="catalogo-ejercicios">
            {exercisesQuery.data?.map((ex: Exercise) => (
              <option key={ex.id} value={ex.name} />
            ))}
          </datalist>
          <button
            type="button"
            onClick={() => setRows((rs) => [...rs, emptyRow()])}
            className="text-sm text-amber-brand"
          >
            + Agregar serie
          </button>
        </div>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Nota</span>
          <input
            type="text"
            aria-label="Nota"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Guardando…" : "Guardar"}
        </button>
      </form>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Historial</h2>
        {historyQuery.data && historyQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-3">
            {historyQuery.data.map((w: Workout) => (
              <li
                key={w.id}
                className="rounded-xl border border-ink-700 bg-ink-900 p-4 text-sm"
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold">
                    {w.date} {w.type && <span className="text-sand-400">· {w.type}</span>}
                  </span>
                  <button
                    type="button"
                    aria-label={`Borrar sesión ${w.date}`}
                    onClick={() => deleteMutation.mutate(w.id)}
                    className="text-xs text-sand-400 hover:text-red-400"
                  >
                    ✕
                  </button>
                </div>
                <ul className="mt-2 space-y-1 text-sand-400">
                  {w.sets.map((s, i) => (
                    <li key={i}>
                      {s.exercise}
                      {s.reps != null && ` · ${s.reps} reps`}
                      {s.weight_grams != null && ` · ${gramsToKg(s.weight_grams)} kg`}
                    </li>
                  ))}
                </ul>
                {w.note && <p className="mt-2 text-xs text-sand-400">{w.note}</p>}
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay sesiones.</p>
        )}
      </section>
    </div>
  );
}
