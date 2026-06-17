# Detalle del día en el historial del check-in — Plan de implementación (Rebanada 22)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** En el historial del check-in, tocar un día abre un modal con el detalle de ese día (ánimo/energía, las 4 dimensiones, win/evité y los compromisos del día).

**Architecture:** Rebanada solo-frontend. `list(30)` ya devuelve el `CheckIn` completo, así que el modal se arma con datos cargados; los compromisos del día se piden con `getDue(date)`. Se agrega un componente reutilizable `ui/Modal.tsx` y un `DayDetailModal` dentro de `check-in.tsx`. Sin cambios de backend ni migración.

**Tech Stack:** React + Vite + TanStack Query/Router + Vitest + Tailwind (tokens neo-brutalistas).

**Contexto del repo (leer antes de empezar):**
- Comandos web: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run <archivo>` para un test, `npx vitest run` para toda la suite, `npm run build` para typecheck + build.
- Tokens del design system: `border-[2.5px] border-ink`, `shadow-brutal`/`shadow-brutal-sm`, `bg-surface`, `bg-accent`, `font-display`, `text-muted`, `text-accent`, `bg-ink/40`. El `Card` (`@/ui/Card`) acepta `interactive` y `className`. El `Chip` (`@/ui/Chip`) acepta `variant`.
- `web/src/lib/checkins.ts` exporta `type CheckIn` con campos `id, date, mood, energy, espiritual, emocional, fisica, financiera, win, avoided, created_at, updated_at` y `list(limit)`.
- `web/src/lib/commitments.ts` exporta `type Commitment { id, target_date, text, done }` y `getDue(date): Promise<Commitment[]>`.
- En `check-in.tsx` ya están importados `getDue`, `toggle`, `Commitment` (línea 6), `Card`, `Chip`, `useState`, y la const `DIMENSIONS` (key/label/short/variant para las 4 dimensiones).

---

## Estructura de archivos

- **Crear** `web/src/ui/Modal.tsx` — shell de modal reutilizable (portal, overlay, Esc, scroll-lock).
- **Crear** `web/src/ui/Modal.test.tsx` — tests del shell.
- **Modificar** `web/src/routes/check-in.tsx` — `DayDetailModal` + filas del historial clickeables + estado `selected`.
- **Modificar** `web/src/routes/check-in.test.tsx` — tests del modal en el historial.

---

## Task 1: Componente reutilizable `ui/Modal.tsx`

**Files:**
- Create: `web/src/ui/Modal.tsx`
- Create: `web/src/ui/Modal.test.tsx`

- [ ] **Step 1: Test (que falla)**

Crear `web/src/ui/Modal.test.tsx`:

```tsx
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Modal } from "./Modal";

afterEach(() => {
  document.body.style.overflow = "";
});

