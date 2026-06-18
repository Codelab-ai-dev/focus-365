# Recordatorios de compromisos (panel in-app) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un panel arriba de la home que destaca los compromisos sin cumplir cuya fecha es hoy o anterior (vencidos), con check para marcarlos cumplidos ahí mismo.

**Architecture:** Backend agrega una query `ListPendingCommitments` (done=false, target_date<=hoy), un método de servicio `Pending` y un endpoint `GET /commitments/pending`. El front agrega `getPendingCommitments()` y un componente `RemindersPanel` que se monta arriba de la grilla de la home; marcar cumplido reusa el endpoint `POST /commitments/{id}/toggle` ya existente.

**Tech Stack:** Go (chi, sqlc, pgx) + React 18 (TanStack Query/Router, Vitest, Tailwind neo-brutalista).

---

## Notas de entorno (leer antes de empezar)

- **Build/vet Go:** `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && go vet ./...`
- **Tests Go (DB):** `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1`
  - **OBLIGATORIO `-p 1`** — la DB de test hace deadlock con paquetes en paralelo.
- **sqlc:** tras editar `db/queries/commitments.sql`, regenerar con `cd api && sqlc generate` (genera `internal/store`). Si `sqlc` no está en PATH, usar `go run github.com/sqlc-dev/sqlc/cmd/sqlc generate` desde `api/`.
- **Front:** `cd web && npm test -- <archivo>` para un test puntual y `npm run build` (tsc typecheck) antes de cerrar cada tarea de front.
- **Convención sqlc del repo:** uuid→`uuid.UUID`, date/timestamptz→`time.Time`. Las queries usan params posicionales `$1,$2,...`.

---

## File Structure

- `api/db/queries/commitments.sql` (modificar) — agrega `ListPendingCommitments`.
- `api/internal/store/*` (generado por sqlc) — nuevo método `ListPendingCommitments`.
- `api/internal/commitments/service.go` (modificar) — método `Pending`.
- `api/internal/commitments/service_test.go` (modificar) — test de `Pending`.
- `api/internal/commitments/handler.go` (modificar) — ruta y handler `/pending`.
- `api/internal/commitments/handler_test.go` (modificar) — test del endpoint.
- `web/src/lib/commitments.ts` (modificar) — `getPendingCommitments()`.
- `web/src/ui/RemindersPanel.tsx` (crear) — el panel.
- `web/src/ui/RemindersPanel.test.tsx` (crear) — test del panel.
- `web/src/routes/index.tsx` (modificar) — montar `<RemindersPanel />`.
- `scripts/smoke-r31.sh` (crear) — smoke de producción.

---

## Task 1: Query + método de servicio `Pending`

**Files:**
- Modify: `api/db/queries/commitments.sql`
- Modify: `api/internal/commitments/service.go`
- Test: `api/internal/commitments/service_test.go`

- [ ] **Step 1: Escribir el test que falla**

Agregar al final de `api/internal/commitments/service_test.go` (usa el helper `newSvc` ya existente en el archivo):

```go
func TestPending(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	today := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	ayer := today.AddDate(0, 0, -1)
	manana := today.AddDate(0, 0, 1)

	// Vencido (ayer), de hoy, y futuro (mañana).
	if err := svc.ReplaceForDate(ctx, uid, ayer, []string{"Vencido"}); err != nil {
		t.Fatalf("Replace ayer: %v", err)
	}
	if err := svc.ReplaceForDate(ctx, uid, today, []string{"De hoy"}); err != nil {
		t.Fatalf("Replace hoy: %v", err)
	}
	if err := svc.ReplaceForDate(ctx, uid, manana, []string{"Futuro"}); err != nil {
		t.Fatalf("Replace manana: %v", err)
	}

	pend, err := svc.Pending(ctx, uid, today)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	// Trae vencido + hoy (no el futuro), vencido primero.
	if len(pend) != 2 || pend[0].Text != "Vencido" || pend[1].Text != "De hoy" {
		t.Fatalf("pending = %+v (esperaba [Vencido, De hoy])", pend)
	}

	// Marcar el vencido como cumplido lo saca de pending.
	if _, err := svc.Toggle(ctx, uid, uuid.MustParse(pend[0].ID)); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	pend2, err := svc.Pending(ctx, uid, today)
	if err != nil {
		t.Fatalf("Pending 2: %v", err)
	}
	if len(pend2) != 1 || pend2[0].Text != "De hoy" {
		t.Fatalf("pending2 = %+v (esperaba [De hoy])", pend2)
	}
}
```

