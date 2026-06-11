import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getMessages, sendMessage, type Message } from "@/lib/ai";

export const Route = createFileRoute("/asistente")({ component: AsistentePage });

function AsistentePage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const historyQuery = useQuery({
    queryKey: ["ai-messages"],
    queryFn: getMessages,
    enabled: !!user,
  });

  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: (message: string) => sendMessage(message),
    onSuccess: () => {
      setError(null);
      setText("");
      qc.invalidateQueries({ queryKey: ["ai-messages"] });
    },
    onError: (err) =>
      // No limpiamos el input: el usuario puede reintentar sin reescribir.
      setError(err instanceof Error ? err.message : "No se pudo enviar"),
  });

  if (!user) return null;

  const messages = historyQuery.data ?? [];

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Asistente</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      <section className="flex flex-col gap-2">
        {messages.length === 0 ? (
          <p className="text-sm text-sand-400">
            Pregúntame sobre tu día, tus finanzas o tus hábitos.
          </p>
        ) : (
          messages.map((m: Message, i: number) => (
            <div
              key={i}
              className={
                m.role === "user"
                  ? "self-end rounded-lg bg-amber-brand/20 px-3 py-2 text-sm text-sand-100"
                  : "self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-100"
              }
            >
              {m.content}
            </div>
          ))
        )}
        {mutation.isPending && (
          <div className="self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-400">
            Pensando…
          </div>
        )}
      </section>

      {error && <p className="text-sm text-streak">{error}</p>}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          const t = text.trim();
          if (t) mutation.mutate(t);
        }}
        className="flex gap-2"
      >
        <input
          type="text"
          aria-label="Mensaje"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Escribe tu pregunta…"
          className="flex-1 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
        />
        <button
          type="submit"
          disabled={mutation.isPending || text.trim() === ""}
          className="rounded-lg bg-amber-brand px-4 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          Enviar
        </button>
      </form>
    </div>
  );
}
