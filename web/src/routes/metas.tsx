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
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { Chip } from "@/ui/Chip";
import { ProgressBar } from "@/ui/ProgressBar";
import { Reveal, RevealItem } from "@/ui/Reveal";

export const Route = createFileRoute("/metas")({ component: MetasPage });

// Las 4 dimensiones de Capitanes: etiqueta visible → valor almacenado.
const DIMENSIONS: { value: string; label: string }[] = [
  { value: "espiritual", label: "Espiritual" },
  { value: "emocional", label: "Emocional" },
  { value: "fisica", label: "Física" },
  { value: "financiera", label: "Financiera" },
];
const DIM_LABEL: Record<string, string> = Object.fromEntries(
  DIMENSIONS.map((d) => [d.value, d.label])
);
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
  const [dimension, setDimension] = useState("espiritual");
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
    <PageTransition>
      <div className="mx-auto max-w-3xl p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Metas</h1>
          <Link
            to="/"
            className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2"
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
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Meta</span>
              <Input
                type="text"
                aria-label="Título de la meta"
                placeholder="Correr una 10k, ahorrar $X…"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </label>
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Dimensión</span>
              <select
                aria-label="Dimensión"
                value={dimension}
                onChange={(e) => setDimension(e.target.value)}
                className="w-full rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-sm text-ink outline-none transition-shadow focus:shadow-brutal-sm"
              >
                {DIMENSIONS.map((d) => (
                  <option key={d.value} value={d.value}>{d.label}</option>
                ))}
              </select>
            </label>
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Fecha límite (opcional)</span>
              <Input
                type="date"
                aria-label="Fecha límite"
                value={deadline}
                onChange={(e) => setDeadline(e.target.value)}
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
              {createMutation.isPending ? "Creando…" : "Crear meta"}
            </Button>
          </form>
        </Card>

        <div className="mt-6 flex gap-3 text-sm">
          {TABS.map((tb) => (
            <button
              key={tb.value}
              type="button"
              onClick={() => setTab(tb.value)}
              className={
                tab === tb.value
                  ? "font-bold text-ink underline decoration-accent decoration-2 underline-offset-2"
                  : "text-muted"
              }
            >
              {tb.label}
            </button>
          ))}
        </div>

        <section className="mt-4">
          {goalsQuery.data && goalsQuery.data.length > 0 ? (
            <Reveal className="space-y-3">
              {goalsQuery.data.map((g: Goal) => (
                <RevealItem key={g.id}>
                  <Card className="p-4 text-sm">
                    <div className="flex items-center justify-between">
                      <span className="font-bold">{g.title}</span>
                      <Chip variant="plain" size="sm">{DIM_LABEL[g.dimension] ?? g.dimension}</Chip>
                    </div>
                    <ProgressBar value={g.progress} className="mt-2" />
                    <div className="mt-1 flex items-center gap-2">
                      <input
                        type="range"
                        aria-label={`Progreso ${g.title}`}
                        min={0}
                        max={100}
                        value={g.progress}
                        onChange={(e) =>
                          patchMutation.mutate({ id: g.id, patch: { progress: Number(e.target.value) } })
                        }
                        className="w-full accent-accent"
                      />
                      <span className="w-10 text-right text-xs text-muted">{g.progress}%</span>
                    </div>
                    {g.deadline && (
                      <p className="mt-1 text-xs">
                        {g.overdue ? (
                          <>
                            <Chip variant="danger" size="sm">Vencida</Chip>
                            {" · "}límite {g.deadline}
                          </>
                        ) : (
                          <span className="text-muted">límite {g.deadline}</span>
                        )}
                      </p>
                    )}
                    <div className="mt-3 flex flex-wrap gap-2">
                      {tab === "active" && (
                        <>
                          <Button
                            type="button"
                            variant="ghost"
                            aria-label={`Completar ${g.title}`}
                            onClick={() => patchMutation.mutate({ id: g.id, patch: { status: "done" } })}
                            className="px-3 py-1 text-xs"
                          >
                            Completar
                          </Button>
                          <Button
                            type="button"
                            variant="ghost"
                            aria-label={`Pausar ${g.title}`}
                            onClick={() => patchMutation.mutate({ id: g.id, patch: { status: "paused" } })}
                            className="px-3 py-1 text-xs"
                          >
                            Pausar
                          </Button>
                        </>
                      )}
                      {tab !== "active" && (
                        <Button
                          type="button"
                          variant="ghost"
                          aria-label={`Reactivar ${g.title}`}
                          onClick={() => patchMutation.mutate({ id: g.id, patch: { status: "active" } })}
                          className="px-3 py-1 text-xs"
                        >
                          Reactivar
                        </Button>
                      )}
                      <Button
                        type="button"
                        variant="ghost"
                        aria-label={`Borrar ${g.title}`}
                        onClick={() => deleteMutation.mutate(g.id)}
                        className="px-3 py-1 text-xs"
                      >
                        Borrar
                      </Button>
                    </div>
                  </Card>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="text-sm text-muted">Aún no tenés metas en esta vista.</p>
          )}
        </section>
      </div>
    </PageTransition>
  );
}
