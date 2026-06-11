import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  create,
  listByCycle,
  remove,
  summary,
  cycles,
  pesosToCents,
  formatMXN,
  todayString,
  type Transaction,
  type CycleSummary,
  type TxType,
} from "@/lib/finances";

export const Route = createFileRoute("/finanzas")({ component: FinanzasPage });

const STATUS_COLOR: Record<CycleSummary["status"], string> = {
  pendiente: "text-sand-400",
  verde: "text-streak",
  rojo: "text-red-400",
};

function FinanzasPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const summaryQuery = useQuery({
    queryKey: ["finance", "summary"],
    queryFn: () => summary(),
    enabled: !!user,
  });
  const listQuery = useQuery({
    queryKey: ["finance", "list"],
    queryFn: () => listByCycle(),
    enabled: !!user,
  });
  const cyclesQuery = useQuery({
    queryKey: ["finance", "cycles"],
    queryFn: () => cycles(),
    enabled: !!user,
  });

  const [type, setType] = useState<TxType>("expense");
  const [montoPesos, setMontoPesos] = useState("");
  const [occurredOn, setOccurredOn] = useState(todayString());
  const [category, setCategory] = useState("");
  const [remark, setRemark] = useState("");
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["finance"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      create({
        type,
        amount: pesosToCents(Number(montoPesos)),
        occurred_on: occurredOn,
        category,
        remark,
      }),
    onSuccess: () => {
      setError(null);
      setMontoPesos("");
      setCategory("");
      setRemark("");
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al guardar"),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => remove(id),
    onSuccess: invalidate,
  });

  if (!user) return null;

  const sum = summaryQuery.data;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Finanzas</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      {sum && (
        <section className="mt-6 rounded-xl border border-ink-700 bg-ink-900 p-6">
          <div className="flex items-center justify-between">
            <span className="text-sm text-sand-400">Ciclo {sum.cycle}</span>
            <span className={`text-sm font-bold ${STATUS_COLOR[sum.status]}`}>
              {sum.status}
            </span>
          </div>
          <p className="mt-2 text-2xl font-extrabold">{formatMXN(sum.net)}</p>
          <p className="mt-1 text-xs text-sand-400">
            Ingresos {formatMXN(sum.income)} · Gastos {formatMXN(sum.expense)}
          </p>
        </section>
      )}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          createMutation.mutate();
        }}
        className="mt-6 space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6"
      >
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Tipo</span>
          <select
            aria-label="Tipo"
            value={type}
            onChange={(e) => setType(e.target.value as TxType)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          >
            <option value="expense">Gasto</option>
            <option value="income">Ingreso</option>
            <option value="transfer">Transferencia</option>
          </select>
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Monto</span>
          <input
            type="number"
            aria-label="Monto"
            min="0"
            step="0.01"
            value={montoPesos}
            onChange={(e) => setMontoPesos(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Fecha</span>
          <input
            type="date"
            aria-label="Fecha"
            value={occurredOn}
            onChange={(e) => setOccurredOn(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Categoría</span>
          <input
            type="text"
            aria-label="Categoría"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Nota</span>
          <input
            type="text"
            aria-label="Nota"
            value={remark}
            onChange={(e) => setRemark(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Guardando…" : "Guardar"}
        </button>
      </form>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Movimientos del ciclo</h2>
        {listQuery.data && listQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-2">
            {listQuery.data.map((tx: Transaction) => (
              <li
                key={tx.id}
                className="flex items-center justify-between rounded-lg border border-ink-700 bg-ink-900 px-4 py-2 text-sm"
              >
                <span>
                  <span className="text-sand-400">{tx.occurred_on}</span>{" "}
                  {tx.category || tx.type}
                </span>
                <span className="flex items-center gap-3">
                  <span className={tx.type === "income" ? "text-streak" : ""}>
                    {formatMXN(tx.amount)}
                  </span>
                  <button
                    type="button"
                    aria-label={`Borrar ${tx.category || tx.type}`}
                    onClick={() => deleteMutation.mutate(tx.id)}
                    className="text-xs text-sand-400 hover:text-red-400"
                  >
                    ✕
                  </button>
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay movimientos.</p>
        )}
      </section>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Historial de ciclos</h2>
        {cyclesQuery.data && cyclesQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-2">
            {cyclesQuery.data.map((c: CycleSummary) => (
              <li
                key={c.cycle}
                className="flex items-center justify-between rounded-lg border border-ink-700 bg-ink-900 px-4 py-2 text-sm"
              >
                <span className="text-sand-400">{c.cycle}</span>
                <span className="flex items-center gap-3">
                  <span>{formatMXN(c.net)}</span>
                  <span className={`font-bold ${STATUS_COLOR[c.status]}`}>{c.status}</span>
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay ciclos.</p>
        )}
      </section>
    </div>
  );
}
