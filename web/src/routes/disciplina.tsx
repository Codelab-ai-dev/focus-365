import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  listHabits,
  createHabit,
  checkHabit,
  archiveHabit,
  removeHabit,
  todayString,
  yesterdayString,
  type Habit,
} from "@/lib/habits";

export const Route = createFileRoute("/disciplina")({ component: DisciplinaPage });

function DisciplinaPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const [showArchived, setShowArchived] = useState(false);
  const habitsQuery = useQuery({
    queryKey: ["habits", showArchived],
    queryFn: () => listHabits(showArchived),
    enabled: !!user,
  });

  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["habits"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      createHabit({
        name: name.trim(),
        target_days: target === "" ? null : Number(target),
      }),
    onSuccess: () => {
      setError(null);
      setName("");
      setTarget("");
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al crear"),
  });

  const checkMutation = useMutation({
    mutationFn: (v: { id: string; day: string; done: boolean }) =>
      checkHabit(v.id, v.day, v.done),
    onSuccess: invalidate,
  });

  const archiveMutation = useMutation({
    mutationFn: (id: string) => archiveHabit(id),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => removeHabit(id),
    onSuccess: invalidate,
  });

  if (!user) return null;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Disciplina</h1>
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
          <span className="text-sm text-sand-400">Hábito o reto</span>
          <input
            type="text"
            aria-label="Nombre del hábito"
            placeholder="Leer 20 min, 100 flexiones…"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Meta de días (opcional)</span>
          <input
            type="number"
            aria-label="Meta de días"
            placeholder="21"
            min="1"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Creando…" : "Crear"}
        </button>
      </form>

      <div className="mt-6 flex gap-3 text-sm">
        <button
          type="button"
          onClick={() => setShowArchived(false)}
          className={!showArchived ? "font-bold text-amber-brand" : "text-sand-400"}
        >
          Activos
        </button>
        <button
          type="button"
          onClick={() => setShowArchived(true)}
          className={showArchived ? "font-bold text-amber-brand" : "text-sand-400"}
        >
          Archivados
        </button>
      </div>

      <section className="mt-4">
        {habitsQuery.data && habitsQuery.data.length > 0 ? (
          <ul className="space-y-3">
            {habitsQuery.data.map((h: Habit) => (
              <li
                key={h.id}
                className="rounded-xl border border-ink-700 bg-ink-900 p-4 text-sm"
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold">{h.name}</span>
                  <span className="text-streak">🔥 {h.current_streak} días</span>
                </div>
                <p className="mt-1 text-xs text-sand-400">
                  Récord {h.best_streak}
                  {h.target_days != null && ` · meta ${h.target_days}`}
                </p>
                {h.target_days != null && (
                  <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-ink-800">
                    <div
                      className="h-full bg-streak"
                      style={{
                        width: `${Math.min(100, (h.current_streak / h.target_days) * 100)}%`,
                      }}
                    />
                  </div>
                )}
                {!showArchived && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button
                      type="button"
                      aria-label={`Marcar hoy ${h.name}`}
                      onClick={() =>
                        checkMutation.mutate({ id: h.id, day: todayString(), done: !h.done_today })
                      }
                      className={
                        h.done_today
                          ? "rounded-lg bg-streak px-3 py-1 text-xs font-bold text-ink-950"
                          : "rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                      }
                    >
                      {h.done_today ? "Hecho hoy ✓" : "Marcar hoy"}
                    </button>
                    {!h.done_yesterday && (
                      <button
                        type="button"
                        aria-label={`Marcar ayer ${h.name}`}
                        onClick={() =>
                          checkMutation.mutate({ id: h.id, day: yesterdayString(), done: true })
                        }
                        className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                      >
                        Marcar ayer
                      </button>
                    )}
                    <button
                      type="button"
                      aria-label={`Archivar ${h.name}`}
                      onClick={() => archiveMutation.mutate(h.id)}
                      className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                    >
                      Archivar
                    </button>
                    <button
                      type="button"
                      aria-label={`Borrar ${h.name}`}
                      onClick={() => deleteMutation.mutate(h.id)}
                      className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400 hover:text-red-400"
                    >
                      Borrar
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-sand-400">
            {showArchived ? "No hay hábitos archivados." : "Aún no hay hábitos."}
          </p>
        )}
      </section>
    </div>
  );
}