- [ ] **Step 2: Correr el test y verificar que falla a compilar**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./internal/commitments/ -run TestPending -count=1`
Expected: FAIL — `svc.Pending undefined`.

- [ ] **Step 3: Agregar la query sqlc**

En `api/db/queries/commitments.sql`, agregar al final:

```sql
-- name: ListPendingCommitments :many
SELECT * FROM commitments
WHERE user_id = $1 AND done = false AND target_date <= $2
ORDER BY target_date ASC, position ASC;
```

- [ ] **Step 4: Regenerar sqlc**

Run: `cd api && sqlc generate` (o `cd api && go run github.com/sqlc-dev/sqlc/cmd/sqlc generate` si `sqlc` no está en PATH).
Expected: sin errores; aparece `ListPendingCommitments` en `internal/store/commitments.sql.go` con `ListPendingCommitmentsParams{ UserID uuid.UUID; TargetDate time.Time }`.

- [ ] **Step 5: Implementar `Pending` en el servicio**

En `api/internal/commitments/service.go`, agregar después de `DueOn`:

```go
// Pending devuelve los compromisos sin cumplir con target_date <= today
// (vencidos + hoy), vencidos primero. Para el panel de recordatorios de la home.
func (s *Service) Pending(ctx context.Context, userID uuid.UUID, today time.Time) ([]Commitment, error) {
	rows, err := s.q.ListPendingCommitments(ctx, store.ListPendingCommitmentsParams{
		UserID:     userID,
		TargetDate: today,
	})
	if err != nil {
		return nil, err
	}
	return mapViews(rows), nil
}
```

- [ ] **Step 6: Correr el test y verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./internal/commitments/ -run TestPending -count=1`
Expected: PASS.

- [ ] **Step 7: Build + vet**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && go vet ./...`
Expected: sin salida (limpio).

- [ ] **Step 8: Commit**

```bash
git add api/db/queries/commitments.sql api/internal/store api/internal/commitments/service.go api/internal/commitments/service_test.go
git commit -m "feat(commitments): query y servicio Pending (recordatorios)"
```

---

## Task 2: Endpoint `GET /commitments/pending`

**Files:**
- Modify: `api/internal/commitments/handler.go`
- Test: `api/internal/commitments/handler_test.go`

- [ ] **Step 1: Escribir el test que falla**

Agregar al final de `api/internal/commitments/handler_test.go` (usa los helpers `newEnv`, `register`, `do` ya existentes). Nota: `do` no permite fijar la fecha; el handler usa `time.Now()`, así que el test crea un compromiso de **ayer** (siempre vencido) y otro de **hoy** relativos a la fecha real, y verifica que `pending` devuelve 2.

```go
func TestPendingEndpoint(t *testing.T) {
	e := newEnv(t)
	uid, tok := e.register(t, "pending@b.com")
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	ayer := today.AddDate(0, 0, -1)
	if err := e.svc.ReplaceForDate(ctx, uid, ayer, []string{"Vencido"}); err != nil {
		t.Fatalf("Replace ayer: %v", err)
	}
	if err := e.svc.ReplaceForDate(ctx, uid, today, []string{"De hoy"}); err != nil {
		t.Fatalf("Replace hoy: %v", err)
	}

	rec := do(t, e.h, http.MethodGet, "/commitments/pending", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperaba 200", rec.Code)
	}
	var body struct {
		Commitments []commitments.Commitment `json:"commitments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Commitments) != 2 || body.Commitments[0].Text != "Vencido" {
		t.Fatalf("commitments = %+v (esperaba [Vencido, De hoy])", body.Commitments)
	}

	// Sin auth -> 401.
	rec401 := do(t, e.h, http.MethodGet, "/commitments/pending", "")
	if rec401.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth status = %d, esperaba 401", rec401.Code)
	}
}
```

- [ ] **Step 2: Correr el test y verificar que falla**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./internal/commitments/ -run TestPendingEndpoint -count=1`
Expected: FAIL — la ruta `/pending` devuelve 404 (no montada), así que el status no es 200.

- [ ] **Step 3: Implementar ruta y handler**

En `api/internal/commitments/handler.go`, dentro de `Routes`, agregar la ruta (antes de `return r`):

```go
	r.Get("/pending", handlePending(svc))
```

Y agregar el handler (después de `handleDue`):

```go
func handlePending(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		pend, err := svc.Pending(r.Context(), userID, today)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"commitments": pend})
	}
}
```

- [ ] **Step 4: Correr el test y verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./internal/commitments/ -count=1`
Expected: PASS (todos los tests del paquete).

