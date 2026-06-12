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
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { Chip } from "@/ui/Chip";
import { Reveal, RevealItem } from "@/ui/Reveal";

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
    <PageTransition>
      <div className="mx-auto max-w-3xl p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Entrenamiento</h1>
          <Link
            to="/"
            className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2 text-sm"
          >
            Volver
          </Link>
        </header>

        <Card className="mt-6 p-6">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              createMutation.mutate();
            }}
            className="space-y-4"
          >
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Fecha</span>
              <Input
                type="date"
                aria-label="Fecha"
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            </label>

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Tipo</span>
              <Input
                type="text"
                aria-label="Tipo"
                placeholder="Fuerza, Pierna…"
                value={type}
                onChange={(e) => setType(e.target.value)}
              />
            </label>

            <div className="space-y-3">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Series</span>
              {rows.map((row, i) => (
                <div key={i} className="flex gap-2">
                  <Input
                    type="text"
                    aria-label={`Ejercicio ${i + 1}`}
                    list="catalogo-ejercicios"
                    placeholder="Ejercicio"
                    value={row.exercise}
                    onChange={(e) => updateRow(i, { exercise: e.target.value })}
                    className="flex-1"
                  />
                  <Input
                    type="number"
                    aria-label={`Reps ${i + 1}`}
                    placeholder="Reps"
                    min="0"
                    value={row.reps}
                    onChange={(e) => updateRow(i, { reps: e.target.value })}
                    className="w-20"
                  />
                  <Input
                    type="number"
                    aria-label={`Peso ${i + 1}`}
                    placeholder="kg"
                    min="0"
                    step="0.5"
                    value={row.weightKg}
                    onChange={(e) => updateRow(i, { weightKg: e.target.value })}
                    className="w-20"
                  />
                </div>
              ))}
              <datalist id="catalogo-ejercicios">
                {exercisesQuery.data?.map((ex: Exercise) => (
                  <option key={ex.id} value={ex.name} />
                ))}
              </datalist>
              <Button
                type="button"
                variant="ghost"
                onClick={() => setRows((rs) => [...rs, emptyRow()])}
                className="px-3 py-1 text-xs"
              >
                + Agregar serie
              </Button>
            </div>

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Nota</span>
              <Input
                type="text"
                aria-label="Nota"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </label>

            {error && (
              <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
                {error}
              </p>
            )}

            <Button
              type="submit"
              disabled={createMutation.isPending}
              className="w-full"
            >
              {createMutation.isPending ? "Guardando…" : "Guardar"}
            </Button>
          </form>
        </Card>

        <section className="mt-8">
          <h2 className="font-display text-lg font-bold tracking-tight">Historial</h2>
          {historyQuery.data && historyQuery.data.length > 0 ? (
            <Reveal className="mt-3 space-y-3">
              {historyQuery.data.map((w: Workout) => (
                <RevealItem key={w.id}>
                  <Card className="p-4 text-sm">
                    <div className="flex items-center justify-between">
                      <span className="font-bold">
                        {w.date}{" "}
                        {w.type && (
                          <span className="text-muted">· {w.type}</span>
                        )}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        aria-label={`Borrar sesión ${w.date}`}
                        onClick={() => deleteMutation.mutate(w.id)}
                        className="px-2 py-0.5 text-xs"
                      >
                        ✕
                      </Button>
                    </div>
                    <ul className="mt-2 space-y-1 text-muted">
                      {w.sets.map((s, i) => (
                        <li key={i}>
                          {s.exercise}
                          {s.reps != null && (
                            <>
                              {" · "}
                              <Chip variant="sky" size="sm">{s.reps} reps</Chip>
                            </>
                          )}
                          {s.weight_grams != null && (
                            <>
                              {" · "}
                              <Chip variant="sky" size="sm">{gramsToKg(s.weight_grams)} kg</Chip>
                            </>
                          )}
                        </li>
                      ))}
                    </ul>
                    {w.note && <p className="mt-2 text-xs text-muted">{w.note}</p>}
                  </Card>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="mt-3 text-sm text-muted">Aún no hay sesiones.</p>
          )}
        </section>
      </div>
    </PageTransition>
  );
}
