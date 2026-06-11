# Plan 3 — Finanzas · Diseño

**Fecha:** 2026-06-10
**Estado:** Aprobado (diseño)
**Autor:** Gustavo (con Claude)
**Depende de:** Plan 1 (Cimientos + Auth) y Plan 2 (Check-in) — ambos en ramas de feature.

## 1. Objetivo

Segundo módulo de dominio de Focus 365: el **control de finanzas personales**.
El usuario registra sus **ingresos**, **gastos** y **transferencias**, y revisa el
**superávit** (ingresos − gastos) de cada **ciclo de pago mensual**. Incluye un
**importador** desde el servicio externo `money.quhou123.com` para traer el
historial de transacciones.

Es la rebanada 4 del diseño general (`2026-06-09-focus-365-design.md`, §7).
Reutiliza el patrón establecido por el check-in (migración → sqlc → handlers
protegidos → lib + página React con TanStack Query) y añade dos piezas nuevas:
**lógica de ciclo de pago** en el dominio y un **comando CLI** de importación.

## 2. Alcance

**Incluye:**
- Tabla `transactions` scoped por `user_id`, con columna `cycle` (mes financiero
  calculado en Go).
- Lógica de **ciclo de pago**: el mes financiero arranca el día de pago (último
  día hábil del mes).
- API REST bajo `/api/v1/finances`, protegida: crear, listar por ciclo, borrar,
  resumen del ciclo actual e historial de ciclos.
- Ruta dedicada `/finanzas` en la SPA: resumen + formulario de captura + lista
  del ciclo + historial.
- Enlace a finanzas desde el home.
- **Importador** `cmd/import`: comando CLI que trae transacciones de
  `money.quhou123.com` y las inserta idempotentemente (por `external_id`).
- Tests backend (store + dominio del ciclo + handlers) y frontend (lib + página).

**Fuera de alcance (planes posteriores):**
- Edición de transacciones (solo crear y borrar; corregir = borrar + recrear).
- Gráficos de tendencia del superávit.
- Multi-moneda (todo en pesos mexicanos, MXN).
- Tarjeta de finanzas en el dashboard real (Plan 7).
- Insights de IA sobre el gasto (Plan 8).
- Calendario de feriados para el día de pago (solo se ajustan fines de semana).

## 3. Decisiones de diseño (confirmadas)

| Decisión | Elección |
|----------|----------|
| Alcance | **Core nativo + importador** juntos en este plan |
| Meses financieros | **On-demand** (sin tabla `financial_months`); se calcula el `cycle` en Go y se guarda en la transacción |
| Monto | **BIGINT en centavos**, una sola moneda |
| Moneda | **Peso mexicano (MXN)** |
| Categorías | **Texto libre** (sin catálogo) |
| Transferencias | **Excluidas del superávit** (no cuentan como ingreso ni gasto) |
| Importador | **Comando CLI** `cmd/import`; token y baseURL vía env/flag |
| Día de pago | **Último día hábil del mes** (si cae sábado o domingo, retrocede al viernes); sin calendario de feriados |
| Mapeo del importador | **Pendiente** de una muestra real de `getTransactions`; el mapeo de campos se parametriza para completarse después |

## 4. Modelo de datos

Migración goose nueva (`api/db/migrations/0003_transactions.sql`):

```sql
-- +goose Up
CREATE TABLE transactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL CHECK (type IN ('income','expense','transfer')),
    amount      BIGINT NOT NULL CHECK (amount >= 0),  -- centavos (MXN)
    occurred_on DATE NOT NULL,
    cycle       DATE NOT NULL,            -- primer día del mes financiero
    category    TEXT NOT NULL DEFAULT '',
    remark      TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','import')),
    external_id TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_transactions_user_cycle
    ON transactions (user_id, cycle DESC, occurred_on DESC);
CREATE UNIQUE INDEX uq_transactions_user_external
    ON transactions (user_id, external_id) WHERE external_id IS NOT NULL;

-- +goose Down
DROP TABLE transactions;
```

- `amount BIGINT` en **centavos** evita errores de redondeo de coma flotante;
  el frontend convierte pesos↔centavos.
