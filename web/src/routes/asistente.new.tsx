import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { sendMessageStream } from "@/lib/ai";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/asistente/new")({ component: NewThreadPage });

function NewThreadPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [streaming, setStreaming] = useState<{ question: string; partial: string } | null>(null);

  const mutation = useMutation({
    mutationFn: (message: string) => {
      setStreaming({ question: message, partial: "" });
      return sendMessageStream(message, undefined, (delta) =>
        setStreaming((s) => (s ? { ...s, partial: s.partial + delta } : s))
      );
    },
    onSuccess: ({ threadId }) => {
      // El hilo recién creado pasa a ser la ruta canónica: replace para que
      // "atrás" no vuelva al chat vacío de /asistente/new.
      qc.invalidateQueries({ queryKey: ["ai-threads"] });
      navigate({ to: "/asistente/$threadId", params: { threadId }, replace: true });
    },
    onError: (err) => {
      // Nada se persistió: se descarta el parcial y el input conserva el texto.
      setStreaming(null);
      setError(err instanceof Error ? err.message : "No se pudo enviar");
    },
  });

  if (!user) return null;

  return (
    <PageTransition>
      <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
        <header className="flex items-center justify-between gap-2">
          <Link to="/asistente" className="font-bold underline decoration-accent decoration-2 underline-offset-2">
            ← Hilos
          </Link>
          <h1 className="font-display text-xl font-bold tracking-tight">Hilo nuevo</h1>
        </header>

        <section className="flex flex-col gap-2">
          {!streaming ? (
            <p className="text-sm text-muted">
              Pregúntame sobre tu día, tus finanzas o tus hábitos.
            </p>
          ) : (
            <>
              <div className="self-end rounded-lg border-2 border-ink bg-accent/30 px-3 py-2 text-sm shadow-brutal-sm">
                {streaming.question}
              </div>
              {streaming.partial === "" ? (
                <div className="self-start rounded-lg border-2 border-ink bg-surface px-3 py-2 text-sm shadow-brutal-sm text-muted">
                  Pensando…
                </div>
              ) : (
                <div className="self-start rounded-lg border-2 border-ink bg-surface px-3 py-2 text-sm shadow-brutal-sm">
                  {streaming.partial}
                </div>
              )}
            </>
          )}
        </section>

        {error && (
          <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
            {error}
          </p>
        )}

        <form
          onSubmit={(e) => {
            e.preventDefault();
            const t = text.trim();
            if (t) mutation.mutate(t);
          }}
          className="flex gap-2"
        >
          <Input
            type="text"
            aria-label="Mensaje"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Escribe tu pregunta…"
            className="flex-1"
          />
          <Button type="submit" disabled={mutation.isPending || text.trim() === ""}>
            Enviar
          </Button>
        </form>
      </div>
    </PageTransition>
  );
}