- [ ] **Step 5: Build + vet**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && go vet ./...`
Expected: limpio.

- [ ] **Step 6: Commit**

```bash
git add api/internal/commitments/handler.go api/internal/commitments/handler_test.go
git commit -m "feat(commitments): endpoint GET /pending (recordatorios)"
```

---

## Task 3: Frontend — lib, panel y montaje en la home

**Files:**
- Modify: `web/src/lib/commitments.ts`
- Create: `web/src/ui/RemindersPanel.tsx`
- Test: `web/src/ui/RemindersPanel.test.tsx`
- Modify: `web/src/routes/index.tsx`

- [ ] **Step 1: Agregar `getPendingCommitments` a la lib**

En `web/src/lib/commitments.ts`, agregar (debajo de `getDue`):

```ts
export function getPendingCommitments(): Promise<Commitment[]> {
  return apiFetch<{ commitments: Commitment[] }>(
    `/api/v1/commitments/pending`
  ).then((r) => r.commitments);
}
```

- [ ] **Step 2: Escribir el test del panel (falla)**

Crear `web/src/ui/RemindersPanel.test.tsx`. Mockea `@/lib/commitments` para controlar los datos y el toggle, y envuelve en un router de memoria (el panel usa `Link`).

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";
import { todayString } from "@/lib/dashboard";

const toggleSpy = vi.fn(() =>
  Promise.resolve({ id: "h1", target_date: todayString(), text: "De hoy", done: true })
);
const getPendingSpy = vi.fn();

vi.mock("@/lib/commitments", () => ({
  getPendingCommitments: () => getPendingSpy(),
  toggle: (id: string) => toggleSpy(id),
}));

import { RemindersPanel } from "./RemindersPanel";

const TODAY = todayString();
const AYER = todayString(new Date(Date.now() - 24 * 60 * 60 * 1000));

function renderPanel() {
  const rootRoute = createRootRoute({ component: RemindersPanel });
  const checkinRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/check-in",
    component: () => <div>check-in</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([checkinRoute]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("RemindersPanel", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("muestra grupos de vencidos y hoy", async () => {
    getPendingSpy.mockResolvedValue([
      { id: "v1", target_date: AYER, text: "Vencido", done: false },
      { id: "h1", target_date: TODAY, text: "De hoy", done: false },
    ]);
    renderPanel();
    expect(await screen.findByText("Vencido")).toBeInTheDocument();
    expect(screen.getByText("De hoy")).toBeInTheDocument();
    expect(screen.getByText(/Vencidos/i)).toBeInTheDocument();
    expect(screen.getByText(/Hoy/i)).toBeInTheDocument();
  });

  it("no renderiza nada si no hay pendientes", async () => {
    getPendingSpy.mockResolvedValue([]);
    const { container } = (() => {
      renderPanel();
      return { container: document.body };
    })();
    await waitFor(() => {
      expect(screen.queryByText(/Recordatorios/i)).not.toBeInTheDocument();
    });
    expect(container).toBeTruthy();
  });

  it("al marcar el check dispara toggle", async () => {
    getPendingSpy.mockResolvedValue([
      { id: "h1", target_date: TODAY, text: "De hoy", done: false },
    ]);
    renderPanel();
    const check = await screen.findByRole("checkbox", { name: /De hoy/i });
    await userEvent.click(check);
    await waitFor(() => expect(toggleSpy).toHaveBeenCalledWith("h1"));
  });
});
```

