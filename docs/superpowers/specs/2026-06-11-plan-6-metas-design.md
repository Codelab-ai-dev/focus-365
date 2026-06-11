# Plan 6 — Metas (Goals) — Diseño

**Fecha:** 2026-06-11
**Rebanada:** 6 de 8 del roadmap (`docs/superpowers/specs/2026-06-09-focus-365-design.md` §7).
**Dimensión:** 5. Metas.

## Objetivo

CRUD de metas personales por usuario, con progreso manual (0-100%), estado manual
(activa / completada / pausada) y fecha límite opcional con marca de "vencida".
Espeja los patrones ya establecidos por la rebanada de Hábitos (Plan 5): handler chi,
servicio con dominio puro, vista JSON calculada, página frontend con pestañas.

## Decisiones de diseño (locked)

- **Progreso:** slider manual 0-100%. No se deriva de nada.
- **Dimensión:** enum fijo — `checkin | finanzas | entrenamiento | mente | general`.
- **Estado:** totalmente manual (`active | done | paused`). Independiente del progreso:
  llegar a 100% **no** cambia el estado automáticamente.
- **Deadline:** opcional. Una meta `active` con `deadline` en el pasado (según la fecha
  del cliente) se marca `overdue`.
- **Vista de lista:** pestañas por estado — Activas / Completadas / Pausadas (espejo de
  Activos/Archivados en Hábitos).
- **API:** Enfoque 1 — un único `PATCH /goals/{id}` que acepta cualquier subconjunto de
  campos mutables. Más `GET`, `POST`, `DELETE`. 4 endpoints.

## Sección 1 — Modelo de datos

Migración `api/db/migrations/0006_goals.sql`:

```sql
-- +goose Up
CREATE TABLE goals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    dimension   TEXT NOT NULL,                    -- checkin|finanzas|entrenamiento|mente|general
    status      TEXT NOT NULL DEFAULT 'active',   -- active|done|paused
    progress    INT  NOT NULL DEFAULT 0,          -- 0..100
    deadline    DATE,                             -- opcional (NULL = sin fecha)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT goals_progress_range CHECK (progress BETWEEN 0 AND 100),
    CONSTRAINT goals_status_valid   CHECK (status IN ('active','done','paused'))
);
CREATE INDEX idx_goals_user_status ON goals (user_id, status, created_at DESC);

-- +goose Down
DROP TABLE goals;
```

- `deadline` es `DATE` (fecha de calendario, sin zona horaria).
- Los `CHECK` son red de seguridad a nivel DB; la validación primaria vive en la capa app
  (validator `oneof` / `min,max`).
- `dimension` no lleva CHECK en DB (se valida en app) para no obligar a migrar la tabla si
  cambian los valores admitidos.
- Todo scopeado por `user_id`; cascade al borrar usuario. No hay tablas hijas.

## Sección 2 — API (REST)

Todos los endpoints bajo `RequireAuth`, montados en `/api/v1/goals`, scopeados por `user_id`.

### `GET /goals?status=active&today=2026-06-11`
- `status` opcional: `active` (default) | `done` | `paused`. Fuera del enum → 400.
- `today` opcional (zona del cliente); fallback a UTC midnight. Sirve para `overdue`.
- Devuelve lista ordenada por `created_at DESC`.

### `POST /goals?today=...`
- Body: `{ title, dimension, deadline? }`.
- Validación: `title` required (trim, no vacío); `dimension oneof=checkin finanzas
  entrenamiento mente general`; `deadline` opcional (`YYYY-MM-DD`).
- Crea con `status='active'`, `progress=0`. → 201 con la meta creada.

### `PATCH /goals/{id}?today=...` (parche parcial)
- Body con cualquier subconjunto de: `progress` (0..100), `status` (`active|done|paused`),
  `title`, `dimension` (enum), `deadline` (string `YYYY-MM-DD` o `null` para limpiar).
