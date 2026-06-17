import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getThreads, type Thread } from "@/lib/ai";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/asistente/")({ component: ThreadListPage });

function ThreadListPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const threadsQuery = useQuery({ queryKey: ["ai-threads"], queryFn: getThreads, enabled: !!user });
  if (!user) return null;
  const threads = threadsQuery.data ?? [];

  return (
    <PageTransition>
      <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Asistente</h1>
          <div className="flex items-center gap-3">
            <Link
              to="/asistente/new"
              aria-label="Nuevo hilo"
              className="rounded-md border-2 border-ink bg-accent px-3 py-1 font-bold shadow-brutal-sm"
            >
              + Nuevo
            </Link>
            <Link to="/" className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2">
              Volver
            </Link>
          </div>
        </header>

        {threads.length === 0 ? (
          <p className="text-sm text-muted">Todavía no tenés hilos. Empezá uno nuevo.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {threads.map((t: Thread) => (
              <li key={t.id}>
                <Link
                  to="/asistente/$threadId"
                  params={{ threadId: t.id }}
                  className="block rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm"
                >
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="truncate font-bold">{t.title || "Sin título"}</span>
                    <span className="shrink-0 text-xs text-muted">{relativeDate(t.updated_at)}</span>
                  </div>
                  {t.preview && <p className="truncate text-sm text-muted">{t.preview}</p>}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </PageTransition>
  );
}

// relativeDate: "hoy"/"Nd"/fecha local. No existe un helper de fechas relativas
// en web/src/lib, así que se define aquí.
function relativeDate(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  if (days <= 0) return "hoy";
  if (days < 7) return `${days}d`;
  return d.toLocaleDateString();
}
