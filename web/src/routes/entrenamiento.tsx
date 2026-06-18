import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
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
import { Modal } from "@/ui/Modal";
import { getProfile, saveProfile, type FitnessProfile } from "@/lib/fitnessProfile";
import { getSuggestion, generateSuggestion, type TrainingSuggestion } from "@/lib/trainingSuggestion";
import { getAdjustment, generateAdjustment, type TrainingAdjustment } from "@/lib/trainingAdjustment";
import { PageTransition } from "@/ui/PageTransition";
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { Chip } from "@/ui/Chip";
import { Reveal, RevealItem } from "@/ui/Reveal";
import { BarChart } from "@/ui/BarChart";
import {
  weeklyVolume,
  weeklyFrequency,
  exerciseNames,
  exerciseProgression,
  personalRecords,
} from "@/lib/trainingProgress";

export const Route = createFileRoute("/entrenamiento")({ component: EntrenamientoPage });

type SetRow = { exercise: string; reps: string; weightKg: string; note: string };

function emptyRow(): SetRow {
  return { exercise: "", reps: "", weightKg: "", note: "" };
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
  const [profileOpen, setProfileOpen] = useState(false);
  const [focus, setFocus] = useState("");
  const [suggestError, setSuggestError] = useState<string | null>(null);
  const [adjustScope, setAdjustScope] = useState<"last" | "week">("last");
  const [adjustError, setAdjustError] = useState<string | null>(null);
  const [progressExercise, setProgressExercise] = useState("");

  const workouts = historyQuery.data ?? [];
  const volume = useMemo(() => weeklyVolume(workouts, 12), [workouts]);
  const frequency = useMemo(() => weeklyFrequency(workouts, 12), [workouts]);
  const names = useMemo(() => exerciseNames(workouts), [workouts]);
  const selectedExercise =
    progressExercise && names.includes(progressExercise) ? progressExercise : names[0] ?? "";
  const progression = useMemo(
    () => (selectedExercise ? exerciseProgression(workouts, selectedExercise, 12) : []),
    [workouts, selectedExercise]
  );
  const prs = useMemo(() => personalRecords(workouts), [workouts]);

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
            note: r.note.trim(),
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

  const suggestionQuery = useQuery({
    queryKey: ["training-suggestion"],
    queryFn: getSuggestion,
    enabled: !!user,
  });
  const suggestMutation = useMutation({
    mutationFn: () => generateSuggestion(focus.trim()),
    onSuccess: (s) => {
      setSuggestError(null);
      qc.setQueryData<TrainingSuggestion | null>(["training-suggestion"], s);
    },
    onError: (e) =>
      setSuggestError(e instanceof Error ? e.message : "No se pudo generar la sugerencia"),
  });

  const adjustmentQuery = useQuery({
    queryKey: ["training-adjustment"],
    queryFn: getAdjustment,
    enabled: !!user,
  });
  const adjustMutation = useMutation({
    mutationFn: () => generateAdjustment(adjustScope),
    onSuccess: (a) => {
      setAdjustError(null);
      qc.setQueryData<TrainingAdjustment | null>(["training-adjustment"], a);
    },
    onError: (e) =>
      setAdjustError(e instanceof Error ? e.message : "No se pudo generar el análisis"),
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
          <div className="flex items-center gap-3">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setProfileOpen(true)}
              className="px-3 py-1 text-xs"
            >
              Mi perfil
            </Button>
            <Link
              to="/"
              className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2 text-sm"
            >
              Volver
            </Link>
          </div>
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
                <div key={i} className="space-y-1">
                  {/* El nombre del ejercicio en su propia línea (ancho completo). */}
                  <Input
                    type="text"
                    aria-label={`Ejercicio ${i + 1}`}
                    list="catalogo-ejercicios"
                    placeholder="Ejercicio"
                    value={row.exercise}
                    onChange={(e) => updateRow(i, { exercise: e.target.value })}
                  />
                  <div className="flex gap-2">
                    <Input
                      type="number"
                      aria-label={`Reps ${i + 1}`}
                      placeholder="Reps"
                      min="0"
                      value={row.reps}
                      onChange={(e) => updateRow(i, { reps: e.target.value })}
                      className="flex-1"
                    />
                    <Input
                      type="number"
                      aria-label={`Peso ${i + 1}`}
                      placeholder="kg"
                      min="0"
                      step="0.5"
                      value={row.weightKg}
                      onChange={(e) => updateRow(i, { weightKg: e.target.value })}
                      className="flex-1"
                    />
                  </div>
                  <Input
                    type="text"
                    aria-label={`Nota serie ${i + 1}`}
                    placeholder="nota de la serie (opcional)"
                    value={row.note}
                    onChange={(e) => updateRow(i, { note: e.target.value })}
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

        <Card className="mt-8 p-6 space-y-3">
          <h2 className="font-display text-lg font-bold tracking-tight">Entrenador IA</h2>
          <div className="flex gap-2">
            <Input
              type="text"
              aria-label="Enfoque"
              placeholder="enfoque opcional: pierna, 30 min, sin saltos…"
              value={focus}
              onChange={(e) => setFocus(e.target.value)}
              className="flex-1"
            />
            <Button
              type="button"
              onClick={() => suggestMutation.mutate()}
              disabled={suggestMutation.isPending}
              className="px-3 py-1 text-xs"
            >
              {suggestMutation.isPending ? "Generando…" : "Sugerir"}
            </Button>
          </div>
          {suggestError && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">
              {suggestError}
            </p>
          )}
          {suggestionQuery.data ? (
            <div className="rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
              <p className="whitespace-pre-wrap text-sm">{suggestionQuery.data.content}</p>
              {suggestionQuery.data.created_at && (
                <p className="mt-2 text-[10px] uppercase tracking-[0.12em] text-muted">
                  {suggestionQuery.data.focus ? `enfoque: ${suggestionQuery.data.focus} · ` : ""}
                  {relativeDateTraining(suggestionQuery.data.created_at)}
                </p>
              )}
            </div>
          ) : (
            !suggestMutation.isPending && (
              <p className="text-sm text-muted">Pedí una sugerencia de entrenamiento.</p>
            )
          )}
        </Card>

        <Card className="mt-8 p-6 space-y-3">
          <h2 className="font-display text-lg font-bold tracking-tight">Análisis del agente</h2>
          <div className="flex flex-wrap items-center gap-2">
            {([
              { v: "last", label: "Último entreno" },
              { v: "week", label: "Última semana" },
            ] as const).map((o) => (
              <button
                key={o.v}
                type="button"
                aria-pressed={adjustScope === o.v}
                onClick={() => setAdjustScope(o.v)}
                className={`rounded-lg border-2 border-ink px-3 py-1 text-xs font-bold shadow-brutal-sm ${adjustScope === o.v ? "bg-accent" : "bg-surface"}`}
              >
                {o.label}
              </button>
            ))}
            <Button
              type="button"
              onClick={() => adjustMutation.mutate()}
              disabled={adjustMutation.isPending}
              className="px-3 py-1 text-xs"
            >
              {adjustMutation.isPending ? "Analizando…" : "Analizar"}
            </Button>
          </div>
          {adjustError && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">
              {adjustError}
            </p>
          )}
          {adjustmentQuery.data ? (
            <div className="rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
              <p className="whitespace-pre-wrap text-sm">{adjustmentQuery.data.content}</p>
              {adjustmentQuery.data.created_at && (
                <p className="mt-2 text-[10px] uppercase tracking-[0.12em] text-muted">
                  {adjustmentQuery.data.scope === "week" ? "última semana" : "último entreno"} · {relativeDateTraining(adjustmentQuery.data.created_at)}
                </p>
              )}
            </div>
          ) : (
            !adjustMutation.isPending && (
              <p className="text-sm text-muted">Pedí un análisis de tus entrenos recientes.</p>
            )
          )}
        </Card>

        <section className="mt-8">
          <h2 className="font-display text-lg font-bold tracking-tight">Historial</h2>
          {workouts.length > 0 ? (
            <Reveal className="mt-3 space-y-3">
              {workouts.map((w: Workout) => (
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
                          {s.note && (
                            <span className="block text-xs text-muted">{s.note}</span>
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

        <section className="mt-8">
          <h2 className="font-display text-lg font-bold tracking-tight">Progreso</h2>
          {workouts.length === 0 ? (
            <p className="mt-3 text-sm text-muted">Registrá entrenos para ver tu progreso.</p>
          ) : (
            <div className="mt-3 space-y-4">
              <Card className="p-4">
                <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Volumen por semana (kg·reps)</h3>
                <BarChart data={volume} className="mt-2" />
              </Card>
              <Card className="p-4">
                <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Frecuencia por semana</h3>
                <BarChart data={frequency} className="mt-2" />
              </Card>
              {names.length > 0 && (
                <Card className="p-4 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Progresión (peso máx.)</h3>
                    <select
                      aria-label="Ejercicio"
                      value={selectedExercise}
                      onChange={(e) => setProgressExercise(e.target.value)}
                      className="rounded-lg border-2 border-ink bg-surface px-2 py-1 text-xs"
                    >
                      {names.map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                    </select>
                  </div>
                  <BarChart data={progression} unit="kg" />
                </Card>
              )}
              {prs.length > 0 && (
                <Card className="p-4">
                  <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-muted">Records</h3>
                  <ul className="mt-2 space-y-1 text-sm">
                    {prs.map((p) => (
                      <li key={p.exercise} className="flex justify-between">
                        <span>{p.exercise}</span>
                        <span className="font-bold">{p.weightKg} kg</span>
                      </li>
                    ))}
                  </ul>
                </Card>
              )}
            </div>
          )}
        </section>

        <ProfileModal open={profileOpen} onClose={() => setProfileOpen(false)} />
      </div>
    </PageTransition>
  );
}

const SEXES = [
  { value: "masculino", label: "Masculino" },
  { value: "femenino", label: "Femenino" },
  { value: "otro", label: "Otro" },
];
const OBJECTIVES = [
  { value: "perder_grasa", label: "Perder grasa" },
  { value: "hipertrofia", label: "Hipertrofia" },
  { value: "fuerza", label: "Fuerza" },
  { value: "resistencia", label: "Resistencia" },
  { value: "salud", label: "Salud general" },
];
const LOCATIONS = [
  { value: "casa", label: "Casa" },
  { value: "gym", label: "Gimnasio" },
  { value: "ambos", label: "Ambos" },
];
const LEVELS = [
  { value: "principiante", label: "Principiante" },
  { value: "intermedio", label: "Intermedio" },
  { value: "avanzado", label: "Avanzado" },
];
const EQUIPMENT = [
  { value: "peso_corporal", label: "Peso corporal" },
  { value: "mancuernas", label: "Mancuernas" },
  { value: "barra", label: "Barra" },
  { value: "banco", label: "Banco" },
  { value: "bandas", label: "Bandas" },
  { value: "kettlebell", label: "Kettlebell" },
  { value: "dominadas", label: "Barra de dominadas" },
  { value: "gym", label: "Gimnasio completo" },
];

function ProfileModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const profileQuery = useQuery<FitnessProfile | null>({
    queryKey: ["fitness-profile"],
    queryFn: getProfile,
    enabled: open,
  });

  const [birthdate, setBirthdate] = useState("");
  const [sex, setSex] = useState("");
  const [heightCm, setHeightCm] = useState("");
  const [weightKg, setWeightKg] = useState("");
  const [objective, setObjective] = useState("");
  const [location, setLocation] = useState("");
  const [level, setLevel] = useState("");
  const [weeklyDays, setWeeklyDays] = useState("");
  const [equipment, setEquipment] = useState<string[]>([]);
  const [limitations, setLimitations] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Precarga el form cuando llega el perfil (o lo deja en blanco si es null).
  useEffect(() => {
    if (!open) return;
    const p = profileQuery.data;
    setBirthdate(p?.birthdate ?? "");
    setSex(p?.sex ?? "");
    setHeightCm(p?.height_cm != null ? String(p.height_cm) : "");
    setWeightKg(p?.weight_grams != null ? String(gramsToKg(p.weight_grams)) : "");
    setObjective(p?.objective ?? "");
    setLocation(p?.location ?? "");
    setLevel(p?.level ?? "");
    setWeeklyDays(p?.weekly_days != null ? String(p.weekly_days) : "");
    setEquipment(p?.equipment ?? []);
    setLimitations(p?.limitations ?? "");
  }, [open, profileQuery.data]);

  const saveMutation = useMutation({
    mutationFn: () =>
      saveProfile({
        birthdate: birthdate || null,
        sex: sex || null,
        height_cm: heightCm ? Number(heightCm) : null,
        weight_grams: weightKg ? kgToGrams(Number(weightKg)) : null,
        objective: objective || null,
        location: location || null,
        level: level || null,
        weekly_days: weeklyDays ? Number(weeklyDays) : null,
        equipment,
        limitations,
      }),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["fitness-profile"] });
      onClose();
    },
    onError: (e) => setError(e instanceof Error ? e.message : "No se pudo guardar"),
  });

  const toggleEquip = (v: string) =>
    setEquipment((prev) => (prev.includes(v) ? prev.filter((x) => x !== v) : [...prev, v]));

  const selectCls = "w-full rounded-lg border-2 border-ink bg-surface px-2 py-1";

  return (
    <Modal open={open} onClose={onClose} title="Mi perfil">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          saveMutation.mutate();
        }}
        className="space-y-3 text-sm"
      >
        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Fecha de nacimiento</span>
          <input type="date" aria-label="Fecha de nacimiento" value={birthdate}
            onChange={(e) => setBirthdate(e.target.value)} className={selectCls} />
        </label>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Sexo</span>
          <select aria-label="Sexo" value={sex} onChange={(e) => setSex(e.target.value)} className={selectCls}>
            <option value="">—</option>
            {SEXES.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </label>

        <div className="flex gap-2">
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Altura (cm)</span>
            <input type="number" aria-label="Altura (cm)" value={heightCm} min={1}
              onChange={(e) => setHeightCm(e.target.value)} className={selectCls} />
          </label>
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Peso (kg)</span>
            <input type="number" aria-label="Peso (kg)" value={weightKg} min={1} step={0.1}
              onChange={(e) => setWeightKg(e.target.value)} className={selectCls} />
          </label>
        </div>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Objetivo</span>
          <select aria-label="Objetivo" value={objective} onChange={(e) => setObjective(e.target.value)} className={selectCls}>
            <option value="">—</option>
            {OBJECTIVES.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </label>

        <div className="flex gap-2">
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Lugar</span>
            <select aria-label="Lugar" value={location} onChange={(e) => setLocation(e.target.value)} className={selectCls}>
              <option value="">—</option>
              {LOCATIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>
          </label>
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Nivel</span>
            <select aria-label="Nivel" value={level} onChange={(e) => setLevel(e.target.value)} className={selectCls}>
              <option value="">—</option>
              {LEVELS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>
          </label>
        </div>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Días por semana</span>
          <input type="number" aria-label="Días por semana" value={weeklyDays} min={1} max={7}
            onChange={(e) => setWeeklyDays(e.target.value)} className={selectCls} />
        </label>

        <fieldset className="space-y-1">
          <legend className="text-xs font-bold text-muted">Equipo</legend>
          <div className="flex flex-wrap gap-2">
            {EQUIPMENT.map((o) => (
              <button key={o.value} type="button" onClick={() => toggleEquip(o.value)}
                aria-pressed={equipment.includes(o.value)}
                className={`rounded-lg border-2 border-ink px-2 py-1 text-xs font-bold shadow-brutal-sm ${equipment.includes(o.value) ? "bg-accent" : "bg-surface"}`}>
                {o.label}
              </button>
            ))}
          </div>
        </fieldset>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Lesiones / limitaciones</span>
          <textarea aria-label="Lesiones o limitaciones" value={limitations} rows={2}
            onChange={(e) => setLimitations(e.target.value)} className={selectCls} />
        </label>

        {error && (
          <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">{error}</p>
        )}

        <Button type="submit" disabled={saveMutation.isPending} className="w-full">
          {saveMutation.isPending ? "Guardando…" : "Guardar"}
        </Button>
      </form>
    </Modal>
  );
}

function relativeDateTraining(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  if (days <= 0) return "hoy";
  if (days < 7) return `${days}d`;
  return d.toLocaleDateString();
}
