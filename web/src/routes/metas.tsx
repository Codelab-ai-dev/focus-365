import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  listGoals,
  createGoal,
  patchGoal,
  deleteGoal,
  type Goal,
  type GoalStatus,
} from "@/lib/goals";

export const Route = createFileRoute("/metas")({ component: MetasPage });

const DIMENSIONS = ["checkin", "finanzas", "entrenamiento", "mente", "general"];
const TABS: { value: GoalStatus; label: string }[] = [
  { value: "active", label: "Activas" },
  { value: "done", label: "Completadas" },
  { value: "paused", label: "Pausadas" },
];

function MetasPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const [tab, setTab] = useState<GoalStatus>("active");
  const goalsQuery = useQuery({
    queryKey: ["goals", tab],
    queryFn: () => listGoals(tab),
    enabled: !!user,
  });

  const [title, setTitle] = useState("");
  const [dimension, setDimension] = useState("general");
  const [deadline, setDeadline] = useState("");
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["goals"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      createGoal({
        title: title.trim(),
        dimension,
        deadline: deadline === "" ? null : deadline,
      }),
    onSuccess: () => {
      setError(null);
      setTitle("");
      setDeadline("");
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al crear"),
  });

  const patchMutation = useMutation({
    mutationFn: (v: { id: string; patch: Parameters<typeof patchGoal>[1] }) =>
      patchGoal(v.id, v.patch),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteGoal(id),
    onSuccess: invalidate,
  });

  if (!user) return null;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Metas</h1>
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
          <span className="text-sm text-sand-400">Meta</span>
          <input
            type="text"
            aria-label="Título de la meta"
            placeholder="Correr una 10k, ahorrar $X…"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Dimensión</span>
          <select
            aria-label="Dimensión"
            value={dimension}
            onChange={(e) => setDimension(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          >
            {DIMENSIONS.map((d) => (
              <option key={d} value={d}>{d}</option>
            ))}
          </select>
        </label>
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Fecha límite (opcional)</span>
          <input
            type="date"
            aria-label="Fecha límite"
            value={deadline}
            onChange={(e) => setDeadline(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Creando…" : "Crear meta"}
        </button>
      </form>

      <div className="mt-6 flex gap-3 text-sm">
        {TABS.map((tb) => (
          <button
            key={tb.value}
            type="button"
            onClick={() => setTab(tb.value)}
            className={tab === tb.value ? "font-bold text-amber-brand" : "text-sand-400"}
          >
            {tb.label}
          </button>
        ))}
      </div>

      <section className="mt-4">
        {goalsQuery.data && goalsQuery.data.length > 0 ? (
          <ul className="space-y-3">
            {goalsQuery.data.map((g: Goal) => (
              <li
                key={g.id}
                className="rounded-xl border border-ink-700 bg-ink-900 p-4 text-sm"
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold">{g.title}</span>
                  <span className="rounded-full bg-ink-800 px-2 py-0.5 text-xs text-sand-400">
                    {g.dimension}
                  </span>
                </div>
                <div className="mt-2 flex items-center gap-2">
                  <input
                    type="range"
                    aria-label={`Progreso ${g.title}`}
                    min={0}
                    max={100}
                    value={g.progress}
                    onChange={(e) =>
                      patchMutation.mutate({ id: g.id, patch: { progress: Number(e.target.value) } })
                    }
                    className="w-full accent-amber-brand"
                  />
                  <span className="w-10 text-right text-xs text-sand-400">{g.progress}%</span>
                </div>
                {g.deadline && (
                  <p className={`mt-1 text-xs ${g.overdue ? "font-bold text-red-400" : "text-sand-400"}`}>
                    {g.overdue ? "Vencida · " : ""}límite {g.deadline}
                  </p>
                )}
                <div className="mt-3 flex flex-wrap gap-2">
                  {tab === "active" && (
                    <>
                      <button
                        type="button"
                        aria-label={`Completar ${g.title}`}
                        onClick={() => patchMutation.mutate({ id: g.id, patch: { status: "done" } })}
                        className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                      >
                        Completar
                      </button>
                      <button
                        type="button"
                        aria-label={`Pausar ${g.title}`}
                        onClick={() => patchMutation.mutate({ id: g.id, patch: { status: "paused" } })}
                        className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                      >
                        Pausar
                      </button>
                    </>
                  )}
                  {tab !== "active" && (
                    <button
                      type="button"
                      aria-label={`Reactivar ${g.title}`}
                      onClick={() => patchMutation.mutate({ id: g.id, patch: { status: "active" } })}
                      className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                    >
                      Reactivar
                    </button>
                  )}
                  <button
                    type="button"
                    aria-label={`Borrar ${g.title}`}
                    onClick={() => deleteMutation.mutate(g.id)}
                    className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400 hover:text-red-400"
                  >
                    Borrar
                  </button>
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-sand-400">Aún no tenés metas en esta vista.</p>
        )}
      </section>
    </div>
  );
}
