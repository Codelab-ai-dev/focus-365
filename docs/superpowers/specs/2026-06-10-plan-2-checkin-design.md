# Plan 2 — Check-in diario · Diseño

**Fecha:** 2026-06-10
**Estado:** Aprobado (diseño)
**Autor:** Gustavo (con Claude)
**Depende de:** Plan 1 (Cimientos + Auth) — mergeado en `master`.

## 1. Objetivo

Primer módulo de dominio de Focus 365: el **check-in diario**. El usuario
registra cada día su **ánimo**, **energía** y **disciplina** (escala 1-10) más
una **nota** opcional, y revisa un **historial** de sus check-ins recientes.

Es la rebanada 3 del diseño general (`2026-06-09-focus-365-design.md`, §7) y la
primera que escribe datos de dominio scoped por `user_id`, estableciendo el
patrón (migración → sqlc → handlers protegidos → lib + página React con TanStack
Query) que reutilizarán los módulos siguientes (finanzas, hábitos, metas).

## 2. Alcance

**Incluye:**
- Tabla `check_ins` con upsert por día.
- API REST bajo `/api/v1/checkins`, protegida y scoped por `user_id`.
- Ruta dedicada `/check-in` en la SPA: formulario con 3 sliders + nota +
  historial de solo lectura.
- Enlace al check-in desde el home.
- Tests backend (store + handlers) y frontend (lib + página).

**Fuera de alcance (planes posteriores):**
- Tarjeta de check-in en el dashboard real (Plan 6).
- Gráficos de tendencia.
- Edición de check-ins de días pasados (el historial es de solo lectura).
- Insights de IA sobre el ánimo (Plan 7).

## 3. Decisiones de diseño (confirmadas)

| Decisión | Elección |
|----------|----------|
| Alcance | Check-in de hoy (guardar/editar) **+ historial** de solo lectura |
| Input de 1-10 | **Sliders** con el valor visible |
| Ubicación | **Ruta dedicada `/check-in`**, enlazada desde el home |
| Edición | **Solo el de hoy** (upsert por día); el historial no se edita |
| Fuente de "hoy" | El **frontend** calcula su fecha local (`YYYY-MM-DD`) y la envía; evita supuestos de timezone en el servidor |

## 4. Modelo de datos

Migración goose nueva (`api/db/migrations/0002_check_ins.sql`):

```sql
-- +goose Up
CREATE TABLE check_ins (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date        DATE NOT NULL,
    mood        INT  NOT NULL CHECK (mood       BETWEEN 1 AND 10),
    energy      INT  NOT NULL CHECK (energy     BETWEEN 1 AND 10),
    discipline  INT  NOT NULL CHECK (discipline BETWEEN 1 AND 10),
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, date)
);
CREATE INDEX idx_check_ins_user_date ON check_ins (user_id, date DESC);

-- +goose Down
DROP TABLE check_ins;
```

- `note NOT NULL DEFAULT ''` evita punteros nulos en Go (sin `pgtype`/`sql.Null`).
- El `CHECK` en BD es defensa en profundidad; la validación primaria está en el
  handler (mensajes claros).
- Índice `(user_id, date DESC)` sirve al historial y al lookup por día.

## 5. API

Base `/api/v1/checkins`, montada con `RequireAuth`. El `user_id` sale del
contexto (middleware de Plan 1); **nunca** del body. Errores con el mismo formato
`{"error": "..."}` de Plan 1.

### `POST /api/v1/checkins` — upsert del día
Body:
```json
{ "date": "2026-06-10", "mood": 7, "energy": 6, "discipline": 8, "note": "buen día" }
```
- Validación: `date` requerida (formato `YYYY-MM-DD`); `mood`/`energy`/
  `discipline` requeridos y 1-10; `note` opcional.
- Upsert por `(user_id, date)`: si ya existe ese día, actualiza valores +
  `updated_at`.
- Respuesta `200` con el check-in completo (`id, date, mood, energy, discipline,
  note, created_at, updated_at`).

### `GET /api/v1/checkins/today?date=YYYY-MM-DD`
- Devuelve el check-in de ese día para el usuario, o `null` (HTTP `200` con
  cuerpo `null`) si no existe.
- `date` requerida como query param.

### `GET /api/v1/checkins?limit=30`
- Historial del usuario, **descendente por fecha**.
- `limit` opcional (default 30, máx 100).
- Respuesta `200` con un array (posiblemente vacío).

**Queries sqlc** (`api/db/queries/check_ins.sql`):
- `UpsertCheckIn` — `INSERT ... ON CONFLICT (user_id, date) DO UPDATE SET mood=, energy=, discipline=, note=, updated_at=now() RETURNING *`.
- `GetCheckInByDate` — `WHERE user_id=$1 AND date=$2`.
- `ListCheckIns` — `WHERE user_id=$1 ORDER BY date DESC LIMIT $2`.

