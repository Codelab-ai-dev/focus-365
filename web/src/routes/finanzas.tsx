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
import {
  importFile,
  getPendingUploads,
  confirmAction,
  cancelAction,
  undoAction,
  type Action,
} from "@/lib/ai";
import { Card } from "@/ui/Card";
import { Chip } from "@/ui/Chip";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { Stat } from "@/ui/Stat";
import { ActionCard } from "@/ui/ActionCard";
import { PageTransition } from "@/ui/PageTransition";
import { Reveal, RevealItem } from "@/ui/Reveal";

export const Route = createFileRoute("/finanzas")({ component: FinanzasPage });

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
  const uploadsQuery = useQuery({
    queryKey: ["finance", "uploads"],
    queryFn: () => getPendingUploads(),
    enabled: !!user,
  });

  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [uploadNote, setUploadNote] = useState<string | null>(null);

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

  const actionMutation = useMutation({
    mutationFn: ({ id, verb }: { id: string; verb: "confirm" | "cancel" | "undo" }) =>
      verb === "confirm"
        ? confirmAction(id)
        : verb === "cancel"
          ? cancelAction(id)
          : undoAction(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["finance", "uploads"] });
      qc.invalidateQueries({ queryKey: ["finance", "list"] });
      qc.invalidateQueries({ queryKey: ["finance", "summary"] });
      qc.invalidateQueries({ queryKey: ["finance", "cycles"] });
    },
  });

  async function handleUpload(file: File) {
    setUploading(true);
    setUploadError(null);
    setUploadNote(null);
    try {
      const res = await importFile(file);
      qc.invalidateQueries({ queryKey: ["finance", "uploads"] });
      const parts: string[] = [`Extraje ${res.created.length}`];
      if (res.dropped > 0) parts.push(`${res.dropped} no se pudieron leer`);
      if (res.truncated) parts.push("truncado a las primeras filas");
      setUploadNote(parts.join(" · "));
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : "No se pudo importar el archivo");
    } finally {
      setUploading(false);
    }
  }

  const proposed = (uploadsQuery.data ?? []).filter(
    (a: Action) => a.status === "proposed"
  );

  const confirmAllMutation = useMutation({
    // allSettled: una confirmación que falle no aborta las demás.
    mutationFn: () => Promise.allSettled(proposed.map((a) => confirmAction(a.id))),
    // onSettled (no onSuccess): tras un fallo parcial igual reconciliamos el
    // caché con el servidor, así no quedan tarjetas obsoletas.
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["finance", "uploads"] });
      qc.invalidateQueries({ queryKey: ["finance", "list"] });
      qc.invalidateQueries({ queryKey: ["finance", "summary"] });
      qc.invalidateQueries({ queryKey: ["finance", "cycles"] });
    },
  });

  if (!user) return null;

  const sum = summaryQuery.data;

  const netoBg =
    sum?.status === "verde"
      ? "bg-money-bg text-money-fg"
      : sum?.status === "rojo"
        ? "bg-danger-bg text-danger-fg"
        : "";

  return (
    <PageTransition>
      <div className="mx-auto max-w-3xl p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Finanzas</h1>
          <Link
            to="/"
            className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2"
          >
            Volver
          </Link>
        </header>

        {sum && (
          <Card className={`mt-6 p-6 ${netoBg}`}>
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                Ciclo {sum.cycle}
              </span>
              <span className="text-sm font-bold">{sum.status}</span>
            </div>
            <div className="mt-4 grid grid-cols-3 gap-4">
              <Stat label="Ingresos" value={sum.income} format={formatMXN} />
              <Stat label="Gastos" value={sum.expense} format={formatMXN} />
              <Stat label="Neto" value={sum.net} format={formatMXN} hideLabel />
            </div>
          </Card>
        )}

        <Card className="mt-6 p-6">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              createMutation.mutate();
            }}
            className="space-y-4"
          >
            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Tipo</span>
              <select
                aria-label="Tipo"
                value={type}
                onChange={(e) => setType(e.target.value as TxType)}
                className="w-full rounded-lg border-[2.5px] border-ink bg-surface px-3 py-2 text-sm text-ink outline-none transition-shadow focus:shadow-brutal-sm"
              >
                <option value="expense">Gasto</option>
                <option value="income">Ingreso</option>
                <option value="transfer">Transferencia</option>
              </select>
            </label>

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Monto</span>
              <Input
                type="number"
                aria-label="Monto"
                min="0"
                step="0.01"
                value={montoPesos}
                onChange={(e) => setMontoPesos(e.target.value)}
              />
            </label>

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Fecha</span>
              <Input
                type="date"
                aria-label="Fecha"
                value={occurredOn}
                onChange={(e) => setOccurredOn(e.target.value)}
              />
            </label>

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Categoría</span>
              <Input
                type="text"
                aria-label="Categoría"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
              />
            </label>

            <label className="block space-y-1">
              <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">Nota</span>
              <Input
                type="text"
                aria-label="Nota"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
              />
            </label>

            {error && (
              <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
                {error}
              </p>
            )}

            <Button
              type="submit"
              disabled={createMutation.isPending}
              className="w-full"
            >
              {createMutation.isPending ? "Guardando…" : "Guardar"}
            </Button>
          </form>
        </Card>

        <Card className="mt-6 p-6">
          <h2 className="font-display text-lg font-bold tracking-tight">Subir comprobante</h2>
          <p className="mt-1 text-sm text-muted">
            Imagen de un ticket, un CSV o un PDF. Extraigo los movimientos para que los confirmes.
          </p>
          <label className="mt-4 flex cursor-pointer flex-col items-center justify-center rounded-lg border-[2.5px] border-dashed border-ink bg-surface px-4 py-8 text-center text-sm font-bold text-muted transition-shadow hover:shadow-brutal-sm">
            <span>{uploading ? "Procesando…" : "Toca para elegir un archivo"}</span>
            <input
              type="file"
              aria-label="Subir comprobante"
              accept="image/*,.csv,.pdf"
              disabled={uploading}
              className="sr-only"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleUpload(file);
                e.target.value = "";
              }}
            />
          </label>

          {uploadNote && (
            <p className="mt-3 rounded-md border-2 border-ink bg-surface px-3 py-2 text-sm font-bold shadow-brutal-sm">
              {uploadNote}
            </p>
          )}
          {uploadError && (
            <p className="mt-3 rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
              {uploadError}
            </p>
          )}

          {uploadsQuery.data && uploadsQuery.data.length > 0 && (
            <div className="mt-4">
              <div className="flex items-center justify-between">
                <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted">
                  Movimientos detectados
                </span>
                <Button
                  type="button"
                  onClick={() => confirmAllMutation.mutate()}
                  disabled={proposed.length === 0 || confirmAllMutation.isPending}
                  className="px-3 py-1 text-xs"
                >
                  {confirmAllMutation.isPending ? "Confirmando…" : "Confirmar todos"}
                </Button>
              </div>
              {uploadsQuery.data.map((a: Action) => (
                <ActionCard
                  key={a.id}
                  action={a}
                  pending={
                    (actionMutation.isPending && actionMutation.variables?.id === a.id) ||
                    confirmAllMutation.isPending
                  }
                  onResolve={(id, verb) => actionMutation.mutate({ id, verb })}
                />
              ))}
            </div>
          )}
        </Card>

        <section className="mt-8">
          <h2 className="font-display text-xl font-bold tracking-tight">Movimientos del ciclo</h2>
          {listQuery.data && listQuery.data.length > 0 ? (
            <Reveal className="mt-3 space-y-2">
              {listQuery.data.map((tx: Transaction) => (
                <RevealItem key={tx.id}>
                  <Card className="flex items-center justify-between p-3">
                    <span>
                      <span className="text-muted">{tx.occurred_on}</span>{" "}
                      {tx.category || tx.type}
                    </span>
                    <span className="flex items-center gap-3">
                      {tx.type === "income" ? (
                        <Chip variant="money">
                          <span className="font-display font-bold">{formatMXN(tx.amount)}</span>
                        </Chip>
                      ) : (
                        <Chip variant="danger">
                          <span className="font-display font-bold">{formatMXN(tx.amount)}</span>
                        </Chip>
                      )}
                      <button
                        type="button"
                        aria-label={`Borrar ${tx.category || tx.type}`}
                        onClick={() => deleteMutation.mutate(tx.id)}
                        className="text-xs text-muted hover:text-danger-fg"
                      >
                        ✕
                      </button>
                    </span>
                  </Card>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="mt-3 text-sm text-muted">Aún no hay movimientos.</p>
          )}
        </section>

        <section className="mt-8">
          <h2 className="font-display text-xl font-bold tracking-tight">Historial de ciclos</h2>
          {cyclesQuery.data && cyclesQuery.data.length > 0 ? (
            <Reveal className="mt-3 space-y-2">
              {cyclesQuery.data.map((c: CycleSummary) => (
                <RevealItem key={c.cycle}>
                  <Card className="flex items-center justify-between p-3 text-sm">
                    <span className="text-muted">{c.cycle}</span>
                    <span className="flex items-center gap-3">
                      <span className="font-display font-bold tracking-tight">{formatMXN(c.net)}</span>
                      <span className="font-bold">{c.status}</span>
                    </span>
                  </Card>
                </RevealItem>
              ))}
            </Reveal>
          ) : (
            <p className="mt-3 text-sm text-muted">Aún no hay ciclos.</p>
          )}
        </section>
      </div>
    </PageTransition>
  );
}
