import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getToday, list, upsert, todayString, type CheckIn } from "@/lib/checkins";
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Input } from "@/ui/Input";
import { Button } from "@/ui/Button";
import { Chip } from "@/ui/Chip";
import { Reveal, RevealItem } from "@/ui/Reveal";

export const Route = createFileRoute("/check-in")({ component: CheckInPage });

const DIMENSIONS: {
  key: "espiritual" | "emocional" | "fisica" | "financiera";
  label: string;
  short: string;
  variant: "accent" | "danger" | "money" | "sun";
}[] = [
  { key: "espiritual", label: "Espiritual", short: "E", variant: "accent" },
  { key: "emocional", label: "Emocional", short: "Em", variant: "danger" },
  { key: "fisica", label: "Fisica", short: "F", variant: "money" },
  { key: "financiera", label: "Financiera", short: "Fi", variant: "sun" },
];

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
  const [espiritual, setEspiritual] = useState("");
  const [emocional, setEmocional] = useState("");
  const [fisica, setFisica] = useState("");
  const [financiera, setFinanciera] = useState("");
  const [win, setWin] = useState("");
  const [avoided, setAvoided] = useState("");
  const [commitments, setCommitments] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const dimSetters = {
    espiritual: setEspiritual,
    emocional: setEmocional,
    fisica: setFisica,
    financiera: setFinanciera,
  } as const;
  const dimValues = { espiritual, emocional, fisica, financiera };

  // Pre-rellena el formulario una sola vez con el check-in de hoy, para no
  // pisar lo que el usuario esté editando si la query se refresca.
  const prefilled = useRef(false);
  useEffect(() => {
    const ci = todayQuery.data;
    if (ci && !prefilled.current) {
      setMood(ci.mood);
      setEnergy(ci.energy);
      setEspiritual(ci.espiritual ?? "");
      setEmocional(ci.emocional ?? "");
      setFisica(ci.fisica ?? "");
      setFinanciera(ci.financiera ?? "");
      setWin(ci.win ?? "");
      setAvoided(ci.avoided ?? "");
      setCommitments(ci.commitments ?? []);
      prefilled.current = true;
    }
  }, [todayQuery.data]);

  const mutation = useMutation({
    mutationFn: () =>
      upsert({
        date: today,
        mood,
        energy,
        espiritual,
        emocional,
        fisica,
        financiera,
        win,
        avoided,
        commitments: commitments.map((c) => c.trim()).filter((c) => c !== ""),
      }),
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
          className="mt-6 space-y-6"
        >
          <Card className="p-6 space-y-6">
            <h2 className="font-display text-sm font-bold uppercase tracking-[0.12em] text-muted">
              ¿Cómo estoy?
            </h2>
            <Slider label="Ánimo" value={mood} onChange={setMood} />
            <Slider label="Energía" value={energy} onChange={setEnergy} />
          </Card>

          <Card className="p-6 space-y-4">
            <h2 className="font-display text-sm font-bold uppercase tracking-[0.12em] text-muted">
              Mis 4 dimensiones
            </h2>
            {DIMENSIONS.map((d) => (
              <label key={d.key} className="block space-y-1">
                <span className="flex items-center gap-2 text-sm">
                  <Chip variant={d.variant}>{d.short}</Chip>
                  <span className="font-bold">{d.label}</span>
                </span>
                <Input
                  aria-label={d.label}
                  placeholder="¿qué hiciste hoy?"
                  value={dimValues[d.key]}
                  onChange={(e) => dimSetters[d.key](e.target.value)}
                />
              </label>
            ))}
          </Card>

          <Card className="p-6 space-y-4">
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                🏆 Win del día
              </span>
              <Input
                aria-label="Win del día"
                placeholder="¿qué hiciste hoy?"
                value={win}
                onChange={(e) => setWin(e.target.value)}
              />
            </label>
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                🚫 ¿Qué evité hoy?
              </span>
              <Input
                aria-label="Qué evité"
                placeholder="¿qué hiciste hoy?"
                value={avoided}
                onChange={(e) => setAvoided(e.target.value)}
              />
            </label>
          </Card>

          <Card className="p-6 space-y-3">
            <h2 className="font-display text-sm font-bold uppercase tracking-[0.12em] text-muted">
              Mañana me comprometo a:
            </h2>
            {commitments.map((c, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  aria-label={`Compromiso ${i + 1}`}
                  value={c}
                  onChange={(e) =>
                    setCommitments(
                      commitments.map((v, j) => (j === i ? e.target.value : v))
                    )
                  }
                />
                <button
                  type="button"
                  aria-label={`Quitar compromiso ${i + 1}`}
                  onClick={() =>
                    setCommitments(commitments.filter((_, j) => j !== i))
                  }
                  className="shrink-0 rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-sm font-bold text-ink transition-shadow hover:shadow-brutal-sm"
                >
                  ✕
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => setCommitments([...commitments, ""])}
              className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2 text-sm"
            >
              + agregar compromiso
            </button>
          </Card>

          {error && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
              {error}
            </p>
          )}

          <Button type="submit" disabled={mutation.isPending} className="w-full">
            {mutation.isPending ? "Guardando…" : "Guardar"}
          </Button>
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
                      Á{ci.mood} · E{ci.energy}
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
