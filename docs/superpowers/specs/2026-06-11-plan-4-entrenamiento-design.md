# Plan 4 — Entrenamiento · Diseño

**Fecha:** 2026-06-11
**Estado:** Aprobado (diseño)
**Autor:** Gustavo (con Claude)
**Depende de:** Plan 1 (Cimientos + Auth), Plan 2 (Check-in) y Plan 3 (Finanzas) — ya integrados en `master`.

## 1. Objetivo

Tercer módulo de dominio de Focus 365: el **registro de entrenamiento**. El
usuario captura cada **sesión** de gym con su lista de **ejercicios** y, por
cada ejercicio, el **detalle de cada serie** (reps + peso). Los ejercicios salen
de un **catálogo propio** que el usuario va construyendo, de modo que los nombres
queden consistentes y, en un plan futuro, se pueda graficar la **progresión** por
ejercicio.

Es el módulo "Entrenamiento" del diseño general (`2026-06-09-focus-365-design.md`,
§1). Reutiliza el patrón ya establecido (migración → sqlc → handlers protegidos →
lib + página React con TanStack Query) que usaron Check-in y Finanzas.

## 2. Alcance

**Incluye:**
- **Catálogo de ejercicios** por usuario (`exercises`): crear y listar.
- **Sesiones de entrenamiento** (`workouts`) con tipo libre y nota.
- **Detalle por serie** (`workout_sets`): cada serie con reps y peso, ligada a un
  ejercicio del catálogo.
- API REST bajo `/api/v1/training`, protegida: catálogo (listar/crear), crear
  sesión completa (con sus series en un solo body), listar historial por rango de
  fechas, ver y borrar sesión.
- Ruta dedicada `/entrenamiento` en la SPA: formulario de captura (fecha, tipo,
  ejercicios con autocompletado del catálogo + series con reps/peso, nota) +
  historial de sesiones agrupadas por fecha.
- Enlace a entrenamiento desde el home.
- Tests backend (store + handlers) y frontend (lib + página).

**Fuera de alcance (planes posteriores):**
- Vista/gráfica de **progresión** por ejercicio (se diseñará aparte).
- **Plantillas de rutina** reutilizables (crear una sesión a partir de una rutina).
- **Cardio** con duración/distancia dedicadas (por ahora el tipo es libre y las
  series son reps/peso; el cardio estructurado es un plan futuro).
- Edición de sesiones (solo crear y borrar; corregir = borrar + recrear).
- Tarjeta de "Entreno de hoy" en el dashboard real (Plan posterior).
- Insights de IA sobre el entrenamiento (Plan posterior).

## 3. Decisiones de diseño (confirmadas)

| Decisión | Elección |
|----------|----------|
| Nivel de registro | **Estructurado por ejercicio** (sesión → ejercicios → series) |
| Nombres de ejercicio | **Catálogo propio** por usuario (no texto libre) |
| Detalle de series | **Por serie** (cada serie tiene sus reps y peso) |
| Tipo de actividad | **Texto libre** (sin enum: "Fuerza", "Pierna", "Cardio"…) |
| Alcance de la rebanada | **Captura + historial** (progresión = plan futuro) |
| Modelo de datos | **Plano (3 tablas)**: `exercises` + `workouts` + `workout_sets` |
| Peso | **Entero en gramos** (igual disciplina que centavos en finanzas); se muestra en kg |
| Reps y peso | **Opcionales** (nullable) para no forzar capturas que no apliquen |
| Edición | **No** (solo crear y borrar) |

## 4. Modelo de datos

Migración goose nueva (`api/db/migrations/0004_training.sql`):

```sql
-- +goose Up
CREATE TABLE exercises (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Único por usuario sin distinguir mayúsculas: "Sentadilla" == "sentadilla".
CREATE UNIQUE INDEX uq_exercises_user_name
    ON exercises (user_id, lower(name));

CREATE TABLE workouts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    type       TEXT NOT NULL DEFAULT '',   -- libre: "Fuerza", "Pierna"…
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workouts_user_date
    ON workouts (user_id, date DESC, created_at DESC);

CREATE TABLE workout_sets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workout_id  UUID NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    exercise_id UUID NOT NULL REFERENCES exercises(id),
    position    INT NOT NULL,            -- orden de la serie dentro de la sesión
    reps        INT,                     -- opcional
    weight_grams INT,                    -- opcional; peso en gramos (80kg = 80000)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workout_sets_workout ON workout_sets (workout_id, position);
CREATE INDEX idx_workout_sets_exercise ON workout_sets (exercise_id);

-- +goose Down
DROP TABLE workout_sets;
DROP TABLE workouts;
DROP TABLE exercises;
```

