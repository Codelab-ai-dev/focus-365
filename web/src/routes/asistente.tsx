import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getMessages, sendMessageStream, confirmAction, cancelAction, undoAction, type Message, type Action } from "@/lib/ai";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { Chip } from "@/ui/Chip";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/asistente")({ component: AsistentePage });

const ACTION_TITLES: Record<string, string> = {
  checkin: "Check-in de hoy",
  movimiento: "Movimiento",
  habito: "Hábito",
  meta: "Meta",
  habito_nuevo: "Nuevo hábito",
  meta_nueva: "Nueva meta",
  entrenamiento: "Entrenamiento",
};

function actionDetails(action: Action): string {
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
    case "habito_nuevo":
      return `${p.name}${p.target_days ? ` · objetivo ${p.target_days} días` : ""}`;
    case "meta_nueva":
      return `${p.title} · ${p.dimension}${p.deadline ? ` · para ${p.deadline}` : ""}`;
    case "entrenamiento": {
      const sets = (p.sets as Array<Record<string, unknown>>) ?? [];
      const detalle = sets
        .map((s) => `${s.exercise}${s.reps ? ` ×${s.reps}` : ""}${s.weight_kg ? ` @${s.weight_kg}kg` : ""}`)
        .join(" · ");
      return `${p.type} · ${detalle}`;
    }
    default:
      return "";
  }
}

function ActionCard({
  action,
  pending,
  onResolve,
}: {
  action: Action;
  pending: boolean;
  onResolve: (id: string, verb: "confirm" | "cancel" | "undo") => void;
}) {
  return (
    <div className="mt-2 rounded-lg border-2 border-ink bg-bg p-3 text-sm shadow-brutal-sm">
      <p className="font-display font-bold">{ACTION_TITLES[action.kind] ?? "Acción"}</p>
      <p className="text-muted">{actionDetails(action)}</p>
      {action.status === "proposed" && (
        <div className="mt-2 flex gap-2">
          <Button
            onClick={() => onResolve(action.id, "confirm")}
            disabled={pending}
            className="px-3 py-1 text-xs"
          >
            Confirmar
          </Button>
          <Button
            variant="ghost"
            onClick={() => onResolve(action.id, "cancel")}
            disabled={pending}
            className="px-3 py-1 text-xs"
          >
            Cancelar
          </Button>
        </div>
      )}
      {action.status === "done" && (
        <div className="mt-2 flex items-center gap-2">
          <Chip variant="money" size="sm">✓ Hecha</Chip>
          <Button
            variant="ghost"
            onClick={() => onResolve(action.id, "undo")}
            disabled={pending}
            className="px-3 py-1 text-xs"
          >
            Deshacer
          </Button>
        </div>
      )}
      {action.status === "cancelled" && <p className="mt-2 text-xs text-muted">Cancelada</p>}
      {action.status === "undone" && <p className="mt-2 text-xs text-muted">Deshecha</p>}
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
    mutationFn: ({ id, verb }: { id: string; verb: "confirm" | "cancel" | "undo" }) =>
      verb === "confirm" ? confirmAction(id) : verb === "cancel" ? cancelAction(id) : undoAction(id),
    onSuccess: (updated) => {
      setError(null);
      qc.setQueryData<Message[]>(["ai-messages"], (prev) =>
        (prev ?? []).map((m) =>
          m.actions?.some((a) => a.id === updated.id)
            ? { ...m, actions: m.actions.map((a) => (a.id === updated.id ? updated : a)) }
            : m
        )
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
    <PageTransition>
      <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Asistente</h1>
          <Link to="/" className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2">Volver</Link>
        </header>

        <section className="flex flex-col gap-2">
          {messages.length === 0 && !streaming ? (
            <p className="text-sm text-muted">
              Pregúntame sobre tu día, tus finanzas o tus hábitos.
            </p>
          ) : (
            messages.map((m: Message, i: number) => (
              <div
                key={i}
                className={
                  m.role === "user"
                    ? "self-end rounded-lg border-2 border-ink bg-accent/30 px-3 py-2 text-sm shadow-brutal-sm"
                    : "self-start rounded-lg border-2 border-ink bg-surface px-3 py-2 text-sm shadow-brutal-sm"
                }
              >
                {m.content}
                {m.role === "assistant" &&
                  m.actions?.map((a) => (
                    <ActionCard
                      key={a.id}
                      action={a}
                      pending={actionMutation.isPending && actionMutation.variables?.id === a.id}
                      onResolve={(id, verb) => actionMutation.mutate({ id, verb })}
                    />
                  ))}
              </div>
            ))
          )}
          {streaming && (
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
          <Button
            type="submit"
            disabled={mutation.isPending || text.trim() === ""}
          >
            Enviar
          </Button>
        </form>
      </div>
    </PageTransition>
  );
}