describe("Modal", () => {
  it("no renderiza nada cuando open=false", () => {
    render(
      <Modal open={false} onClose={() => {}} title="Título">
        <p>contenido</p>
      </Modal>
    );
    expect(screen.queryByText("contenido")).not.toBeInTheDocument();
  });

  it("renderiza el título y los hijos cuando open=true", () => {
    render(
      <Modal open onClose={() => {}} title="Lun 16 jun">
        <p>contenido</p>
      </Modal>
    );
    expect(screen.getByText("Lun 16 jun")).toBeInTheDocument();
    expect(screen.getByText("contenido")).toBeInTheDocument();
  });

  it("llama onClose al presionar Escape", () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>x</p>
      </Modal>
    );
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("llama onClose al click en el overlay pero no dentro del contenido", () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>adentro</p>
      </Modal>
    );
    // click dentro del contenido: NO cierra
    fireEvent.click(screen.getByText("adentro"));
    expect(onClose).not.toHaveBeenCalled();
    // click en el overlay (role=dialog): cierra
    fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("llama onClose con el botón ✕", () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>x</p>
      </Modal>
    );
    fireEvent.click(screen.getByLabelText("Cerrar"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/ui/Modal.test.tsx`
Expected: FAIL (no existe `./Modal`).

- [ ] **Step 3: Implementar `web/src/ui/Modal.tsx`**

```tsx
import { useEffect, type ReactNode } from "react";
import { createPortal } from "react-dom";

// Modal es el shell reutilizable: overlay oscurecido + tarjeta centrada (portal
// al body). Cierra con Esc, con click en el overlay y con el botón ✕. Bloquea el
// scroll del body mientras está abierto.
export function Modal({
  open,
  onClose,
  title,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onClick={onClose}
      className="fixed inset-0 z-50 flex items-end justify-center bg-ink/40 p-4 sm:items-center"
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="max-h-[85vh] w-full max-w-md overflow-y-auto rounded-lg border-[2.5px] border-ink bg-surface shadow-brutal"
      >
        <div className="flex items-center justify-between border-b-2 border-ink px-4 py-3">
          <h2 className="font-display text-lg font-bold tracking-tight">{title}</h2>
          <button
            type="button"
            aria-label="Cerrar"
            onClick={onClose}
            className="rounded-md border-2 border-ink px-2 py-1 text-sm font-bold shadow-brutal-sm"
          >
            ✕
          </button>
        </div>
        <div className="p-4">{children}</div>
      </div>
    </div>,
    document.body
  );
}
```

- [ ] **Step 4: Verde**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/ui/Modal.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/ui/Modal.tsx web/src/ui/Modal.test.tsx
git commit -m "feat(web): componente Modal reutilizable (portal, Esc, overlay)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: `DayDetailModal` + historial clickeable en `check-in.tsx`

**Files:**
- Modify: `web/src/routes/check-in.tsx`
- Modify: `web/src/routes/check-in.test.tsx`

- [ ] **Step 1: Tests (que fallan)**

En `web/src/routes/check-in.test.tsx`, agregá un `fetchMock` que devuelva un historial con un check-in (con una dimensión y win con texto) y un compromiso para ese día, y tests que abran el modal. Poné este helper y el bloque de tests al final del archivo (dentro del `describe("CheckInPage", ...)` o en uno nuevo con su propio `beforeEach`):

```tsx
// fetchMock con historial: la lista /checkins devuelve un día con contenido, y
// /commitments/due devuelve un compromiso de ese día.
function fetchMockWithHistory() {
  const day = {
    id: "h1", date: "2026-06-16", mood: 7, energy: 6,
    espiritual: "oré 10 min", emocional: "", fisica: "", financiera: "",
    win: "cerré el trato", avoided: "",
    created_at: "", updated_at: "",
  };
  return vi.fn((url: string, opts?: RequestInit) => {
    if (url.includes("/commitments/due")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({ commitments: [{ id: "k1", target_date: "2026-06-16", text: "leer", done: true }] }),
          { status: 200 }
        )
      );
    }
    if (url.includes("/today")) {
      return Promise.resolve(new Response("null", { status: 200 }));
    }
    if (url.includes("/checkins?") || url.endsWith("/checkins")) {
      return Promise.resolve(new Response(JSON.stringify([day]), { status: 200 }));
    }
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify(day), { status: 200 }));
    }
    return Promise.resolve(new Response("[]", { status: 200 }));
  });
}