- **`exercises`** es el catálogo: el índice único parcial por `(user_id, lower(name))`
  evita duplicados por mayúsculas/minúsculas y permite "crear si no existe" al
  capturar.
- **`workouts`** es la sesión: `date` es el día del entreno (fecha local del
  cliente, como en check-in/finanzas); `type` y `note` son texto libre.
- **`workout_sets`**: una fila **por serie**. Cada fila apunta a su `workout` y a un
  `exercise` del catálogo. `position` ordena las series dentro de la sesión y
  permite reconstruir el orden de captura. `reps` y `weight_grams` son opcionales.
- `weight_grams INT` (entero) evita los líos de coma flotante; soporta incrementos
  de 0.5 kg (= 500 g). El frontend convierte kg↔gramos.
- El borrado de una sesión arrastra sus series (`ON DELETE CASCADE`). El catálogo
  **no** se borra al borrar sesiones (el `exercise_id` no tiene cascade); un
  ejercicio del catálogo persiste para el historial y la progresión futura.
- El índice `idx_workout_sets_exercise` deja lista la consulta de progresión por
  ejercicio (plan futuro) sin migración adicional.

## 5. API

Base `/api/v1/training`, montada con `RequireAuth`. El `user_id` sale del contexto
(middleware de Plan 1); **nunca** del body. Errores con el formato `{"error":"..."}`
de Plan 1. El peso viaja en **gramos**.

### `GET /api/v1/training/exercises` — catálogo
- Lista los ejercicios del usuario, **ascendente por nombre**.
- Respuesta `200` con un array (posiblemente vacío).

### `POST /api/v1/training/exercises` — crear ejercicio
Body:
```json
{ "name": "Sentadilla" }
```
- Validación: `name` requerido (no vacío tras `trim`).
- Idempotente: si ya existe (mismo `lower(name)`), devuelve el existente.
- Respuesta `201` con el ejercicio (o `200` con el existente).

### `POST /api/v1/training/workouts` — crear sesión completa
Body (la sesión y todas sus series en una sola llamada):
```json
{
  "date": "2026-06-11",
  "type": "Fuerza",
  "note": "buen pump",
  "sets": [
    { "exercise": "Sentadilla", "reps": 8, "weight_grams": 80000 },
    { "exercise": "Sentadilla", "reps": 8, "weight_grams": 80000 },
    { "exercise": "Sentadilla", "reps": 6, "weight_grams": 80000 },
    { "exercise": "Press banca", "reps": 10, "weight_grams": 60000 }
  ]
}
```
- Validación: `date` requerida (`YYYY-MM-DD`); `sets` no vacío; cada set requiere
  `exercise` (nombre no vacío); `reps`/`weight_grams` opcionales y, si vienen, ≥ 0.
- Por cada `set`, el servidor resuelve/crea el ejercicio del catálogo por nombre
  (mismo `lower(name)`), asigna `position` según el orden recibido, y crea el
  `workout` + sus `workout_sets` en **una transacción** (todo o nada).
- `type`/`note` opcionales (default `''`).
- Respuesta `201` con la sesión completa (ver forma en `GET /workouts/{id}`).

### `GET /api/v1/training/workouts?from=YYYY-MM-DD&to=YYYY-MM-DD`
- Historial de sesiones del usuario en el rango (ambos opcionales; default: últimas
  sesiones), **descendente por fecha**. Cada sesión trae sus series con el **nombre**
  del ejercicio resuelto.
- Respuesta `200` con un array.

### `GET /api/v1/training/workouts/{id}` — detalle
- Devuelve una sesión **si pertenece al usuario**, con sus series ordenadas por
  `position` y el nombre de cada ejercicio:
```json
{
  "id": "…", "date": "2026-06-11", "type": "Fuerza", "note": "buen pump",
  "sets": [
    { "exercise": "Sentadilla", "reps": 8, "weight_grams": 80000 },
    { "exercise": "Sentadilla", "reps": 8, "weight_grams": 80000 }
  ]
}
```
- Respuesta `200`; `404` si no existe o no es del usuario.

### `DELETE /api/v1/training/workouts/{id}`
- Borra la sesión `{id}` **si pertenece al usuario** (las series caen en cascada).
- Respuesta `204` si borró; `404` si no existe o no es del usuario.

**Queries sqlc** (`api/db/queries/training.sql`):
- `ListExercises` — `WHERE user_id=$1 ORDER BY name ASC`.
- `UpsertExercise` — `INSERT ... ON CONFLICT (user_id, lower(name)) DO UPDATE SET name=name RETURNING *` (devuelve el existente o el nuevo; resuelve catálogo on-the-fly).
- `CreateWorkout` — `INSERT INTO workouts ... RETURNING *`.
- `CreateWorkoutSet` — `INSERT INTO workout_sets ... RETURNING *`.
- `ListWorkouts` — sesiones del usuario por rango de fechas, `ORDER BY date DESC, created_at DESC`.
- `GetWorkout` — una sesión `WHERE id=$1 AND user_id=$2`.
- `ListSetsByWorkout` — series de una sesión con `JOIN exercises` para el nombre, `ORDER BY position ASC`.
- `ListSetsByWorkoutIDs` — series de varias sesiones (para el historial) con el nombre del ejercicio, evitando N+1.
- `DeleteWorkout` — `DELETE ... WHERE id=$1 AND user_id=$2` (filtra por dueño).

## 6. Estructura de código

**Backend dominio** (paquete nuevo `api/internal/training`, espejo de `finance`):
- `service.go` — `Service{ q *store.Queries, pool *pgxpool.Pool }`; métodos
  `ListExercises(ctx, userID)`, `CreateExercise(ctx, userID, name)`,
  `CreateWorkout(ctx, userID, in)`, `ListWorkouts(ctx, userID, from, to)`,
  `GetWorkout(ctx, userID, id)`, `DeleteWorkout(ctx, userID, id)`. `CreateWorkout`
  abre una **transacción** (`pool.Begin`), resuelve/crea ejercicios, inserta la
  sesión y sus series, y traduce entre tipos del dominio y `store`.
- `types.go` — structs del dominio: `Workout`, `WorkoutSet` (json con `exercise`
  como nombre, `reps`/`weight_grams` opcionales), `WorkoutInput`, `SetInput`,
  `Exercise`.
- `handler.go` — `Routes(svc *Service) http.Handler` (chi); usa `httpx` (de Plan 2)
  para decodificar/validar/responder y `auth.UserIDFromContext`.
- Montaje en `internal/server/server.go`:
  `r.Mount("/training", training.Routes(...))` bajo `/api/v1`, dentro del grupo
  envuelto con `auth.RequireAuth(tokenManager)`.

> Nota: a diferencia de `finance`, el servicio de training necesita el `*pgxpool.Pool`
> (no solo `*store.Queries`) para abrir la transacción de `CreateWorkout`. `store.New`
> acepta cualquier `DBTX`, así que dentro de la transacción se usa `store.New(tx)`.

**Frontend:**
- `web/src/lib/training.ts` — tipos `Exercise`, `Workout`, `WorkoutSet`,
  `WorkoutInput`, `SetInput`; funciones `listExercises()`, `createExercise(name)`,
  `createWorkout(input)`, `listWorkouts(from?, to?)`, `getWorkout(id)`,
  `removeWorkout(id)` sobre `apiFetch`. Helpers `kgToGrams`/`gramsToKg` y
  `todayString()` (reusar el de finanzas si conviene, o duplicar el helper local).
- `web/src/routes/entrenamiento.tsx` — ruta protegida. `useQuery` para el catálogo
  de ejercicios y para el historial de sesiones; `useMutation` para crear sesión y
  para borrar (invalidan las queries al éxito). Formulario: fecha, tipo (texto),
  filas de series (selector/autocompletado de ejercicio desde el catálogo + reps +
  peso en kg, con botón "agregar serie"), nota, botón Guardar. Historial de sesiones
  agrupadas por fecha, mostrando ejercicios y sus series, con botón borrar por
  sesión. Estilo Warm Discipline.
