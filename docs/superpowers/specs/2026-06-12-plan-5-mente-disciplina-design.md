# Plan 5 — Mente / Disciplina (hábitos + rachas) — Diseño

**Fecha:** 2026-06-12
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Cuarta dimensión del sistema (módulo 4 del design global): seguimiento de **hábitos y retos** con **rachas** que gamifican la disciplina. Es distinto del check-in diario (que ya guarda un score subjetivo de `discipline` 1–10): acá se trackean hábitos/retos concretos y objetivos.

Modelo **unificado**: una sola entidad `habit` que puede ser "abierta" (corre indefinida) o "con reto" (meta opcional de N días). Registro **binario** diario (hecho / no hecho), extensible a cantidades en una rebanada futura. Racha **diaria estricta** con récord histórico. Marcado de **hoy + ayer** (ventana de gracia de un día). Las rachas se **calculan desde los logs** (sin contadores desnormalizados).

**Fuera de alcance (por ahora):** cantidades/objetivos numéricos por día, agenda por días de semana, calendario de edición completo del historial, desarchivar, archivado automático al cumplir la meta, insignias persistidas. Se evalúan en rebanadas futuras.

## 2. Modelo de datos

Migración `0005_habits.sql`. Dos tablas, scoped por `user_id` como el resto del sistema. UUIDs, timestamps UTC.

### `habits`

| columna | tipo | nota |
|---|---|---|
| `id` | UUID PK `gen_random_uuid()` | |
| `user_id` | UUID NOT NULL FK → `users(id)` ON DELETE CASCADE | |
| `name` | TEXT NOT NULL | nombre del hábito/reto |
| `target_days` | INT NULL | meta de N días (challenge). NULL = hábito abierto |
| `archived_at` | TIMESTAMPTZ NULL | archivado manual; NULL = activo |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

Índices:
- Único **parcial** `uq_habits_user_name ON habits (user_id, lower(name)) WHERE archived_at IS NULL` — no se permiten dos hábitos **activos** con el mismo nombre (sin distinguir mayúsculas), pero un nombre archivado no bloquea recrearlo.
- `idx_habits_user_active ON habits (user_id, created_at DESC)` — listar por usuario.

### `habit_logs`

Una fila = ese día el hábito se cumplió.

| columna | tipo | nota |
|---|---|---|
| `id` | UUID PK `gen_random_uuid()` | |
| `habit_id` | UUID NOT NULL FK → `habits(id)` ON DELETE CASCADE | |
| `day` | DATE NOT NULL | el día cumplido |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

Índices:
- Único `uq_habit_logs_habit_day ON habit_logs (habit_id, day)` — un registro por hábito por día. Marcar = upsert; desmarcar = delete.
- `idx_habit_logs_habit_day ON habit_logs (habit_id, day DESC)` — leer logs por hábito.

> Nota: la columna `value` para cantidades futuras se omite a propósito (YAGNI); se agrega en la rebanada que construya la lógica de cantidades.

## 3. Cálculo de rachas

Se deriva todo de los `habit_logs` (días ordenados) en Go al listar. Helper puro y testeable:

```
computeStreaks(days []time.Time, today time.Time) (current, best int, doneToday, doneYesterday bool)
```

Reglas:
- `doneToday` / `doneYesterday`: existe log para hoy / ayer.
- **Racha actual (`current_streak`)**: días consecutivos contando hacia atrás desde un *ancla*:
  - ancla = hoy si está hecho; si no, ayer si está hecho; si no → `current = 0`.
  - Marcar ayer pero todavía no hoy mantiene la racha **viva** (estado "pendiente hoy") en vez de caer a 0; coherente con la ventana de gracia.
  - Ni hoy ni ayer marcados → la racha se cortó, vale 0.
- **Récord (`best_streak`)**: la corrida consecutiva más larga sobre todo el historial.

Ejemplo: logs `[lun, mar, mié, vie, sáb]`, hoy = domingo sin marcar → ancla en sáb → `current = 2` (vie, sáb), `best = 3` (lun–mié), `doneToday = false`, `doneYesterday = true`.

El **progreso del reto** (si `target_days` no es NULL) es presentación: `current_streak / target_days`. Al alcanzar la meta se marca el hito visual; no hay archivado automático.

