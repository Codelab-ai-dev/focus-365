import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { motion } from "framer-motion";
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
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { Chip } from "@/ui/Chip";
import { PageTransition } from "@/ui/PageTransition";
import { Reveal, RevealItem } from "@/ui/Reveal";

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
    <PageTransition>
      <div className="mx-auto max-w-3xl p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Disciplina</h1>
          <Link
            to="/"
            className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2"
          >
            Volver
          </Link>
        </header>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            createMutation.mutate();
          }}
          className="mt-6"
        >
          <Card className="p-6 space-y-4">
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                Hábito o reto
              </span>
              <Input
                type="text"
                aria-label="Nombre del hábito"
                placeholder="Leer 20 min, 100 flexiones…"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                Meta de días (opcional)
              </span>
              <Input
                type="number"
                aria-label="Meta de días"
                placeholder="21"
                min="1"
                value={target}
                onChange={(e) => setTarget(e.target.value)}
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
              {createMutation.isPending ? "Creando…" : "Crear"}
            </Button>
          </Card>
        </form>

        <div className="mt-6 flex gap-3 text-sm">
          <button
            type="button"
            onClick={() => setShowArchived(false)}
            className={
              !showArchived
                ? "font-bold text-ink underline decoration-accent decoration-2 underline-offset-2"
                : "text-muted"
            }
          >
            Activos
          </button>
          <button
            type="button"
            onClick={() => setShowArchived(true)}
            className={
              showArchived
                ? "font-bold text-ink underline decoration-accent decoration-2 underline-offset-2"
                : "text-muted"
            }
          >
            Archivados
          </button>
        </div>

        <section className="mt-4">
          {habitsQuery.data && habitsQuery.data.length > 0 ? (
            <Reveal className="space-y-3">
              {habitsQuery.data.map((h: Habit) => (
                <RevealItem key={h.id}>
                  <Card className="p-4 text-sm">
                    <div className="flex items-center justify-between">
                      <span className="font-bold">{h.name}</span>
                      <Chip variant="sun" size="sm">🔥 {h.current_streak} días</Chip>
                    </div>
                    <p className="mt-1 text-xs text-muted">
                      Récord {h.best_streak}
                      {h.target_days != null && ` · meta ${h.target_days}`}
                    </p>
                    {h.target_days != null && (
                      <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-surface border-2 border-ink">
                        <div
                          className="h-full bg-accent"
                          style={{
                            width: `${Math.min(100, (h.current_streak / h.target_days) * 100)}%`,
                          }}
                        />
                      </div>
                    )}
                    {!showArchived && (
                      <div className="mt-3 flex flex-wrap items-center gap-2">
                        <motion.button
                          whileTap={{ scale: 0.85 }}
                          type="button"
                          aria-label={`Marcar hoy ${h.name}`}
                          onClick={() =>
                            checkMutation.mutate({ id: h.id, day: todayString(), done: !h.done_today })
                          }
                          className={`grid h-9 w-9 place-items-center rounded-md border-[2.5px] border-ink text-lg font-bold shadow-brutal-sm transition-colors ${
                            h.done_today ? "bg-accent text-[#16130e]" : "bg-surface text-muted"
                          }`}
                        >
                          {h.done_today ? "✓" : ""}
                        </motion.button>
                        {!h.done_yesterday && (
                          <Button
                            variant="ghost"
                            type="button"
                            aria-label={`Marcar ayer ${h.name}`}
                            onClick={() =>
                              checkMutation.mutate({ id: h.id, day: yesterdayString(), done: true })
                            }
                          >
                            Marcar ayer
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          type="button"
                          aria-label={`Archivar ${h.name}`}
                          onClick={() => archiveMutation.mutate(h.id)}
                        >
                          Archivar
                        </Button>
                        <Button
                          variant="ghost"
                          type="button"
                          aria-label={`Borrar ${h.name}`}
                          onClick={() => deleteMutation.mutate(h.id)}
                        >
                          Borrar
                        </Button>
                      </div>
                    )}
                  </Card>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="text-sm text-muted">
              {showArchived ? "No hay hábitos archivados." : "Aún no hay hábitos."}
            </p>
          )}
        </section>
      </div>
    </PageTransition>
  );
}
