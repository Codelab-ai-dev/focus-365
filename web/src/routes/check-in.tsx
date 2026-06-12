import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getToday, list, upsert, todayString, type CheckIn } from "@/lib/checkins";
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Reveal, RevealItem } from "@/ui/Reveal";

export const Route = createFileRoute("/check-in")({ component: CheckInPage });

function CheckInPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const today = todayString();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const todayQuery = useQuery({
    queryKey: ["checkin", "today", today],
    queryFn: () => getToday(today),
    enabled: !!user,
  });
  const historyQuery = useQuery({
    queryKey: ["checkin", "list"],
    queryFn: () => list(30),
    enabled: !!user,
  });

  const [mood, setMood] = useState(5);
  const [energy, setEnergy] = useState(5);
  const [discipline, setDiscipline] = useState(5);
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Pre-rellena el formulario una sola vez con el check-in de hoy, para no
  // pisar lo que el usuario esté editando si la query se refresca.
  const prefilled = useRef(false);
  useEffect(() => {
    const ci = todayQuery.data;
    if (ci && !prefilled.current) {
      setMood(ci.mood);
      setEnergy(ci.energy);
      setDiscipline(ci.discipline);
      setNote(ci.note);
      prefilled.current = true;
    }
  }, [todayQuery.data]);

  const mutation = useMutation({
    mutationFn: () => upsert({ date: today, mood, energy, discipline, note }),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["checkin", "today"] });
      qc.invalidateQueries({ queryKey: ["checkin", "list"] });
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al guardar"),
  });

  if (!user) return null;

  return (
    <PageTransition>
      <div className="mx-auto max-w-xl p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Check-in de hoy</h1>
          <Link
            to="/"
            className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2 text-sm"
          >
            Volver
          </Link>
        </header>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
          className="mt-6"
        >
          <Card className="p-6 space-y-6">
            <Slider label="Ánimo" value={mood} onChange={setMood} />
            <Slider label="Energía" value={energy} onChange={setEnergy} />
            <Slider label="Disciplina" value={discipline} onChange={setDiscipline} />

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                Nota
              </span>
              <textarea
                aria-label="Nota"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                rows={3}
                className="w-full rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-sm text-ink outline-none transition-shadow focus:shadow-brutal-sm"
              />
            </label>

            {error && (
              <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
                {error}
              </p>
            )}

            <Button
              type="submit"
              disabled={mutation.isPending}
              className="w-full"
            >
              {mutation.isPending ? "Guardando…" : "Guardar"}
            </Button>
          </Card>
        </form>

        <section className="mt-8">
          <h2 className="font-display text-xl font-bold tracking-tight">Historial</h2>
          {historyQuery.data && historyQuery.data.length > 0 ? (
            <Reveal className="mt-3 space-y-2">
              {historyQuery.data.map((ci: CheckIn) => (
                <RevealItem key={ci.id}>
                  <Card className="flex items-center justify-between px-4 py-2 text-sm">
                    <span className="text-muted">{ci.date}</span>
                    <span>
                      Á{ci.mood} · E{ci.energy} · D{ci.discipline}
                    </span>
                  </Card>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="mt-3 text-sm text-muted">Aún no hay check-ins.</p>
          )}
        </section>
      </div>
    </PageTransition>
  );
}

function Slider({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
}) {
  return (
    <label className="block space-y-1">
      <span className="flex items-center justify-between text-sm">
        <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">{label}</span>
        <span className="font-display font-bold text-accent">{value}</span>
      </span>
      <input
        type="range"
        min={1}
        max={10}
        step={1}
        aria-label={label}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-accent"
      />
    </label>
  );
}