- [ ] **Step 3: Correr el test y verificar que falla**

Run: `cd web && npm test -- src/ui/RemindersPanel.test.tsx`
Expected: FAIL — no existe `./RemindersPanel`.

- [ ] **Step 4: Implementar el panel**

Crear `web/src/ui/RemindersPanel.tsx`:

```tsx
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
            <Link
              to="/check-in"
              search={{ date: c.target_date }}
              className="text-sm font-bold hover:underline"
            >
              {c.text}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

Nota: si el typecheck se queja de `search={{ date: ... }}` por las rutas tipadas de TanStack Router, usar `to={\`/check-in?date=${c.target_date}\`}` sin la prop `search`. La home y el check-in ya leen `date` desde la query string.

- [ ] **Step 5: Correr el test y verificar que pasa**

Run: `cd web && npm test -- src/ui/RemindersPanel.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 6: Montar el panel en la home**

En `web/src/routes/index.tsx`:

1. Agregar el import junto a los otros de `@/ui`:

```tsx
import { RemindersPanel } from "@/ui/RemindersPanel";
```

2. Insertar el panel justo después del `<p>` del saludo y antes del primer `<Reveal>`:

```tsx
        <p className="mt-4 text-sm text-muted">
          Hola, <span className="font-bold text-ink">{user.name}</span> · {fecha} ·{" "}
          {s.dimensions_active} dimensiones en marcha
        </p>

        <RemindersPanel />

        <Reveal className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
```

- [ ] **Step 7: Typecheck + build del front**

Run: `cd web && npm run build`
Expected: build exitoso (tsc sin errores).

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/commitments.ts web/src/ui/RemindersPanel.tsx web/src/ui/RemindersPanel.test.tsx web/src/routes/index.tsx
git commit -m "feat(web): panel de recordatorios de compromisos en la home"
```

---

## Task 4: Smoke de producción

**Files:**
- Create: `scripts/smoke-r31.sh`

- [ ] **Step 1: Escribir el script de smoke**

Crear `scripts/smoke-r31.sh` (basarse en el patrón de smokes previos; usar `grep -o` para extraer ids, nunca `sed` greedy):

```bash
#!/usr/bin/env bash
# Smoke R31 — Recordatorios de compromisos. Crea un compromiso vencido (ayer) y
# uno de hoy vía el POST de check-in (que guarda los compromisos del body para el
# día SIGUIENTE del `date` enviado), verifica que GET /commitments/pending
# devuelve ambos, marca el de hoy con toggle y verifica que ya no aparece.
set -euo pipefail
BASE="${BASE:-https://k4qv268333w8o5f2rze678hd.31.220.21.131.sslip.io}"
API="$BASE/api/v1"
TS="$(date +%s)"
EMAIL="smoke-r31-$TS@focus365.test"
PASS="Smoke-r31-$TS!"

# Fechas: anteayer y ayer (en UTC) para que los compromisos caigan en ayer y hoy.
ANTEAYER="$(date -u -v-2d +%Y-%m-%d 2>/dev/null || date -u -d '2 days ago' +%Y-%m-%d)"
AYER="$(date -u -v-1d +%Y-%m-%d 2>/dev/null || date -u -d '1 day ago' +%Y-%m-%d)"
HOY="$(date -u +%Y-%m-%d)"

echo "== register =="
REG="$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"name\":\"Smoke R31\"}")"
TOKEN="$(printf '%s' "$REG" | grep -o '"access_token":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$TOKEN" ] || { echo "  FALLO: sin access_token: $REG"; exit 1; }
echo "  register -> token OK"

auth=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')

echo "== crear compromisos (check-in anteayer -> vencido; ayer -> hoy) =="
curl -s "${auth[@]}" -X POST "$API/checkins" \
  -d "{\"date\":\"$ANTEAYER\",\"mood\":5,\"energy\":5,\"commitments\":[\"Vencido\"]}" >/dev/null
curl -s "${auth[@]}" -X POST "$API/checkins" \
  -d "{\"date\":\"$AYER\",\"mood\":5,\"energy\":5,\"commitments\":[\"De hoy\"]}" >/dev/null

echo "== GET /commitments/pending (debe traer ambos) =="
PEND="$(curl -s "${auth[@]}" "$API/commitments/pending")"
echo "  body: $(printf '%s' "$PEND" | head -c 300)"
printf '%s' "$PEND" | grep -q '"text":"Vencido"' || { echo "  FALLO: falta 'Vencido'"; exit 1; }
printf '%s' "$PEND" | grep -q '"text":"De hoy"'  || { echo "  FALLO: falta 'De hoy'"; exit 1; }
echo "  OK: pending trae vencido + hoy"

echo "== marcar 'De hoy' como cumplido y verificar que sale de pending =="
# id del compromiso cuyo texto es 'De hoy'
HOY_ID="$(printf '%s' "$PEND" | grep -o '{"id":"[^"]*","target_date":"[^"]*","text":"De hoy"' | grep -o '"id":"[^"]*"' | head -1 | sed 's/.*:"//; s/"//')"
[ -n "$HOY_ID" ] || { echo "  FALLO: no pude extraer el id de 'De hoy'"; exit 1; }
curl -s "${auth[@]}" -X POST "$API/commitments/$HOY_ID/toggle" >/dev/null
PEND2="$(curl -s "${auth[@]}" "$API/commitments/pending")"
printf '%s' "$PEND2" | grep -q '"text":"De hoy"' && { echo "  FALLO: 'De hoy' sigue en pending tras toggle"; exit 1; }
printf '%s' "$PEND2" | grep -q '"text":"Vencido"' || { echo "  FALLO: 'Vencido' desapareció"; exit 1; }
echo "  OK: 'De hoy' cumplido sale de pending; 'Vencido' permanece"

echo
echo "SMOKE R31: OK"
```

Nota: el campo JSON exacto del orden de claves del compromiso (`id`,`target_date`,`text`,`done`) viene del struct `Commitment` en `service.go` — el `grep -o` del Step asume ese orden; si el orden difiere, ajustar el patrón o extraer el id con un parser. Verificar el orden real corriendo `GET /commitments/pending` una vez.

- [ ] **Step 2: Hacer el script ejecutable**

Run: `chmod +x scripts/smoke-r31.sh`

- [ ] **Step 3: Commit**

```bash
git add scripts/smoke-r31.sh
git commit -m "chore(smoke): script de recordatorios de compromisos (rebanada 31)"
```

(El smoke se corre contra producción **después** del deploy manual en Coolify.)

---

## Self-Review (hecho por el autor del plan)

- **Cobertura del spec:** query `ListPendingCommitments` (T1) ✓; servicio `Pending` (T1) ✓; endpoint `GET /pending` (T2) ✓; reuso de `/toggle` (no se toca, usado en T3/smoke) ✓; `getPendingCommitments` (T3) ✓; `RemindersPanel` con grupos vencido/hoy, oculto si vacío, check inline, link al check-in (T3) ✓; montaje en la home (T3) ✓; tests backend y front ✓; smoke (T4) ✓.
- **Sin placeholders:** todos los pasos traen código/comando concreto.
- **Consistencia de tipos:** `Commitment{id,target_date,text,done}` igual en lib y backend; `ListPendingCommitmentsParams{UserID,TargetDate}` coincide con el uso en `Pending`; `getPendingCommitments`/`toggle` nombrados igual en lib, panel y test.
- **Riesgo conocido:** la prop `search` de TanStack Router puede no typecheckear contra rutas tipadas; el Step 4 de T3 documenta el fallback a query string. El orden de claves JSON del smoke se valida en T4 Step 1.