- `cycle` se calcula en Go al guardar y se persiste, para que listar/resumir por
  ciclo sea un simple `WHERE cycle = $1` sin recomputar.
- `occurred_on` es la fecha real del movimiento; `cycle` es el mes financiero al
  que pertenece (pueden diferir alrededor del día de pago).
- `external_id` solo lo usan las transacciones importadas; el índice único
  parcial garantiza idempotencia del importador por `(user_id, external_id)`.
- `source` distingue `manual` vs `import` (útil para depurar y para no borrar
  importadas por accidente en la UI).
- El `CHECK` en BD es defensa en profundidad; la validación primaria está en el
  handler (mensajes claros).

## 5. Lógica de ciclo de pago

Paquete de dominio nuevo (`api/internal/finance`), funciones puras y testeables:

```
payday(año, mes)
    = último día del mes
    → si cae sábado, retrocede al viernes (−1)
    → si cae domingo, retrocede al viernes (−2)
    (solo fines de semana; sin calendario de feriados)

financialMonth(fecha)
    = sea p = payday(año de fecha, mes de fecha)
    → si fecha >= p  → el ciclo es el mes SIGUIENTE  (mes+1)
    → si fecha <  p  → el ciclo es el mes ACTUAL     (mes)

cycle(fecha)
    = primer día (día 1) del financialMonth(fecha)
```

Ejemplos (2026):
- `29 may 2026` (viernes, es el payday de mayo porque 30/31 son sáb/dom) → ciclo **Junio** (`2026-06-01`).
- `10 jun 2026` → payday de junio es `30 jun` (martes); 10 < 30 → ciclo **Junio** (`2026-06-01`).
- `30 jun 2026` (es el payday de junio) → 30 >= 30 → ciclo **Julio** (`2026-07-01`).

El "ciclo actual" para el resumen y los defaults se calcula con
`cycle(hoy)` usando la fecha que envía el frontend (fecha local del cliente,
igual que en el check-in, para evitar supuestos de timezone en el servidor).

## 6. API

Base `/api/v1/finances`, montada con `RequireAuth`. El `user_id` sale del
contexto (middleware de Plan 1); **nunca** del body. Errores con el mismo formato
`{"error": "..."}` de Plan 1. Los montos viajan en **centavos**.

### `POST /api/v1/finances/transactions` — crear
Body:
```json
{ "type": "expense", "amount": 15050, "occurred_on": "2026-06-10",
  "category": "comida", "remark": "súper" }
```
- Validación: `type` ∈ {income, expense, transfer}; `amount` requerido y ≥ 0;
  `occurred_on` requerida (formato `YYYY-MM-DD`); `category`/`remark` opcionales.
- El servidor calcula `cycle = cycle(occurred_on)` y `source = 'manual'`.
- Respuesta `201` con la transacción completa.

### `GET /api/v1/finances/transactions?cycle=YYYY-MM`
- Lista las transacciones del ciclo indicado, **descendente por fecha**.
- `cycle` opcional como `YYYY-MM`; default = ciclo actual (`cycle(hoy)`).
- Respuesta `200` con un array (posiblemente vacío).

### `DELETE /api/v1/finances/transactions/{id}`
- Borra la transacción `{id}` **si pertenece al usuario**.
- Respuesta `204` si borró; `404` si no existe o no es del usuario.

### `GET /api/v1/finances/summary?cycle=YYYY-MM`
- Resumen de un ciclo (default = ciclo actual):
```json
{ "cycle": "2026-06", "income": 5000000, "expense": 3200000,
  "net": 1800000, "status": "pendiente" }
```
- `net = Σ income − Σ expense` (las **transferencias se excluyen**).
- `status`:
  - **pendiente** — es el ciclo en curso (aún no cierra).
  - **verde** — ciclo cerrado con `net >= 0` (hubo superávit).
  - **rojo** — ciclo cerrado con `net < 0` (hubo déficit).
- Un ciclo "cierra" cuando llega su día de pago; es decir, está cerrado si
  `cycle < cycle(hoy)`.

### `GET /api/v1/finances/cycles`
- Historial de ciclos del usuario con sus totales, **descendente por ciclo**.
- Cada elemento tiene la misma forma que `/summary` (cycle, income, expense, net,
  status). El ciclo actual aparece como `pendiente`.