describe("CheckInPage — detalle del día", () => {
  afterEach(() => vi.restoreAllMocks());

  it("toca una fila del historial y abre el modal con el detalle", async () => {
    vi.stubGlobal("fetch", fetchMockWithHistory());
    renderPage();
    const row = await screen.findByLabelText("Ver detalle del 2026-06-16");
    await userEvent.click(row);
    // dimensión con texto visible; dimensión vacía oculta
    expect(await screen.findByText(/oré 10 min/)).toBeInTheDocument();
    expect(screen.getByText(/cerré el trato/)).toBeInTheDocument();
    expect(screen.queryByText("Emocional:")).not.toBeInTheDocument();
    // compromiso del día desde getDue
    expect(screen.getByText("leer")).toBeInTheDocument();
  });

  it("cierra el modal con ✕", async () => {
    vi.stubGlobal("fetch", fetchMockWithHistory());
    renderPage();
    const row = await screen.findByLabelText("Ver detalle del 2026-06-16");
    await userEvent.click(row);
    await screen.findByText(/oré 10 min/);
    await userEvent.click(screen.getByLabelText("Cerrar"));
    await waitFor(() =>
      expect(screen.queryByText(/oré 10 min/)).not.toBeInTheDocument()
    );
  });
});
```

> Nota: el `describe` existente tiene su propio `beforeEach(() => vi.stubGlobal("fetch", fetchMock()))`. Para estos tests usá un `describe` separado (como arriba) que stubea `fetchMockWithHistory` por test, para no chocar con el `fetchMock` de los demás.

- [ ] **Step 2: Verlos fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/routes/check-in.test.tsx`
Expected: FAIL (no existe el botón "Ver detalle del…").

- [ ] **Step 3: Importar `Modal` y agregar el estado `selected`**

En `web/src/routes/check-in.tsx`:

a) Agregar el import (junto a los otros `@/ui/...`):

```tsx
import { Modal } from "@/ui/Modal";
```

b) Dentro de `CheckInPage`, junto a los otros `useState`, agregar:

```tsx
  const [selected, setSelected] = useState<CheckIn | null>(null);
```

- [ ] **Step 4: Hacer clickeables las filas del historial**

Reemplazar el `.map` del historial (el bloque `historyQuery.data.map(...)` dentro de `<Reveal>`) por:

```tsx
              {historyQuery.data.map((ci: CheckIn) => (
                <RevealItem key={ci.id}>
                  <button
                    type="button"
                    aria-label={`Ver detalle del ${ci.date}`}
                    onClick={() => setSelected(ci)}
                    className="w-full text-left"
                  >
                    <Card interactive className="flex items-center justify-between px-4 py-2 text-sm">
                      <span className="text-muted">{ci.date}</span>
                      <span>
                        Á{ci.mood} · E{ci.energy}
                      </span>
                    </Card>
                  </button>
                </RevealItem>
              ))}
```

- [ ] **Step 5: Montar el modal**

Justo antes de cerrar el `<div className="mx-auto max-w-xl p-6">` (después de la `<section>` del historial), agregar:

```tsx
        <DayDetailModal checkin={selected} onClose={() => setSelected(null)} />
```

- [ ] **Step 6: Implementar `DayDetailModal` y `formatDay`**

Agregar al final del archivo `check-in.tsx` (junto a `Slider`):

```tsx
function DayDetailModal({
  checkin,
  onClose,
}: {
  checkin: CheckIn | null;
  onClose: () => void;
}) {
  // Los compromisos de ese día: se piden solo cuando hay un día seleccionado.
  const dueQuery = useQuery({
    queryKey: ["commitments", "due", checkin?.date],
    queryFn: () => getDue(checkin!.date),
    enabled: !!checkin,
  });
  const commitments = dueQuery.data ?? [];
  const done = commitments.filter((c) => c.done).length;

  return (
    <Modal
      open={checkin !== null}
      onClose={onClose}
      title={checkin ? formatDay(checkin.date) : ""}
    >
      {checkin && (
        <div className="space-y-4 text-sm">
          <p className="font-bold">
            Ánimo {checkin.mood} · Energía {checkin.energy}
          </p>

          {DIMENSIONS.some((d) => checkin[d.key]) && (
            <div className="space-y-2">
              {DIMENSIONS.filter((d) => checkin[d.key]).map((d) => (
                <p key={d.key} className="flex items-start gap-2">
                  <Chip variant={d.variant}>{d.short}</Chip>
                  <span>
                    <span className="font-bold">{d.label}:</span> {checkin[d.key]}
                  </span>
                </p>
              ))}
            </div>
          )}

          {checkin.win && (
            <p>
              🏆 <span className="font-bold">Win:</span> {checkin.win}
            </p>
          )}
          {checkin.avoided && (
            <p>
              🚫 <span className="font-bold">Evité:</span> {checkin.avoided}
            </p>
          )}

          <div className="space-y-2 border-t-2 border-ink pt-3">
            <div className="flex items-center justify-between">
              <span className="font-display text-xs font-bold uppercase tracking-[0.12em] text-muted">
                📋 Compromisos
              </span>
              {commitments.length > 0 && (
                <span className="font-display text-xs font-bold text-accent">
                  {done}/{commitments.length} ✓
                </span>
              )}
            </div>
            {commitments.length === 0 ? (
              <p className="text-muted">Sin compromisos ese día.</p>
            ) : (
              commitments.map((c: Commitment) => (
                <p key={c.id} className="flex items-center gap-2">
                  <span>{c.done ? "✓" : "✗"}</span>
                  <span className={c.done ? "line-through text-muted" : ""}>
                    {c.text}
                  </span>
                </p>
              ))
            )}
          </div>
        </div>
      )}
    </Modal>
  );
}

// formatDay muestra "lun 16 jun" parseando YYYY-MM-DD como fecha LOCAL (evita el
// corrimiento de día de new Date("YYYY-MM-DD"), que interpreta UTC).
function formatDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  return date.toLocaleDateString("es", {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
}
```

