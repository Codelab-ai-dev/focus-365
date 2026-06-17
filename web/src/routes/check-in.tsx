import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getToday, list, upsert, todayString, type CheckIn } from "@/lib/checkins";
import { getDue, toggle, type Commitment } from "@/lib/commitments";
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Input } from "@/ui/Input";
import { Button } from "@/ui/Button";
import { Chip } from "@/ui/Chip";
import { Reveal, RevealItem } from "@/ui/Reveal";
import { Modal } from "@/ui/Modal";

export const Route = createFileRoute("/check-in")({ component: CheckInPage });

const DIMENSIONS: {
  key: "espiritual" | "emocional" | "fisica" | "financiera";
  label: string;
  short: string;
  variant: "accent" | "danger" | "money" | "sun";
}[] = [
  { key: "espiritual", label: "Espiritual", short: "E", variant: "accent" },
  { key: "emocional", label: "Emocional", short: "Em", variant: "danger" },
  { key: "fisica", label: "Física", short: "F", variant: "money" },
  { key: "financiera", label: "Financiera", short: "Fi", variant: "sun" },
];

function CheckInPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const today = todayString();
  const tomorrow = todayString(
    new Date(new Date().getTime() + 24 * 60 * 60 * 1000)
  );

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
  // Compromisos cuyo objetivo es hoy: "ayer te comprometiste a" (marcar cumplido).
  const dueQuery = useQuery({
    queryKey: ["commitments", "due", today],
    queryFn: () => getDue(today),
    enabled: !!user,
  });
  // Compromisos de mañana: precargan la lista editable "mañana me comprometo a".
  const tomorrowQuery = useQuery({
    queryKey: ["commitments", "due", tomorrow],
    queryFn: () => getDue(tomorrow),
    enabled: !!user,
  });

  const [selected, setSelected] = useState<CheckIn | null>(null);

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
      prefilled.current = true;
    }
  }, [todayQuery.data]);

  // Precarga la lista "mañana me comprometo a" desde los compromisos de mañana,
  // una sola vez, para no pisar lo que el usuario esté editando.
  const commitmentsPrefilled = useRef(false);
  useEffect(() => {
    const data = tomorrowQuery.data;
    if (data && !commitmentsPrefilled.current) {
      setCommitments(data.map((c) => c.text));
      commitmentsPrefilled.current = true;
    }
  }, [tomorrowQuery.data]);

  const toggleMutation = useMutation({
    mutationFn: (id: string) => toggle(id),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ["commitments", "due", today] }),
  });

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

        {dueQuery.data && dueQuery.data.length > 0 && (
          <Card className="mt-6 p-6 space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="font-display text-sm font-bold uppercase tracking-[0.12em] text-muted">
                📋 Ayer te comprometiste a
              </h2>
              <span className="font-display text-sm font-bold text-accent">
                {dueQuery.data.filter((c) => c.done).length}/
                {dueQuery.data.length} ✓
              </span>
            </div>
            {dueQuery.data.map((c: Commitment) => (
              <button
                key={c.id}
                type="button"
                aria-label={`Marcar: ${c.text}`}
                onClick={() => toggleMutation.mutate(c.id)}
                className="flex w-full items-center gap-3 rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-left text-sm font-bold text-ink transition-shadow hover:shadow-brutal-sm"
              >
                <span
                  className={`flex h-5 w-5 shrink-0 items-center justify-center rounded border-2 border-ink text-xs ${
                    c.done ? "bg-accent text-ink" : "bg-surface"
                  }`}
                >
                  {c.done ? "✓" : ""}
                </span>
                <span className={c.done ? "line-through text-muted" : ""}>
                  {c.text}
                </span>
              </button>
            ))}
          </Card>
        )}

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
                placeholder="¿qué decidiste NO hacer?"
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
                  <button
                    type="button"
                    aria-label={`Ver detalle del ${ci.date}`}
                    onClick={() => setSelected(ci)}
                    className="w-full text-left"
                  >
                    <Card interactive className="flex items-center justify-between px-4 py-2 text-sm">
                      <span className="text-muted">{ci.date}</span>
                      <span>
                        Á{ci.mood} · E{ci.energy}
                      </span>
                    </Card>
                  </button>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="mt-3 text-sm text-muted">Aún no hay check-ins.</p>
          )}
        </section>

        <DayDetailModal checkin={selected} onClose={() => setSelected(null)} />
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

function DayDetailModal({
  checkin,
  onClose,
}: {
  checkin: CheckIn | null;
  onClose: () => void;
}) {
  // Los compromisos de ese día: se piden solo cuando hay un día seleccionado.
  const dueQuery = useQuery({
    queryKey: ["commitments", "due", checkin?.date],
    queryFn: () => getDue(checkin!.date),
    enabled: !!checkin,
  });
  const commitments = dueQuery.data ?? [];
  const done = commitments.filter((c) => c.done).length;

  return (
    <Modal
      open={checkin !== null}
      onClose={onClose}
      title={checkin ? formatDay(checkin.date) : ""}
    >
      {checkin && (
        <div className="space-y-4 text-sm">
          <p className="font-bold">
            Ánimo {checkin.mood} · Energía {checkin.energy}
          </p>

          {DIMENSIONS.some((d) => checkin[d.key]) && (
            <div className="space-y-2">
              {DIMENSIONS.filter((d) => checkin[d.key]).map((d) => (
                <p key={d.key} className="flex items-start gap-2">
                  <Chip variant={d.variant}>{d.short}</Chip>
                  <span>
                    <span className="font-bold">{d.label}:</span> {checkin[d.key]}
                  </span>
                </p>
              ))}
            </div>
          )}

          {checkin.win && (
            <p>
              🏆 <span className="font-bold">Win:</span> {checkin.win}
            </p>
          )}
          {checkin.avoided && (
            <p>
              🚫 <span className="font-bold">Evité:</span> {checkin.avoided}
            </p>
          )}

          <div className="space-y-2 border-t-2 border-ink pt-3">
            <div className="flex items-center justify-between">
              <span className="font-display text-xs font-bold uppercase tracking-[0.12em] text-muted">
                📋 Compromisos
              </span>
              {commitments.length > 0 && (
                <span className="font-display text-xs font-bold text-accent">
                  {done}/{commitments.length} ✓
                </span>
              )}
            </div>
            {commitments.length === 0 ? (
              <p className="text-muted">Sin compromisos ese día.</p>
            ) : (
              commitments.map((c: Commitment) => (
                <p key={c.id} className="flex items-center gap-2">
                  <span>{c.done ? "✓" : "✗"}</span>
                  <span className={c.done ? "line-through text-muted" : ""}>
                    {c.text}
                  </span>
                </p>
              ))
            )}
          </div>
        </div>
      )}
    </Modal>
  );
}

// formatDay muestra "lun 16 jun" parseando YYYY-MM-DD como fecha LOCAL (evita el
// corrimiento de día de new Date("YYYY-MM-DD"), que interpreta UTC).
function formatDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  return date.toLocaleDateString("es", {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
}