## 4. API

Paquete Go `internal/habits`, montado bajo `RequireAuth` en `/api/v1/habits`. Todo scoped por `user_id` del contexto; `{id}` ajeno o inexistente → 404.

| # | Método y ruta | Request | Respuesta |
|---|---|---|---|
| 1 | `GET /api/v1/habits` | query `?archived=true` opcional | 200 lista de `Habit` (ver §5). Sin query: activos. Con `archived=true`: archivados |
| 2 | `POST /api/v1/habits` | `{ "name": string, "target_days": int? }` | 201 `Habit` |
| 3 | `POST /api/v1/habits/{id}/check` | `{ "day": "YYYY-MM-DD"?, "done": bool }` | 200 `Habit` recalculado |
| 4 | `POST /api/v1/habits/{id}/archive` | — | 200 `Habit` |
| 5 | `DELETE /api/v1/habits/{id}` | — | 204 |

Detalles:
- **#2 create:** `name` requerido (trim); `target_days` opcional `omitempty,min=1`. **Idempotente por nombre activo:** si ya existe un hábito activo con ese nombre (sin distinguir mayúsculas), se devuelve ese mismo (vía `ON CONFLICT ... DO UPDATE SET name = habits.name RETURNING *`, igual que el catálogo de ejercicios en training). Siempre responde 201 con el `Habit`. Nota: `target_days` de un hábito ya existente no se modifica en el create idempotente.
- **#3 check (toggle único):** `day` por defecto hoy. Solo se acepta **hoy o ayer** (zona del server); cualquier otro día → 400. `done:true` → `UpsertHabitLog` (ON CONFLICT DO NOTHING); `done:false` → `DeleteHabitLog`. Devuelve el hábito recalculado.
- **#4 archive:** set `archived_at = now()`. Sale de la lista activa.
- **#5 delete:** borra hábito + logs (cascade). Para corregir errores.
- Sin desarchivar en esta rebanada.

**Errores:** 400 (validación / día fuera de la ventana hoy-ayer), 401 (sin sesión), 404 (no es del usuario / no existe), 500.

## 5. Forma de los datos (JSON)

```jsonc
// Habit (respuesta)
{
  "id": "uuid",
  "name": "Leer 20 min",
  "target_days": 21,          // o null
  "current_streak": 5,
  "best_streak": 8,
  "done_today": true,
  "done_yesterday": true,
  "archived_at": null,        // o ISO timestamp si archivado
  "created_at": "ISO timestamp"
}
```

```jsonc
// HabitInput (POST create)
{ "name": "Leer 20 min", "target_days": 21 }   // target_days opcional

// CheckInput (POST check)
{ "day": "2026-06-12", "done": true }           // day opcional (default hoy)
```

## 6. Estructura de código

Calcado de `training`/`finance`.

**Backend**
- `api/db/migrations/0005_habits.sql` — tablas + índices (§2).
- `api/db/queries/habits.sql` — sqlc: `CreateHabit` (ON CONFLICT (user_id, lower(name)) WHERE archived_at IS NULL DO UPDATE SET name = habits.name RETURNING * — idempotente por nombre activo), `ListHabits` (activos, ORDER BY created_at DESC), `ListArchivedHabits`, `GetHabit` (id + user_id), `ArchiveHabit` (set archived_at, scoped, RETURNING), `DeleteHabit` (:execrows, scoped), `UpsertHabitLog` (ON CONFLICT (habit_id, day) DO NOTHING), `DeleteHabitLog` (habit_id + day), `ListLogsByHabitIDs` (`WHERE habit_id = ANY($1::uuid[]) ORDER BY day`).
- `api/internal/habits/types.go` — `Habit` (vista con calculados), `HabitInput`, `CheckInput`, `dateLayout`.
- `api/internal/habits/service.go` — `Service{q *store.Queries, pool *pgxpool.Pool}`; métodos `List(ctx, userID, archived) ([]Habit, error)`, `Create(ctx, userID, HabitInput) (*Habit, error)`, `SetCheck(ctx, userID, habitID, day, done) (*Habit, error)`, `Archive(ctx, userID, habitID) (*Habit, error)`, `Delete(ctx, userID, habitID) error`. Helper puro `computeStreaks(days, today)`. Evita N+1 con `ListLogsByHabitIDs` + agrupación en mapa. `GetHabit` traduce `pgx.ErrNoRows` → `(nil, nil)` para 404 en el handler.
- `api/internal/httpx/httpx.go` — labels nuevos en `fieldLabel`: `TargetDays`→"la meta de días", `Day`→"el día", `Done`→"el estado". (`Name` ya existe.)
- `api/internal/server/server.go` — `habitsSvc := habits.NewService(q, d.Pool)` + `r.Mount("/habits", habits.Routes(habitsSvc))` dentro del grupo `RequireAuth`.

