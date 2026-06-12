import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getMessages, sendMessageStream, confirmAction, cancelAction, type Message } from "@/lib/ai";

export const Route = createFileRoute("/asistente")({ component: AsistentePage });

const ACTION_TITLES: Record<string, string> = {
  checkin: "Check-in de hoy",
  movimiento: "Movimiento",
  habito: "Hábito",
  meta: "Meta",
};

function actionDetails(action: NonNullable<Message["action"]>): string {
  const p = action.payload as Record<string, unknown>;
  switch (action.kind) {
    case "checkin":
      return `Ánimo ${p.mood} · Energía ${p.energy} · Disciplina ${p.discipline}`;
    case "movimiento":
      return `${p.type === "income" ? "Ingreso" : "Gasto"} de $${(Number(p.amount_centavos) / 100).toFixed(2)} en ${p.category}`;
    case "habito":
      return "Marcar como hecho hoy";
    case "meta":
      return `Progreso al ${p.progress}%`;
    default:
      return "";
  }
}

function ActionCard({
  message,
  pending,
  onResolve,
}: {
  message: Message;
  pending: boolean;
  onResolve: (id: string, verb: "confirm" | "cancel") => void;
}) {
  const action = message.action!;
  return (
    <div className="mt-2 rounded-lg border border-amber-brand/40 bg-ink-800 p-3 text-sm">
      <p className="font-bold">{ACTION_TITLES[action.kind] ?? "Acción"}</p>
      <p className="text-sand-400">{actionDetails(action)}</p>
      {action.status === "proposed" && (
        <div className="mt-2 flex gap-2">
          <button
            onClick={() => onResolve(message.id, "confirm")}
            disabled={pending}
            className="rounded-lg bg-amber-brand px-3 py-1 text-xs font-bold text-ink-950 disabled:opacity-60"
          >
            Confirmar
          </button>
          <button
            onClick={() => onResolve(message.id, "cancel")}
            disabled={pending}
            className="rounded-lg border border-ink-700 px-3 py-1 text-xs disabled:opacity-60"
          >
            Cancelar
          </button>
        </div>
      )}
      {action.status === "done" && <p className="mt-2 text-xs font-bold text-amber-brand">✓ Hecha</p>}
      {action.status === "cancelled" && <p className="mt-2 text-xs text-sand-400">Cancelada</p>}
    </div>
  );
}

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
  const [streaming, setStreaming] = useState<{ question: string; partial: string } | null>(null);

  const actionMutation = useMutation({
    mutationFn: ({ id, verb }: { id: string; verb: "confirm" | "cancel" }) =>
      verb === "confirm" ? confirmAction(id) : cancelAction(id),
    onSuccess: (updated) => {
      setError(null);
      qc.setQueryData<Message[]>(["ai-messages"], (prev) =>
        (prev ?? []).map((m) => (m.id === updated.id ? updated : m))
      );
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "No se pudo resolver la acción"),
  });

  const mutation = useMutation({
    mutationFn: (message: string) => {
      setStreaming({ question: message, partial: "" });
      return sendMessageStream(message, (delta) =>
        setStreaming((s) => (s ? { ...s, partial: s.partial + delta } : s))
      );
    },
    onSuccess: (reply, message) => {
      setError(null);
      setText("");
      // Actualizar el caché optimistamente con la pregunta y la respuesta
      // persistida antes de quitar las burbujas — evita el parpadeo.
      qc.setQueryData<Message[]>(["ai-messages"], (prev) => [
        ...(prev ?? []),
        { id: "", role: "user", content: message, created_at: reply.created_at },
        reply,
      ]);
      setStreaming(null);
    },
    onError: (err) => {
      // Nada se persistió: se descarta el parcial y el input conserva el texto.
      setStreaming(null);
      setError(err instanceof Error ? err.message : "No se pudo enviar");
    },
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
        {messages.length === 0 && !streaming ? (
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
              {m.role === "assistant" && m.action && (
                <ActionCard
                  message={m}
                  pending={actionMutation.isPending}
                  onResolve={(id, verb) => actionMutation.mutate({ id, verb })}
                />
              )}
            </div>
          ))
        )}
        {streaming && (
          <>
            <div className="self-end rounded-lg bg-amber-brand/20 px-3 py-2 text-sm text-sand-100">
              {streaming.question}
            </div>
            {streaming.partial === "" ? (
              <div className="self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-400">
                Pensando…
              </div>
            ) : (
              <div className="self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-100">
                {streaming.partial}
              </div>
            )}
          </>
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
