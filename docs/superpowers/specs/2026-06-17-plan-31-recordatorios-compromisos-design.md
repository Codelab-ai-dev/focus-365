# Rebanada 31 — Recordatorios de compromisos (panel in-app) · Diseño

**Fecha:** 2026-06-17
**Estado:** Diseño aprobado.

## Resumen

Un **panel de recordatorios arriba de la home** que destaca los compromisos
**pendientes** (no cumplidos) cuya fecha objetivo es **hoy o anterior**. Hoy los
compromisos solo se ven entrando al check-in de un día concreto; este panel los
trae al primer pantallazo para que nada se escape.

In-app puro: **sin** push del navegador, email, service worker ni cron. (El push
del navegador queda como posible rebanada futura.)

## Alcance

- Muestra compromisos con `done = false` y `target_date <= hoy`, en dos grupos:
  - **Vencidos** (`target_date < hoy`) — destacados con estilo de alerta, primero.
  - **Hoy** (`target_date = hoy`).
- Cada ítem tiene un check para **marcarlo cumplido** sin salir de la home.
- Tocar el texto del compromiso lleva al check-in de su día (`/check-in?date=…`).
- Si no hay nada pendiente, el panel **no se renderiza** (cero ruido).
- **Fuera de alcance:** push/email/notificaciones del SO; "mover a hoy" /
  re-agendar; compromisos futuros; recordatorios por hora del día.

## Backend

### Query nueva (`api/db/queries/commitments.sql`)

```sql
-- name: ListPendingCommitments :many
SELECT * FROM commitments
WHERE user_id = $1 AND done = false AND target_date <= $2
ORDER BY target_date ASC, position ASC;
```

`$2` = hoy (UTC, truncado al día). Trae vencidos + hoy en un solo viaje, vencidos
primero. Excluye cumplidos y futuros por el `WHERE`.

### Service (`api/internal/commitments/service.go`)

```go
// Pending devuelve los compromisos sin cumplir con target_date <= today
// (vencidos + hoy), vencidos primero. Para el panel de recordatorios.
func (s *Service) Pending(ctx context.Context, userID uuid.UUID, today time.Time) ([]Commitment, error)
```

Usa la query nueva y el `toView`/`mapViews` existentes. Sin lógica de
clasificación: devuelve la lista plana ordenada; el front separa vencido/hoy.

### Endpoint (`api/internal/commitments/handler.go`)

Agregar a `Routes`: `r.Get("/pending", handlePending(svc))`.

`handlePending` (análogo a `handleDue`):
- `userID` del contexto; 401 si no hay.
- `today := time.Now().UTC().Truncate(24 * time.Hour)` (sin parámetro `date`).
- `svc.Pending(ctx, userID, today)`; 500 en error.
- Responde `{"commitments": [...]}` (200).

**Marcar cumplido** reutiliza el endpoint existente `POST /commitments/{id}/toggle`.
No se agrega nada para eso.

## Frontend

### `web/src/lib/commitments.ts`

Agregar:
```ts
export async function getPendingCommitments(): Promise<Commitment[]>
```
GET a `/commitments/pending`, devuelve `body.commitments`. Reutiliza el tipo
`Commitment` y el `toggleCommitment()` ya existentes.

### `web/src/ui/RemindersPanel.tsx` (componente nuevo)

- Query `["commitments", "pending"]` → `getPendingCommitments`.
- Si la lista está vacía (o `isLoading`/`isError`), **no renderiza nada** (return
  null) — el panel aparece solo cuando hay pendientes.
- Separa la lista en `vencidos` (`target_date < hoy`) y `hoy` (`target_date = hoy`)
  comparando con `todayString()` (helper ya usado en la home).
- Card neo-brutalista (`border-2 border-ink shadow-brutal bg-surface`) arriba de la
  grilla. Sección "Vencidos" con estilo de alerta (`bg-danger-bg`/`text-danger-fg`)
  y sección "Hoy".
- Cada ítem: checkbox + texto. El texto es un `Link` a `/check-in?date=<target_date>`.
- Al marcar el check: mutación que llama `toggleCommitment(id)` y al éxito invalida
  `["commitments", "pending"]` y `["dashboard"]`. Mientras la mutación de ese ítem
  está en vuelo, el check queda deshabilitado (evita doble toggle).

### `web/src/routes/index.tsx`

Renderizar `<RemindersPanel />` arriba de la grilla de tiles (dentro del estado
autenticado, después del saludo/encabezado).

## Manejo de errores

- Backend: 401 sin auth, 500 en error de DB (igual que el resto del módulo).
- Frontend: si la query falla, el panel no se muestra (no rompe la home). El error
  de toggle deja el check como estaba (sin optimismo agresivo); TanStack Query
  re-sincroniza al invalidar.

## Tests

### Backend (`-p 1`, `TEST_DATABASE_URL`)

- `service_test.go`: `Pending` incluye vencidos y hoy, excluye cumplidos y futuros,
  y ordena vencidos antes que hoy.
- `handler_test.go`: `GET /pending` devuelve 200 con los pendientes; 401 sin auth.

### Frontend (Vitest + `npm run build`)

- `RemindersPanel.test.tsx`: renderiza vencidos y hoy en grupos separados; no se
  muestra si la lista está vacía; al marcar un check dispara `toggleCommitment` e
  invalida las queries.

## Verificación / smoke

`scripts/smoke-r31.sh`: registra un usuario y crea compromisos vía el `POST
/check-in`, recordando que ese endpoint guarda los compromisos del body para el
**día siguiente** del `date` enviado:

- `POST /check-in` con `date = anteayer` y `commitments:["vencido"]` → compromiso
  con `target_date = ayer` (vencido).
- `POST /check-in` con `date = ayer` y `commitments:["de hoy"]` → compromiso con
  `target_date = hoy`.

Luego `GET /commitments/pending` debe devolver **ambos** (vencido primero). Toma el
`id` del de hoy, `POST /commitments/{id}/toggle`, y verifica que `pending` ya solo
trae el vencido. (El `POST /check-in` requiere `mood` y `energy` 1..10.)
