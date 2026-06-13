# Plan 17 — Check-in diario de Capitanes de Dios — Diseño

**Fecha:** 2026-06-13
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Rediseñar el check-in diario para alinearlo al **Daily Check-in de Capitanes de
Dios** (el método de restauración masculina del usuario, construido sobre 4
dimensiones). El check-in actual es genérico (ánimo / energía / **disciplina** /
nota) y no tiene nada que ver con el documento. El nuevo lleva:

- **Mood** y **Energy** (1-10) — se conservan.
- **Reflexión 4D:** una línea de texto por dimensión — **Espiritual, Emocional,
  Física, Financiera** (el corazón del check-in).
- **Win del día** y **¿Qué evité hoy?** (texto).
- **Mañana me comprometo a:** lista de compromisos (texto).

**Decisiones (brainstorming con mockup del formulario, aprobado):**
- Se **elimina `discipline`** (no existe en el documento).
- La acción IA `registrar_checkin` queda como **solo métricas** (mood/energy):
  un atajo rápido de los números por chat; las reflexiones/win/evité/compromisos
  los escribe el usuario en el formulario.
- El dashboard resume: **hecho + mood/energy + win**.
- Compromisos = **solo texto** por ahora (rastrearlos al día siguiente es una
  rebanada futura).

**Fuera de alcance:** compromisos rastreables día a día; alinear el vocabulario
de dimensiones de *Metas* (checkin/finanzas/entrenamiento/mente/general) a las
4D de Capitanes (es otro feature).

## 2. Modelo de datos (migración `0013_checkin_capitanes.sql`)

`check_ins` **elimina** `discipline` y `note`; **agrega**:

```sql
-- +goose Up
ALTER TABLE check_ins
    DROP COLUMN discipline,
    DROP COLUMN note,
    ADD COLUMN dim_espiritual TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_emocional  TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_fisica     TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_financiera TEXT NOT NULL DEFAULT '',
    ADD COLUMN win            TEXT NOT NULL DEFAULT '',
    ADD COLUMN avoided        TEXT NOT NULL DEFAULT '',
    ADD COLUMN commitments    JSONB NOT NULL DEFAULT '[]';
-- +goose Down
ALTER TABLE check_ins
    DROP COLUMN dim_espiritual, DROP COLUMN dim_emocional,
    DROP COLUMN dim_fisica, DROP COLUMN dim_financiera,
    DROP COLUMN win, DROP COLUMN avoided, DROP COLUMN commitments,
    ADD COLUMN discipline INT NOT NULL DEFAULT 0,
    ADD COLUMN note TEXT NOT NULL DEFAULT '';
```

`mood`/`energy` se conservan; los check-ins existentes mantienen sus números y
los campos nuevos quedan vacíos. Las queries de `check_ins.sql`
(`UpsertCheckIn`, `GetCheckInByDate`, `ListCheckIns`) se actualizan al nuevo
conjunto de columnas; sqlc regenera.

## 3. Backend — servicio de check-in (`checkin`)

`Input` y `CheckIn` cambian:

```go
type Input struct {
	Date          time.Time
	Mood, Energy  int
	Espiritual, Emocional, Fisica, Financiera string
	Win, Avoided  string
	Commitments   []string
}
type CheckIn struct {
	ID, Date string
	Mood, Energy int
	Espiritual, Emocional, Fisica, Financiera string
	Win, Avoided string
	Commitments  []string
	CreatedAt, UpdatedAt time.Time
}
```

- **`Upsert`** reemplaza el registro completo del día (lo usa el formulario).
- **`UpsertMetrics(userID, date, mood, energy)`** nuevo: upsert **parcial** —
  inserta/actualiza solo mood/energy preservando reflexiones/win/avoided/
  commitments si el registro ya existe (lo usa la acción IA). Vía una query
  `UpsertCheckInMetrics` con `ON CONFLICT (user_id, date) DO UPDATE SET
  mood=..., energy=..., updated_at=now()` que **no toca** las demás columnas.
- **Validación** (handler): mood/energy 1-10 requeridos; textos y compromisos
  opcionales (se puede guardar solo los números, como hoy). `commitments`
  acepta un array de strings (se filtran vacíos/trim).

## 4. API

`POST /checkins` (upsert completo, body con todos los campos), `GET
/checkins/today`, `GET /checkins` — sin cambios de ruta, solo de forma del
body/response. El nuevo `commitments` viaja como array JSON.

## 5. IA — solo métricas