**Frontend**
- `web/src/lib/habits.ts` — tipos `Habit`, `HabitInput`; funciones `listHabits(archived?)`, `createHabit(input)`, `checkHabit(id, day, done)`, `archiveHabit(id)`, `removeHabit(id)`; helpers `todayString()`, `yesterdayString()`.
- `web/src/routes/disciplina.tsx` — página `/disciplina`: lista de hábitos activos (cada uno con toggle de **hoy**, control para marcar **ayer** si quedó pendiente, racha actual, récord, barra de progreso al reto); formulario de alta (nombre + meta opcional); acceso a archivados; botones archivar/borrar.
- `web/src/routes/index.tsx` — enlace "Disciplina" en el home (junto a Check-in / Finanzas / Entrenamiento).

Ruta SPA `/disciplina`; recurso API `/api/v1/habits`.

## 7. Manejo de errores

- **Validación (400):** `name` vacío → "Falta el nombre"; `target_days < 1` → label "la meta de días"; `day` fuera de {hoy, ayer} → 400 con mensaje claro. Vía `httpx.DecodeAndValidate` + chequeo explícito de la ventana en el handler.
- **Auth (401):** todo bajo `RequireAuth`; `auth.UserIDFromContext` ausente → 401.
- **No encontrado (404):** `GetHabit`/`ArchiveHabit`/`DeleteHabit` filtran por `user_id`; cero filas → 404.
- **Servidor (500):** errores de DB inesperados vía `httpx.WriteErr`.
- Las mutaciones de un solo statement no necesitan transacción explícita.

## 8. Testing

**Backend**
- **Store** (`internal/store/habits_test.go`): crear hábito; `UpsertHabitLog` idempotente (mismo día no duplica); `ListLogsByHabitIDs` agrupa por hábito; archivar excluye de `ListHabits` y aparece en `ListArchivedHabits`; delete cascade borra logs; único parcial permite recrear el nombre tras archivar.
- **Servicio — `computeStreaks` (tabla de casos puros):** corrida consecutiva hasta hoy; racha viva anclada en ayer (hoy pendiente); racha cortada (ni hoy ni ayer) = 0; récord > actual; historial vacío = 0; un solo día = 1.
- **Handler** (`internal/habits/handler_test.go`): crear + listar con campos calculados; check hoy y check ayer OK; rechaza marcar anteayer (400); `done:false` baja la racha; archivar saca de activos y aparece en `?archived=true`; aislamiento entre usuarios (404); 401 sin token.

**Frontend**
- **Lib** (`habits.test.ts`): cada función pega a la URL/método correctos, manda Bearer con token, arma el body de check (`day`, `done`).
- **Página** (`disciplina.test.tsx`): muestra hábitos con su racha; el toggle de hoy dispara `POST /api/v1/habits/{id}/check` con `done:true`; crear hábito dispara `POST /api/v1/habits`; archivar dispara `POST /api/v1/habits/{id}/archive`.

## 9. Criterios de aceptación

1. Tablas `habits` / `habit_logs` creadas; todas las queries scoped por `user_id`.
2. Crear hábito (con o sin meta) y verlo en la lista activa.
3. Marcar hoy suma a la racha; el récord se conserva ≥ racha actual.
4. Marcar ayer dentro de la ventana de gracia; marcar anteayer da 400.
5. Desmarcar (`done:false`) recalcula la racha hacia abajo.
6. Archivar mueve el hábito al historial (`?archived=true`); borrar lo elimina con sus logs.
7. Aislamiento entre usuarios (404 sobre hábitos ajenos); 401 sin sesión.
8. `make check`, tests de frontend y `npm run build` en verde; smoke e2e dockerizado OK.