- Sólo aplica los campos presentes en el JSON (ver Sección 3, manejo de presencia).
- → 200 con la meta actualizada; 404 si no es del usuario; 400 si algún campo viola validación.

### `DELETE /goals/{id}`
- → 204 si borró; 404 si no era del usuario.

### Errores transversales
- 401 sin token (middleware `RequireAuth`).
- 404 cuando la meta no pertenece al usuario: query scopeada por `user_id` →
  `pgx.ErrNoRows` → servicio devuelve `(nil, nil)` → handler responde 404.
- 400 validación con labels en español vía `httpx`.

### `overdue` (calculado, no persistido)
`overdue = status == 'active' && deadline != nil && deadline < today`, con `today` del
cliente (`?today=`) o UTC midnight por defecto. Se computa en el servicio al construir la vista.

### Labels nuevos en `httpx.fieldLabel`
`title→"título"`, `dimension→"dimensión"`, `deadline→"fecha límite"`,
`progress→"progreso"`, `status→"estado"`.

## Sección 3 — Vista JSON / dominio

JSON serializado (devuelto por `GET`/`POST`/`PATCH`):

```json
{
  "id": "uuid",
  "title": "Correr una 10k",
  "dimension": "entrenamiento",
  "status": "active",
  "progress": 40,
  "deadline": "2026-08-01",
  "overdue": false,
  "created_at": "2026-06-11T10:00:00Z"
}
```

`deadline` serializa `null` cuando no hay fecha.

Tipos en `internal/goals/types.go`:

- `Goal` — struct con tags JSON; `Deadline *time.Time` (puntero → `null`); `Overdue bool`.
- `GoalInput` (POST):
  - `Title string` `validate:"required"`
  - `Dimension string` `validate:"required,oneof=checkin finanzas entrenamiento mente general"`
  - `Deadline *string` (parseado a fecha `YYYY-MM-DD` si viene)
- `GoalPatch` (PATCH) — todos punteros para distinguir "ausente":
  - `Title *string`
  - `Dimension *string` `validate:"omitempty,oneof=checkin finanzas entrenamiento mente general"`
  - `Status *string` `validate:"omitempty,oneof=active done paused"`
  - `Progress *int` `validate:"omitempty,min=0,max=100"`
  - `Deadline` — manejo especial de presencia (abajo).

### Manejo de presencia del `deadline` en PATCH
Hay que distinguir tres casos: clave **ausente** (no tocar), `null` explícito (limpiar la
fecha), `"YYYY-MM-DD"` (fijar). Implementación: el handler decodifica el body a
`map[string]json.RawMessage` (o el campo `Deadline json.RawMessage` dentro de la struct) y:
- clave ausente → no incluir `deadline` en el UPDATE.
- `null` → setear `deadline = NULL`.
- string fecha → parsear y setear.

El mismo criterio de presencia aplica para decidir qué columnas entran en el `UPDATE`
(sólo los campos presentes). Las queries sqlc usan `COALESCE`/columnas condicionales o,
más simple, un set de queries acotado; el plan detallará la estrategia exacta de sqlc.

### Servicio `internal/goals/service.go`
- `List(ctx, userID, status, today) ([]Goal, error)`
- `Create(ctx, userID, in GoalInput, today) (*Goal, error)`
- `Patch(ctx, userID, id, patch GoalPatch, today) (*Goal, error)` — `(nil,nil)` si no es del usuario.
- `Delete(ctx, userID, id) (bool, error)`
- `buildGoal(row store.Goal, today) *Goal` — arma la vista calculando `overdue`.

Todo scopeado por `user_id`; `pgx.ErrNoRows → (nil,nil) → 404` en el handler.

## Sección 4 — Frontend `/metas`

### `web/src/lib/goals.ts` (espejo de `habits.ts`)
- Tipos `Goal`, `GoalInput`, `GoalPatch`, `GoalStatus`.
- `listGoals(status, today)`, `createGoal(input, today)`, `patchGoal(id, patch, today)`,
  `deleteGoal(id)`.