- Respuesta `200` con un array.

**Queries sqlc** (`api/db/queries/transactions.sql`):
- `CreateTransaction` — `INSERT ... RETURNING *`.
- `ListTransactionsByCycle` — `WHERE user_id=$1 AND cycle=$2 ORDER BY occurred_on DESC, created_at DESC`.
- `DeleteTransaction` — `DELETE ... WHERE id=$1 AND user_id=$2` (filtra por dueño).
- `SummarizeCycles` — agrega por `cycle`: `SUM` condicional de income/expense
  (`FILTER (WHERE type='income'|'expense')`), `GROUP BY cycle ORDER BY cycle DESC`.
- `UpsertImportedTransaction` — `INSERT ... ON CONFLICT (user_id, external_id) DO UPDATE SET ...` para el importador.

## 7. Estructura de código

**Backend dominio** (paquete nuevo `api/internal/finance`, espejo de `checkin`):
- `cycle.go` — funciones puras `payday`, `financialMonth`, `cycle` (§5) +
  helpers de parseo/format `YYYY-MM`.
- `service.go` — `Service{ q *store.Queries }`; métodos `Create(ctx, userID, in)`,
  `ListByCycle(ctx, userID, cycle)`, `Delete(ctx, userID, id)`,
  `Summary(ctx, userID, cycle)`, `Cycles(ctx, userID)`. Calcula `cycle`, traduce
  entre tipos del dominio y `store`, y decide el `status` de cada ciclo.
- `handler.go` — `Routes(svc *Service) http.Handler` (chi); usa `httpx`
  (de Plan 2) para decodificar/validar/responder y `auth.UserIDFromContext`.
- Montaje en `internal/server/server.go`:
  `r.Mount("/finances", finance.Routes(...))` bajo `/api/v1`, dentro del grupo
  envuelto con `auth.RequireAuth(tokenManager)`.

**Importador** (paquete nuevo `api/cmd/import` + `api/internal/finance/importer.go`):
- `cmd/import/main.go` — comando CLI. Lee config por flags/env:
  `--token`/`FOCUS_IMPORT_TOKEN`, `--base-url`/`FOCUS_IMPORT_BASE_URL`,
  `--email` (a qué usuario de Focus asignar), `--from`/`--to` (rango de fechas),
  `DATABASE_URL`. Abre el pool, resuelve el `user_id` por email, llama al importer.
- `importer.go` — cliente HTTP del servicio externo + mapeo a `transactions`:
  - `fetchTransactions(ctx, cfg, from, to) ([]external, error)` — pagina/llama a
    `getTransactions` del servicio externo.
  - `mapToTransaction(external) store.UpsertImportedTransactionParams` — **mapeo de
    campos PENDIENTE** de una muestra real de la respuesta de `getTransactions`
    (forma del JSON, nombres de campos, cómo deriva `type`/`amount`/`occurred_on`/
    `external_id`). Se completará cuando Gustavo aporte la muestra.
  - Inserta con `UpsertImportedTransaction` (`source='import'`), idempotente por
    `external_id`; calcula `cycle` con la misma lógica del dominio.
- El importador reusa `internal/finance.cycle()` para no duplicar la regla.

**Frontend:**
- `web/src/lib/finances.ts` — tipos `Transaction`, `TransactionInput`,
  `CycleSummary`; funciones `create(input)`, `listByCycle(cycle)`,
  `remove(id)`, `summary(cycle?)`, `cycles()` sobre `apiFetch`. Helpers
  `pesosToCents`/`centsToPesos` y `currentCycleString()` (`YYYY-MM` local).
- `web/src/routes/finanzas.tsx` — ruta protegida. `useQuery` para summary del
  ciclo actual + lista del ciclo + historial de ciclos; `useMutation` para crear
  y para borrar (invalidan las queries al éxito). Formulario: select de tipo,
  input de monto en pesos (se convierte a centavos), fecha, categoría, nota,
  botón Guardar. Lista del ciclo con botón borrar por fila. Sección de historial
  de ciclos con su `status` (pendiente/verde/rojo) coloreado. Estilo Warm
  Discipline.
- Enlace desde `web/src/routes/index.tsx` al `/finanzas`.

## 8. Flujo de datos