> `checkin[d.key]` indexa `CheckIn` con `d.key` (`"espiritual" | "emocional" | "fisica" | "financiera"`), todos campos `string` de `CheckIn` — typecheck OK. `getDue`, `Commitment`, `Chip`, `useQuery` y `DIMENSIONS` ya están en el archivo.

- [ ] **Step 7: Verde + suite completa + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido).

- [ ] **Step 8: Commit**

```bash
git add web/src/routes/check-in.tsx web/src/routes/check-in.test.tsx
git commit -m "feat(web): detalle del día en el historial del check-in (modal)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: Cierre — review, merge y verificación

**Files:** verificación + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-22-detalle-dia-checkin-design.md`. Aplicar nits.

- [ ] **Step 2: Suite verde + build:** `cd web && npx vitest run && npm run build`.

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`. Mensaje de merge describiendo la rebanada 22.

- [ ] **Step 4: Deploy manual (Coolify) + verificación visual.** Es solo-frontend: no hay smoke con curl. Tras el deploy, verificación visual: en `/check-in`, tocar un día del historial abre el modal con el detalle; cerrar con ✕/Esc/fondo. (El auto-deploy no dispara y el usuario deploya a mano — ver memoria.)

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-22-detalle-dia-checkin.md`.

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 `ui/Modal.tsx` (portal, overlay, Esc, scroll-lock, ✕, role/aria) → Task 1. ✓
- §2 `DayDetailModal` (header fecha local, ánimo/energía, dimensiones no vacías, win/avoided condicionales, compromisos vía getDue con contador y ✓/✗, vacío "sin compromisos") → Task 2 Step 6. ✓
- §3 cambios en check-in.tsx (estado `selected`, filas clickeables con aria-label, montaje del modal) → Task 2 Steps 3–5. ✓
- §4 errores (getDue falla → sección vacía; día solo ánimo/energía; cierre Esc/fondo/✕ + scroll-lock) → Task 1 (Modal) + Task 2 (render condicional). ✓
- §5 testing (Modal: open/closed/Esc/overlay-vs-contenido/✕; check-in: abrir con datos, dimensión vacía oculta, compromisos mockeados, cerrar) → Tasks 1–2. ✓
- §6 aceptación → Task 3 (verificación visual). ✓

**Placeholders:** ninguno; todo el código está completo.

**Consistencia de tipos/firmas:** `Modal({open, onClose, title, children})` definido en Task 1 y usado igual en Task 2. `DayDetailModal({checkin: CheckIn | null, onClose})`. `formatDay(iso: string): string`. `getDue(date) → Promise<Commitment[]>` y `Commitment{id, text, done}` ya en el repo. `DIMENSIONS[].key` ∈ campos string de `CheckIn`. Los aria-labels de los tests (`Ver detalle del 2026-06-16`, `Cerrar`) coinciden con los del código. ✓
