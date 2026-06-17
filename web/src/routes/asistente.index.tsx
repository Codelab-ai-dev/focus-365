import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getThreads, searchChat, type Thread, type MessageHit, type ThreadHit } from "@/lib/ai";
import { Input } from "@/ui/Input";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/asistente/")({ component: ThreadListPage });

// useDebounced devuelve el valor tras `ms` sin cambios.
function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

function ThreadListPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const [query, setQuery] = useState("");
  const debounced = useDebounced(query.trim(), 250);
  const searching = debounced.length >= 2;

  const threadsQuery = useQuery({
    queryKey: ["ai-threads"],
    queryFn: getThreads,
    enabled: !!user && !searching,
  });
  const searchQuery = useQuery({
    queryKey: ["ai-search", debounced],
    queryFn: () => searchChat(debounced),
    enabled: !!user && searching,
  });

  if (!user) return null;
  const threads = threadsQuery.data ?? [];
  const results = searchQuery.data;

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

        <Input
          type="search"
          aria-label="Buscar en el chat"
          placeholder="Buscar…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />

        {searching ? (
          <SearchResultsView results={results} query={debounced} navigate={navigate} />
        ) : threads.length === 0 ? (
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

function SearchResultsView({
  results,
  query,
  navigate,
}: {
  results: { threads: ThreadHit[]; messages: MessageHit[] } | undefined;
  query: string;
  navigate: ReturnType<typeof useNavigate>;
}) {
  if (!results) return <p className="text-sm text-muted">Buscando…</p>;
  if (results.threads.length === 0 && results.messages.length === 0) {
    return <p className="text-sm text-muted">Sin resultados para «{query}».</p>;
  }
  const open = (threadId: string) => navigate({ to: "/asistente/$threadId", params: { threadId } });
  return (
    <div className="flex flex-col gap-4">
      {results.threads.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="font-display text-sm font-bold uppercase tracking-wide text-muted">Hilos</h2>
          {results.threads.map((t) => (
            <button
              key={t.id}
              onClick={() => open(t.id)}
              className="block w-full rounded-lg border-2 border-ink bg-surface px-3 py-2 text-left shadow-brutal-sm"
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="truncate font-bold">{t.title || "Sin título"}</span>
                <span className="shrink-0 text-xs text-muted">{relativeDate(t.updated_at)}</span>
              </div>
              {t.preview && <p className="truncate text-sm text-muted">{t.preview}</p>}
            </button>
          ))}
        </section>
      )}
      {results.messages.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="font-display text-sm font-bold uppercase tracking-wide text-muted">Mensajes</h2>
          {results.messages.map((m) => (
            <button
              key={m.id}
              onClick={() => open(m.thread_id)}
              className="block w-full rounded-lg border-2 border-ink bg-surface px-3 py-2 text-left shadow-brutal-sm"
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="truncate text-xs font-bold text-muted">{m.thread_title || "Sin título"}</span>
                <span className="shrink-0 text-xs text-muted">{m.role === "user" ? "Vos" : "Asistente"}</span>
              </div>
              <p className="text-sm">{highlight(m.content, query)}</p>
            </button>
          ))}
        </section>
      )}
    </div>
  );
}

// highlight resalta (best-effort, case-insensitive) la primera aparición del
// término. Si no la encuentra (p.ej. por acentos), devuelve el texto tal cual.
function highlight(text: string, query: string): ReactNode {
  const i = text.toLowerCase().indexOf(query.toLowerCase());
  if (i < 0) return text;
  return (
    <>
      {text.slice(0, i)}
      <mark className="bg-accent">{text.slice(i, i + query.length)}</mark>
      {text.slice(i + query.length)}
    </>
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