- **`checkinPayload`** pasa a `{mood, energy}` (se elimina discipline y note).
- **Tool `registrar_checkin`**: parámetros `mood`, `energy` (1-10) requeridos.
  Descripción ajustada: registra solo tus métricas del día; las reflexiones se
  escriben en el formulario.
- **Ejecutor:** llama `checkin.UpsertMetrics(userID, today, mood, energy)`
  (upsert parcial, no pisa reflexiones). El **`result`** del execute guarda
  `{prev_mood, prev_energy, existed, date}` (snapshot de los dos números y si el
  registro existía).
- **Undo:** si `existed`, restaura mood/energy previos vía `UpsertMetrics`; si
  no existía (la acción creó el registro) y sigue sin reflexiones, borra el día
  (`checkin.Delete`, ya existe); si el usuario ya le agregó reflexiones, solo
  revierte los números (best-effort, no borra contenido del usuario).
- **`actionSummary` checkin:** «Propongo registrar tus métricas de hoy: ánimo N,
  energía M.»
- **Contexto del chat (`chatcontext`):** los check-ins recientes ahora incluyen
  las 4 reflexiones + win + avoided (texto), dándole a la IA contexto rico de la
  vida del usuario. (Se mantiene el límite de 14 recientes.)
- **Compat:** acciones de check-in **previas a la migración** tienen `result`
  con discipline/note; su undo es best-effort (si el payload viejo no calza, no
  rompe — app personal). El `parseActionPayload("checkin")` con el payload
  nuevo (solo mood/energy) rechaza payloads viejos con discipline por
  `DisallowUnknownFields`, lo cual solo afecta el camino de proponer (no hay
  propuestas viejas pendientes en producción).

## 6. Dashboard

- `Snapshot.checkin`: de `{present, mood, energy, discipline}` a `{present,
  mood, energy, win}`.
- **MoodCard:** sin cambios (barras mood/energy).
- **CheckinCard:** ya no muestra «disciplina N» → «Hecho ✓ · <win>» si hay win,
  «Hecho ✓» si no, «Pendiente» si no hay check-in.

## 7. Frontend — formulario `/check-in`

Reescritura con la estructura aprobada (estilo neo-brutalista):
- **¿Cómo estoy?** mood/energy (steppers/sliders, como hoy).
- **¿Qué hice hoy en mis 4 dimensiones?** 4 inputs con chip de color:
  E (espiritual, morado), Em (emocional, rojo), F (física, verde), Fi
  (financiera, amarillo).
- **🏆 Win del día** y **🚫 ¿Qué evité hoy?** (inputs de texto).
- **Mañana me comprometo a:** lista editable con «+ agregar compromiso» y quitar.
- **Precarga:** al montar, `getToday` rellena el formulario con el check-in de
  hoy si existe (incluidos los compromisos). Guardar hace el upsert completo.
- La lib `web/src/lib/checkins.ts` (o equivalente) actualiza tipos y el POST.

## 8. Testing

- **Migración:** drop discipline/note, add columnas; mood/energy preservados de
  un check-in sembrado.
- **Servicio:** `Upsert` completo round-trip (incluidos commitments JSONB);
  `UpsertMetrics` parcial preserva reflexiones existentes y crea mínimo si no hay.
- **Validación:** mood/energy fuera de rango → 400; textos/compromisos opcionales.
- **IA:** ejecutor de `registrar_checkin` llama UpsertMetrics; result guarda
  prev/existed; undo con check-in previo restaura, sin previo borra, con
  reflexiones agregadas solo revierte números.
- **chatcontext:** incluye las 4 reflexiones.
- **Dashboard:** snapshot con win; CheckinCard muestra win/Pendiente.
- **Frontend:** form guarda todos los campos; precarga; agregar/quitar
  compromiso; los tests existentes del check-in se adaptan a la forma nueva.
- **E2E producción:** guardar un check-in completo (con 4D + compromisos) →
  `GET /checkins/today` lo devuelve; registrar mood/energy por chat → confirmar
  → no pisa reflexiones previas.

## 9. Criterios de aceptación

- El formulario de `/check-in` captura mood, energy, las 4 reflexiones, win,
  qué evité y los compromisos; al recargar los muestra.
- La IA puede registrar solo mood/energy por chat sin borrar lo que escribiste.
- El dashboard ya no menciona disciplina; muestra hecho + mood/energy + win.
- Las 7 acciones del chat y el deshacer siguen funcionando; suites en verde;
  smoke de producción del check-in completo OK.