## 6. Estructura de código

**Helpers HTTP compartidos** (paquete nuevo `api/internal/httpx`):
- Se extraen los helpers que hoy viven en `auth/handler.go` para que todos los
  módulos los reusen (DRY): `WriteJSON(w, status, v)`, `WriteErr(w, status, msg)`,
  `DecodeAndValidate(w, r, dst) bool` y `ValidationMessage(err) string` (con
  `fieldLabel`/`capitalize`).
- `auth` se refactoriza para usar `httpx` (sin cambiar su comportamiento ni sus
  tests); `fieldLabel` aprende las etiquetas de los campos de check-in.

**Backend dominio** (paquete nuevo `api/internal/checkin`, espejo de `auth`):
- `service.go` — `Service{ q *store.Queries }`; métodos `Upsert(ctx, userID, in)`,
  `Today(ctx, userID, date) (*CheckIn, error)`, `List(ctx, userID, limit)`.
  Traduce entre tipos del dominio y `store`.
- `handler.go` — `Routes(svc *Service) http.Handler` (chi); usa `httpx` para
  decodificar/validar/responder y `auth.UserIDFromContext` para el `user_id`.
- Montaje en `internal/server/server.go`: `r.Mount("/checkins", checkin.Routes(...))`
  bajo `/api/v1`, envuelto con `auth.RequireAuth(tokenManager)`.

**Frontend:**
- `web/src/lib/checkins.ts` — tipos `CheckIn`, `CheckInInput`; funciones
  `getToday(date)`, `list(limit)`, `upsert(input)` sobre `apiFetch`.
- `web/src/routes/check-in.tsx` — ruta protegida. `useQuery` para hoy + historial;
  `useMutation` para guardar (invalida ambas queries al éxito). 3 sliders 1-10 con
  valor visible, textarea de nota, botón Guardar; pre-rellena con el check-in de
  hoy. Sección de historial de solo lectura. Estilo Warm Discipline.
- Enlace desde `web/src/routes/index.tsx` al `/check-in`.

## 7. Flujo de datos

1. Usuario abre `/check-in`. `useQuery(["checkin","today",fecha])` → `GET
   /api/v1/checkins/today?date=...` con `Authorization: Bearer`.
2. Si hay check-in, los sliders se inicializan con sus valores; si no, default 5.
3. Al Guardar: `useMutation` → `POST /api/v1/checkins`. Al éxito, invalida
   `["checkin","today"]` y `["checkin","list"]` → la UI refleja el guardado.
4. nginx (mismo origen) proxya `/api` → Go; `RequireAuth` valida el token e
   inyecta `user_id`; el handler hace upsert/select scoped por ese `user_id`.

## 8. Manejo de errores

- `400` validación (mensajes por campo, reusando el helper de Plan 1): rango
  1-10, fecha faltante/mal formada.
- `401` sin token o token inválido (lo da `RequireAuth`).
- `500` errores de BD → `{"error":"error interno"}`, sin filtrar detalles.
- Frontend: la mutación captura `ApiError` y muestra `err.message` cerca del
  botón Guardar; estados de carga deshabilitan el botón.

## 9. Testing

**Backend (Go, requiere `TEST_DATABASE_URL`):**
- `store`: upsert inserta; upsert del mismo `(user,date)` actualiza (no duplica);
  get-by-date; list ordenado desc + respeta limit.
- `handler`: `POST` crea (200 + cuerpo); `POST` repetido mismo día actualiza;
  `GET /today` devuelve el día o `null`; `GET` lista; validación fuera de rango
  → 400; sin token → 401; aislamiento por usuario (user B no ve check-ins de A).

**Frontend (Vitest + Testing Library):**
- `lib/checkins`: `upsert`/`getToday`/`list` llaman a la ruta y método correctos
  (fetch mockeado), incluyen Bearer cuando hay token.
- `check-in` page: renderiza los 3 sliders y la nota; al guardar dispara el POST;
  muestra el historial; pre-rellena con el check-in de hoy.

## 10. Criterios de aceptación

- `docker compose up` + migraciones: la tabla `check_ins` existe.
- Logueado, en `/check-in`: guardar un check-in responde 200 y persiste; recargar
  muestra los valores guardados; volver a guardar el mismo día **actualiza** (no
  crea duplicado).
- El historial lista los check-ins recientes descendente.
- Un usuario nunca ve los check-ins de otro.
- Sin sesión, `/check-in` redirige a `/login`; la API responde 401.
- `go build/vet/test` y `tsc/vitest` en verde.