1. Usuario abre `/finanzas`. Se calcula el ciclo actual en el cliente
   (`currentCycleString()`), `useQuery` pide summary + lista + ciclos con
   `Authorization: Bearer`.
2. Al guardar una transacción: `useMutation` → `POST .../transactions`. El
   servidor calcula el `cycle` desde `occurred_on` y persiste. Al éxito, invalida
   summary + lista + ciclos → la UI refleja el cambio.
3. Al borrar: `useMutation` → `DELETE .../transactions/{id}`; invalida lo mismo.
4. nginx (mismo origen) proxya `/api` → Go; `RequireAuth` valida el token e
   inyecta `user_id`; los handlers operan scoped por ese `user_id`.
5. Importación (offline, operador): `go run ./cmd/import --email=... --from=... --to=...`
   → trae de `money.quhou123.com` → upsert idempotente en `transactions` con
   `source='import'`. La SPA las muestra como cualquier otra transacción del ciclo.

## 9. Manejo de errores

- `400` validación (mensajes por campo, reusando el helper `httpx` de Plan 2):
  tipo inválido, monto negativo/faltante, fecha faltante/mal formada, `cycle`
  con formato distinto de `YYYY-MM`.
- `401` sin token o token inválido (lo da `RequireAuth`).
- `404` al borrar una transacción inexistente o de otro usuario.
- `500` errores de BD → `{"error":"error interno"}`, sin filtrar detalles.
- Importador (CLI): errores claros a stderr y exit code ≠ 0 si falla la
  autenticación externa, la red o el mapeo; reporta cuántas insertó/actualizó.
- Frontend: las mutaciones capturan `ApiError` y muestran `err.message` cerca del
  formulario; estados de carga deshabilitan los botones.

## 10. Testing

**Backend (Go, requiere `TEST_DATABASE_URL`):**
- `finance/cycle` (puro, sin BD): `payday` ajusta sábado→viernes y domingo→viernes;
  `financialMonth`/`cycle` para fechas antes/igual/después del payday, incluido
  cruce de año (diciembre→enero).
- `store`: crear inserta; listar por ciclo ordena desc; borrar respeta dueño
  (no borra de otro usuario); `SummarizeCycles` agrega income/expense y excluye
  transfers; upsert importado es idempotente por `external_id`.
- `handler`: `POST` crea (201 + cuerpo con `cycle` calculado); `GET` lista por
  ciclo; `DELETE` 204 y luego 404; `GET /summary` calcula net y status
  (pendiente para el actual, verde/rojo para cerrado); validación (tipo/monto/
  fecha) → 400; sin token → 401; aislamiento por usuario (user B no ve ni borra
  lo de A).

**Frontend (Vitest + Testing Library):**
- `lib/finances`: `create`/`listByCycle`/`remove`/`summary`/`cycles` llaman a la
  ruta y método correctos (fetch mockeado), incluyen Bearer cuando hay token;
  `pesosToCents`/`centsToPesos` redondean bien.
- `finanzas` page: renderiza el resumen y la lista; al guardar dispara el POST
  con el monto en centavos; al borrar dispara el DELETE; muestra el historial de
  ciclos con su status.

**Importador:** prueba unitaria de `mapToTransaction` con una muestra (cuando
Gustavo aporte el JSON real de `getTransactions`); hasta entonces, el mapeo y su
test quedan marcados como pendientes en el plan.

## 11. Criterios de aceptación

- `docker compose up` + migraciones: la tabla `transactions` existe.
- Logueado, en `/finanzas`: crear una transacción responde 201 y persiste;
  recargar la muestra en el ciclo correcto; borrar la quita.
- El resumen del ciclo actual muestra `net = ingresos − gastos` (transferencias
  excluidas) y `status = pendiente`; un ciclo pasado muestra verde/rojo.
- El historial lista los ciclos recientes descendente.
- Una transacción del día del pago cae en el ciclo del mes siguiente (regla §5).
- Un usuario nunca ve ni borra las transacciones de otro.
- Sin sesión, `/finanzas` redirige a `/login`; la API responde 401.
- El importador inserta idempotentemente (correr dos veces no duplica).
- `go build/vet/test` y `tsc/vitest` en verde.
