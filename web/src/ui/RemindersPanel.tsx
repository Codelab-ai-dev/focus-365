import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { getPendingCommitments, toggle, type Commitment } from "@/lib/commitments";
import { todayString } from "@/lib/dashboard";

// RemindersPanel muestra arriba de la home los compromisos sin cumplir cuya fecha
// es hoy o anterior. Si no hay nada pendiente, no renderiza nada.
export function RemindersPanel() {
  const qc = useQueryClient();
  const { data, isSuccess } = useQuery({
    queryKey: ["commitments", "pending"],
    queryFn: getPendingCommitments,
  });

  const mut = useMutation({
    mutationFn: (id: string) => toggle(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commitments", "pending"] });
      qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });

  if (!isSuccess || data.length === 0) return null;

  const today = todayString();
  const vencidos = data.filter((c) => c.target_date < today);
  const hoy = data.filter((c) => c.target_date === today);

  return (
    <div className="mt-4 border-2 border-ink bg-surface p-4 shadow-brutal">
      <h2 className="font-display text-lg font-bold">Recordatorios</h2>
      {vencidos.length > 0 && (
        <Group title={`Vencidos (${vencidos.length})`} items={vencidos} danger mut={mut} />
      )}
      {hoy.length > 0 && <Group title="Hoy" items={hoy} mut={mut} />}
    </div>
  );
}

function Group({
  title,
  items,
  danger,
  mut,
}: {
  title: string;
  items: Commitment[];
  danger?: boolean;
  mut: ReturnType<typeof useMutation<Commitment, Error, string>>;
}) {
  return (
    <div className="mt-3">
      <p
        className={`text-xs font-bold uppercase tracking-wide ${
          danger ? "text-danger-fg" : "text-muted"
        }`}
      >
        {title}
      </p>
      <ul className="mt-2 space-y-2">
        {items.map((c) => (
          <li
            key={c.id}
            className={`flex items-center gap-3 border-2 border-ink px-3 py-2 ${
              danger ? "bg-danger-bg" : "bg-surface"
            }`}
          >
            <input
              type="checkbox"
              aria-label={c.text}
              className="h-5 w-5 shrink-0 accent-ink"
              checked={false}
              disabled={mut.isPending && mut.variables === c.id}
              onChange={() => mut.mutate(c.id)}
            />
            <Link to="/check-in" className="text-sm font-bold hover:underline">
              {c.text}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
