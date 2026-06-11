import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getDashboard, todayString, type Snapshot } from "@/lib/dashboard";
import { formatMXN } from "@/lib/finances";
import { getInsight } from "@/lib/ai";

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
    return <p className="p-6 text-sand-400">Cargando tu día…</p>;
  }

  if (query.isError || !query.data) {
    return (
      <div className="p-6">
        <p className="text-streak">No pudimos cargar tu día.</p>
        <button
          onClick={() => query.refetch()}
          className="mt-3 rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
        >
          Reintentar
        </button>
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
    <div className="p-6">
      <AIBand />
      <p className="mt-4 text-sm text-sand-400">
        Hola, <span className="text-amber-brand">{user.name}</span> · {fecha} ·{" "}
        {s.dimensions_active} dimensiones en marcha
      </p>

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        <StreakCard s={s} />
        <FinanceCard s={s} />
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <MoodCard s={s} />
        <CheckinCard s={s} />
        <TrainingCard s={s} />
        <GoalsCard s={s} />
      </div>
    </div>
  );
}

function AIBand() {
  const { user } = useAuth();
  const insightQ = useQuery({
    queryKey: ["ai-insight", todayString()],
    queryFn: getInsight,
    enabled: !!user,
  });

  const base =
    "rounded-lg border border-dashed border-amber-brand bg-amber-brand/10 px-4 py-3 text-sm font-bold text-amber-brand";

  if (insightQ.isLoading) {
    return <div className={base}>✦ Generando tu insight…</div>;
  }
  if (insightQ.data?.available && insightQ.data.content) {
    return <div className={base}>✦ {insightQ.data.content}</div>;
  }
  return <div className={base}>✦ Tu insight del día llega pronto</div>;
}

function Card({
  to,
  title,
  big,
  children,
}: {
  to: string;
  title: string;
  big?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      to={to}
      className={`flex flex-col gap-1 rounded-lg border border-ink-700 bg-ink-800 p-4 ${
        big ? "min-h-[88px]" : "min-h-[64px]"
      }`}
    >
      <span className="text-sm font-bold text-sand-100">{title}</span>
      {children}
    </Link>
  );
}

function StreakCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/disciplina" title="🔥 Racha" big>
      {s.streak.total === 0 ? (
        <span className="text-sm text-sand-400">Sin hábitos aún</span>
      ) : (
        <>
          <span className="text-2xl font-extrabold text-streak">
            {s.streak.best_current} días
          </span>
          <span className="text-xs text-sand-400">
            {s.streak.done_today}/{s.streak.total} hábitos hoy
          </span>
        </>
      )}
    </Card>
  );
}

function FinanceCard({ s }: { s: Snapshot }) {
  const color =
    s.finance.status === "verde"
      ? "text-money"
      : s.finance.status === "rojo"
        ? "text-streak"
        : "text-sand-400";
  return (
    <Card to="/finanzas" title="Superávit del ciclo" big>
      <span className={`text-2xl font-extrabold ${color}`}>{formatMXN(s.finance.net)}</span>
      <span className="text-xs text-sand-400">
        {s.finance.cycle} · {s.finance.status}
      </span>
    </Card>
  );
}

function Bar({ value }: { value: number }) {
  // value 1-10 → ancho proporcional.
  return (
    <div className="h-2 w-full rounded bg-ink-700">
      <div className="h-2 rounded bg-amber-brand" style={{ width: `${value * 10}%` }} />
    </div>
  );
}

function MoodCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/check-in" title="Ánimo / Energía">
      {s.checkin == null ? (
        <span className="text-xs text-sand-400">Sin check-in hoy</span>
      ) : (
        <div className="flex flex-col gap-1">
          <Bar value={s.checkin.mood} />
          <Bar value={s.checkin.energy} />
        </div>
      )}
    </Card>
  );
}

function CheckinCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/check-in" title="Check-in de hoy">
      {s.checkin?.present ? (
        <span className="text-xs text-money">
          Hecho ✓ · disciplina {s.checkin.discipline}
        </span>
      ) : (
        <span className="text-xs text-sand-400">Pendiente</span>
      )}
    </Card>
  );
}

function TrainingCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/entrenamiento" title="Entreno de hoy">
      <span className="text-xs text-sand-400">
        {s.training.trained_today ? `${s.training.type} ✓` : "Sin entreno hoy"}
      </span>
    </Card>
  );
}

function GoalsCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/metas" title="Metas activas">
      <span className="text-xs text-sand-400">
        {s.goals.active} activas · {s.goals.avg_progress}% prom.
      </span>
      {s.goals.overdue > 0 && (
        <span className="text-xs text-streak">{s.goals.overdue} vencida(s)</span>
      )}
    </Card>
  );
}
