import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getToday, list, upsert, todayString, type CheckIn } from "@/lib/checkins";

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

  // Pre-rellena el formulario con el check-in de hoy si existe.
  useEffect(() => {
    const ci = todayQuery.data;
    if (ci) {
      setMood(ci.mood);
      setEnergy(ci.energy);
      setDiscipline(ci.discipline);
      setNote(ci.note);
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
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Check-in de hoy</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
        className="mt-6 space-y-6 rounded-xl border border-ink-700 bg-ink-900 p-6"
      >
        <Slider label="Ánimo" value={mood} onChange={setMood} />
        <Slider label="Energía" value={energy} onChange={setEnergy} />
        <Slider label="Disciplina" value={discipline} onChange={setDiscipline} />

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Nota</span>
          <textarea
            aria-label="Nota"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={3}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        {error && <p className="text-sm text-streak">{error}</p>}

        <button
          type="submit"
          disabled={mutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {mutation.isPending ? "Guardando…" : "Guardar"}
        </button>
      </form>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Historial</h2>
        {historyQuery.data && historyQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-2">
            {historyQuery.data.map((ci: CheckIn) => (
              <li
                key={ci.id}
                className="flex items-center justify-between rounded-lg border border-ink-700 bg-ink-900 px-4 py-2 text-sm"
              >
                <span className="text-sand-400">{ci.date}</span>
                <span>
                  Á{ci.mood} · E{ci.energy} · D{ci.discipline}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay check-ins.</p>
        )}
      </section>
    </div>
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
        <span className="text-sand-400">{label}</span>
        <span className="font-bold text-amber-brand">{value}</span>
      </span>
      <input
        type="range"
        min={1}
        max={10}
        step={1}
        aria-label={label}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-amber-brand"
      />
    </label>
  );
}