- Enlace desde `web/src/routes/index.tsx` al `/entrenamiento`.

## 7. Flujo de datos

1. Usuario abre `/entrenamiento`. `useQuery` pide el catálogo de ejercicios y el
   historial reciente con `Authorization: Bearer`.
2. Captura una sesión: elige fecha y tipo, agrega filas de series (cada una con su
   ejercicio del catálogo —o uno nuevo—, reps y peso en kg), nota, y Guarda.
   `useMutation` → `POST .../workouts` con el peso convertido a **gramos**. El
   servidor crea ejercicios faltantes, inserta sesión + series en una transacción.
   Al éxito, invalida catálogo + historial → la UI refleja el cambio.
3. Al borrar una sesión: `useMutation` → `DELETE .../workouts/{id}`; invalida el
   historial. Las series caen en cascada; el catálogo no se toca.
4. nginx (mismo origen) proxya `/api` → Go; `RequireAuth` valida el token e inyecta
   `user_id`; los handlers operan scoped por ese `user_id`.

## 8. Manejo de errores

- `400` validación (mensajes por campo, reusando `httpx` de Plan 2): nombre de
  ejercicio vacío, `date` faltante/mal formada, `sets` vacío, set sin `exercise`,
  reps/peso negativos.
- `401` sin token o token inválido (lo da `RequireAuth`).
- `404` al ver/borrar una sesión inexistente o de otro usuario.
- `500` errores de BD → `{"error":"error interno"}`, sin filtrar detalles; la
  transacción de `CreateWorkout` hace **rollback** ante cualquier fallo (no quedan
  sesiones a medias).
- Frontend: las mutaciones capturan `ApiError` y muestran `err.message` cerca del
  formulario; estados de carga deshabilitan los botones.

## 9. Testing

**Backend (Go, requiere `TEST_DATABASE_URL`):**
- `store`: `UpsertExercise` es idempotente por `(user_id, lower(name))`; crear
  sesión + series persiste; `ListWorkouts` filtra por rango y ordena desc;
  `ListSetsByWorkout` ordena por `position` y resuelve el nombre del ejercicio;
  `DeleteWorkout` respeta dueño y arrastra las series (cascade).
- `handler`: `POST /exercises` crea/idempotente; `POST /workouts` crea sesión con
  sus series (201 + cuerpo con nombres resueltos) y crea ejercicios on-the-fly;
  `GET /workouts` lista por rango; `GET /workouts/{id}` 200 y 404; `DELETE` 204 y
  luego 404; validación (nombre/fecha/sets) → 400; sin token → 401; aislamiento por
  usuario (user B no ve ni borra lo de A); rollback: un set con datos inválidos no
  deja la sesión a medias.

**Frontend (Vitest + Testing Library):**
- `lib/training`: `listExercises`/`createExercise`/`createWorkout`/`listWorkouts`/
  `getWorkout`/`removeWorkout` llaman a la ruta y método correctos (fetch mockeado),
  incluyen Bearer cuando hay token; `kgToGrams`/`gramsToKg` convierten bien
  (incl. 0.5 kg).
- `entrenamiento` page: renderiza el catálogo y el historial; al guardar una sesión
  dispara el POST con el peso en gramos; al borrar dispara el DELETE; agrupa las
  series por ejercicio en el historial.

## 10. Criterios de aceptación

- `docker compose up` + migraciones: existen las tablas `exercises`, `workouts`,
  `workout_sets`.
- Logueado, en `/entrenamiento`: crear una sesión con varias series responde 201 y
  persiste; recargar la muestra en el historial; borrarla la quita.
- Capturar una serie con un ejercicio nuevo lo agrega al catálogo y queda disponible
  para la siguiente captura.
- Capturar dos veces el mismo nombre de ejercicio (distinta capitalización) no
  duplica el catálogo.
- El peso se guarda en gramos y se muestra en kg sin errores de redondeo.
- Un usuario nunca ve ni borra las sesiones de otro.
- Sin sesión, `/entrenamiento` redirige a `/login`; la API responde 401.
- Un fallo a mitad de la captura no deja sesiones a medias (transacción con rollback).
- `go build/vet/test` (con `make test`, `-p 1`) y `tsc/vitest` en verde.