- Reutiliza el `apiFetch` con token y `todayString()` existentes.

### `web/src/routes/metas.tsx`
- **Pestañas de estado:** Activas / Completadas / Pausadas (patrón visual de `disciplina.tsx`).
  TanStack Query con queryKey `['goals', status]`.
- **Tarjetas de meta:** título, chip de dimensión, barra de progreso, deadline (si hay).
  Las `active` vencidas resaltan en rojo con etiqueta "Vencida".
- **Slider de progreso** 0-100% → `patchGoal(id, { progress })` con invalidación de query.
- **Botones de estado** según pestaña:
  - Activas → "Completar" (`status:'done'`) y "Pausar" (`status:'paused'`).
  - Completadas / Pausadas → "Reactivar" (`status:'active'`).
  - Siempre: "Borrar".
- **Form de creación:** título + select de dimensión + fecha límite opcional → `createGoal`.
- Estados de carga y vacío ("Aún no tenés metas…"), como las demás páginas.

### `web/src/routes/index.tsx`
Agregar enlace/tarjeta "Metas" en el home, junto a Disciplina, Finanzas, etc.

### `routeTree.gen.ts`
Se regenera con `npx vite build` solo, antes de `npm run build` (`tsc -b && vite build`).

### Colores
Paleta "Warm Discipline" existente; barra de progreso con `amber-brand`/`streak`,
"Vencida" en un rojo de la escala. Sin nuevos colores en `tailwind.config.js` salvo necesidad.

## Sección 5 — Testing y criterios de aceptación

### Backend — unit (dominio puro)
`buildGoal`/`overdue`:
- `active` + `deadline` pasado → `overdue=true`.
- `done`/`paused` con deadline pasado → `false`.
- sin deadline → `false`.
- deadline futuro → `false`.
- deadline == today → `false`.

### Backend — integración (`handler_test.go`, patrón Hábitos con `testutil.NewDB`)
1. `TestCreateAndList` — POST → 201; GET `?status=active` lista con `progress=0`,
   `status=active`, `overdue=false`.
2. `TestPatchProgress` — PATCH `{progress:40}` → 200, progreso 40, status sigue `active`.
3. `TestPatchStatusTransitions` — `active→done→active`; aparece/desaparece de cada tab.
4. `TestProgress100DoesNotChangeStatus` — PATCH `{progress:100}` → status sigue `active`.
5. `TestOverdue` — meta con deadline pasado y `?today=` posterior → `overdue=true`;
   tras `status:'done'` → `false`.
6. `TestDeadlineClear` — PATCH `deadline:null` limpia; PATCH sin la clave conserva.
7. `TestValidation` — dimensión inválida → 400; status inválido → 400; progress 150 → 400;
   title vacío → 400.
8. `TestDelete` — DELETE → 204; segundo DELETE → 404.
9. `TestRequiresAuth` — sin token → 401.
10. `TestUserIsolation` — B no ve metas de A; B no puede PATCH ni DELETE la de A → 404.

### Frontend — Vitest (`goals.test.ts` + `metas.test.tsx`)
- `goals.ts`: cada función arma URL/método/body correctos (incluye `?status=` y `?today=`).
- `metas.tsx`: render de pestañas, cambio de pestaña, slider dispara PATCH, botones de estado
  disparan el PATCH correcto, "Vencida" para activas vencidas, estado vacío.

### E2E docker smoke (bash, como Plan 5)
Levantar stack, registrar usuario, crear meta, PATCH progreso/estado, verificar overdue con
`?today=`, aislamiento entre usuarios, borrar. Salida `SMOKE OK`.

### Criterios de aceptación
- CRUD completo de metas, scopeado por usuario.
- `progress` y `status` mutables de forma independiente (100% no cambia estado).
- `overdue` calculado correctamente con la fecha del cliente.
- Pestañas Activas/Completadas/Pausadas funcionando.
- `make check` verde + frontend Vitest verde + smoke `SMOKE OK`.
