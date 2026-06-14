import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getDashboard, todayString, type Snapshot } from "@/lib/dashboard";
import { formatMXN } from "@/lib/finances";
import { getInsight } from "@/lib/ai";
import { Card } from "@/ui/Card";
import { Chip } from "@/ui/Chip";
import { Stat } from "@/ui/Stat";
import { Button } from "@/ui/Button";
import { PageTransition } from "@/ui/PageTransition";
import { Reveal, RevealItem } from "@/ui/Reveal";

export const Route = createFileRoute("/")({ component: DashboardPage });

function DashboardPage() {
  const { user } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const query = useQuery({
    queryKey: ["dashboard", todayString()],
    queryFn: getDashboard,
    enabled: !!user,
  });

  if (!user) return null;

  if (query.isLoading) {
    return <p className="p-6 text-muted">Cargando tu día…</p>;
  }

  if (query.isError || !query.data) {
    return (
      <div className="p-6">
        <p className="w-fit rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
          No pudimos cargar tu día.
        </p>
        <Button variant="ghost" className="mt-3" onClick={() => query.refetch()}>
          Reintentar
        </Button>
      </div>
    );
  }

  const s = query.data;
  const fecha = new Date().toLocaleDateString("es-MX", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });

  return (
    <PageTransition>
      <div className="mx-auto max-w-3xl p-6">
        <AIBand />
        <p className="mt-4 text-sm text-muted">
          Hola, <span className="font-bold text-ink">{user.name}</span> · {fecha} ·{" "}
          {s.dimensions_active} dimensiones en marcha
        </p>

        <Reveal className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <RevealItem>
            <StreakHero s={s} />
          </RevealItem>
          <RevealItem>
            <FinanceCard s={s} />
          </RevealItem>
        </Reveal>

        <Reveal className="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <RevealItem><MoodCard s={s} /></RevealItem>
          <RevealItem><CheckinCard s={s} /></RevealItem>
          <RevealItem><TrainingCard s={s} /></RevealItem>
          <RevealItem><GoalsCard s={s} /></RevealItem>
        </Reveal>
      </div>
    </PageTransition>
  );
}

function AIBand() {
  const { user } = useAuth();
  const insightQ = useQuery({
    queryKey: ["ai-insight", todayString()],
    queryFn: getInsight,
    enabled: !!user,
    // Si la IA falla, degradamos al placeholder sin reintentar: la banda nunca
    // debe quedarse cargando ni golpear repetidamente un endpoint caído.
    retry: false,
  });

  let content = "✦ Tu insight del día llega pronto";
  if (insightQ.isLoading) {
    content = "✦ Generando tu insight…";
  } else if (insightQ.data?.available && insightQ.data.content) {
    content = `✦ ${insightQ.data.content}`;
  }
  return (
    <Link to="/asistente" className="block">
      <Card interactive className="flex items-center gap-3 px-4 py-3">
        <Chip variant="accent">IA</Chip>
        <span className="text-sm font-bold">{content}</span>
      </Card>
    </Link>
  );
}

// Tile envuelve Card interactiva en un Link; bg permite pintar el tile entero
// con un chip-color (los hijos ponen el texto con su fg correspondiente).
function Tile({
  to,
  title,
  bg = "",
  children,
}: {
  to: string;
  title: string;
  bg?: string;
  children: React.ReactNode;
}) {
  return (
    <Link to={to} className="block h-full">
      <Card interactive className={`flex h-full min-h-[72px] flex-col gap-1 p-4 ${bg}`}>
        <span className="text-[10px] font-bold uppercase tracking-[0.12em] opacity-70">
          {title}
        </span>
        {children}
      </Card>
    </Link>
  );
}

function StreakHero({ s }: { s: Snapshot }) {
  return (
    <Link to="/disciplina" className="block h-full">
      <Card interactive className="flex h-full flex-col justify-between bg-accent p-5 text-[#16130e]">
        <span className="text-[10px] font-bold uppercase tracking-[0.12em] opacity-70">
          🔥 Racha
        </span>
        {s.streak.total === 0 ? (
          <span className="text-sm font-medium opacity-80">Sin hábitos aún</span>
        ) : (
          <>
            <div className="flex items-end gap-2">
              <Stat label="" value={s.streak.best_current} suffix=" días" hideLabel />
              <span className="animate-flicker text-2xl">🔥</span>
            </div>
            <span className="text-xs font-bold opacity-80">
              {s.streak.done_today}/{s.streak.total} hábitos hoy
            </span>
          </>
        )}
      </Card>
    </Link>
  );
}

function FinanceCard({ s }: { s: Snapshot }) {
  const bg =
    s.finance.status === "verde"
      ? "bg-money-bg text-money-fg"
      : s.finance.status === "rojo"
        ? "bg-danger-bg text-danger-fg"
        : "";
  return (
    <Tile to="/finanzas" title="Superávit del ciclo" bg={bg}>
      <Stat
        label=""
        value={s.finance.net}
        format={formatMXN}
        hideLabel
      />
      <span className="text-xs font-bold opacity-70">
        {s.finance.cycle} · {s.finance.status}
      </span>
    </Tile>
  );
}

function Bar({ value }: { value: number }) {
  // value 1-10 → ancho proporcional.
  return (
    <div className="h-2 w-full overflow-hidden rounded-md border-2 border-ink bg-surface">
      <div className="h-full bg-accent" style={{ width: `${value * 10}%` }} />
    </div>
  );
}

function MoodCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/check-in" title="Ánimo / Energía" bg="bg-sky-bg text-sky-fg">
      {s.checkin == null ? (
        <span className="text-xs opacity-80">Sin check-in hoy</span>
      ) : (
        <div className="flex flex-col gap-1">
          <Bar value={s.checkin.mood} />
          <Bar value={s.checkin.energy} />
        </div>
      )}
    </Tile>
  );
}

function CheckinCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/check-in" title="Check-in de hoy" bg="bg-sun-bg text-sun-fg">
      {s.checkin?.present ? (
        <span className="text-xs font-bold">Hecho ✓{s.checkin.win ? ` · ${s.checkin.win}` : ""}</span>
      ) : (
        <span className="text-xs opacity-80">Pendiente</span>
      )}
    </Tile>
  );
}

function TrainingCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/entrenamiento" title="Entreno de hoy">
      <span className="text-xs font-bold text-muted">
        {s.training.trained_today ? `${s.training.type} ✓` : "Sin entreno hoy"}
      </span>
    </Tile>
  );
}

function GoalsCard({ s }: { s: Snapshot }) {
  return (
    <Tile to="/metas" title="Metas activas">
      <span className="text-xs font-bold text-muted">
        {s.goals.active} activas · {s.goals.avg_progress}% prom.
      </span>
      {s.goals.overdue > 0 && (
        <Chip variant="danger" className="mt-1 w-fit">
          {s.goals.overdue} vencida(s)
        </Chip>
      )}
    </Tile>
  );
}
