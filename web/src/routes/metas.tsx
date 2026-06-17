import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  listGoals,
  createGoal,
  patchGoal,
  deleteGoal,
  todayString,
  type Goal,
  type GoalStatus,
} from "@/lib/goals";
import {
  listGoalNotes,
  createGoalNote,
  deleteGoalNote,
  type GoalNote,
} from "@/lib/goalNotes";
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Modal } from "@/ui/Modal";
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
  const [notesGoal, setNotesGoal] = useState<Goal | null>(null);

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
                      <Button
                        type="button"
                        variant="ghost"
                        aria-label={`Notas de ${g.title}`}
                        onClick={() => setNotesGoal(g)}
                        className="px-3 py-1 text-xs"
                      >
                        📝 Notas
                      </Button>
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

        <GoalNotesModal goal={notesGoal} onClose={() => setNotesGoal(null)} />
      </div>
    </PageTransition>
  );
}

function GoalNotesModal({
  goal,
  onClose,
}: {
  goal: Goal | null;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [body, setBody] = useState("");
  const [noteDate, setNoteDate] = useState(todayString());
  const [error, setError] = useState<string | null>(null);

  const notesQuery = useQuery({
    queryKey: ["goal-notes", goal?.id],
    queryFn: () => listGoalNotes(goal!.id),
    enabled: goal !== null,
  });

  const addMutation = useMutation({
    mutationFn: () => createGoalNote(goal!.id, { note_date: noteDate, body }),
    onSuccess: () => {
      setBody("");
      setError(null);
      qc.invalidateQueries({ queryKey: ["goal-notes", goal!.id] });
    },
    onError: (e) => setError(e instanceof Error ? e.message : "No se pudo guardar"),
  });

  const delMutation = useMutation({
    mutationFn: (noteId: string) => deleteGoalNote(goal!.id, noteId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["goal-notes", goal!.id] }),
  });

  const notes = notesQuery.data ?? [];

  return (
    <Modal open={goal !== null} onClose={onClose} title={goal ? goal.title : ""}>
      <div className="space-y-4 text-sm">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (body.trim()) addMutation.mutate();
          }}
          className="space-y-2"
        >
          <textarea
            aria-label="Nueva nota"
            placeholder="¿qué avanzaste?"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            className="w-full rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm"
            rows={3}
          />
          <div className="flex items-center gap-2">
            <input
              type="date"
              aria-label="Fecha de la nota"
              value={noteDate}
              onChange={(e) => setNoteDate(e.target.value)}
              className="rounded-lg border-2 border-ink bg-surface px-2 py-1"
            />
            <Button type="submit" disabled={addMutation.isPending || body.trim() === ""} className="px-3 py-1 text-xs">
              {addMutation.isPending ? "Guardando…" : "Agregar"}
            </Button>
          </div>
          {error && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">
              {error}
            </p>
          )}
        </form>

        {notes.length === 0 ? (
          <p className="text-muted">Sin notas todavía.</p>
        ) : (
          <ul className="space-y-2">
            {notes.map((n: GoalNote) => (
              <li key={n.id} className="rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
                <div className="flex items-start justify-between gap-2">
                  <span className="text-xs font-bold text-muted">{formatDay(n.note_date)}</span>
                  <button
                    type="button"
                    aria-label={`Borrar nota del ${n.note_date}`}
                    onClick={() => delMutation.mutate(n.id)}
                    className="shrink-0 text-xs font-bold text-ink"
                  >
                    🗑️
                  </button>
                </div>
                <p className="mt-1 whitespace-pre-wrap">{n.body}</p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}

// formatDay muestra "lun 16 jun" parseando YYYY-MM-DD como fecha LOCAL.
function formatDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString("es", {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
}
