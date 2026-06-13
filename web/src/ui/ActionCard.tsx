import type { Action } from "@/lib/ai";
import { Button } from "@/ui/Button";
import { Chip } from "@/ui/Chip";

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

export function ActionCard({
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
