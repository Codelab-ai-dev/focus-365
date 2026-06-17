# Plan 26 — Notas por serie (entrenamiento, slice C1) — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 0. Contexto: descomposición del módulo de entrenamiento

Tercer paso de la expansión de entrenamiento. Sub-proyectos:
- A — Perfil de fitness (hecho, R24).
- B — Agente de sugerencias (hecho, R25).
- **C — Notas por ejercicio + ajustes del agente**, que se parte en:
  - **C1 — Notas por serie** (este spec): registrar una nota por serie y verla.
  - **C2 — Ajustes del agente**: el agente lee los entrenos + notas y propone
    ajustes. Depende de C1.
- D — Evolución / progreso.

Este spec cubre **solo C1**.

## 1. Visión y alcance

Cada **serie** del registro de entreno (cada fila ejercicio + reps + peso) gana
una **nota opcional** ("leí pesado", "molestia rodilla", "fácil, subir peso"). Se
guarda con la serie y se muestra en el historial debajo de esa serie. Como
extensión natural, el **Entrenador IA (slice B)** empieza a incluir esas notas en
su contexto, así sus sugerencias ya las consideran.

**Decisiones (brainstorming):**
- **Nota por fila/serie** (cada fila del form = una serie de un ejercicio), no una
  nota por ejercicio agrupado.
- En el form, la nota va en una **segunda línea** debajo de cada serie.

**Fuera de alcance (C2):** que el agente genere **ajustes** explícitos leyendo
estas notas; eso es C2. Acá solo se registran, se ven y se suman al contexto del
agente de B.

## 2. Modelo de datos (migración `0022_workout_set_note.sql`)

```sql
-- +goose Up
ALTER TABLE workout_sets ADD COLUMN note TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE workout_sets DROP COLUMN note;
```

- Extiende `workout_sets` (sin tabla nueva). `NOT NULL DEFAULT ''` → las series
  existentes quedan con nota vacía.

## 3. Backend (paquete `training`)

### Queries (`api/db/queries/training.sql`)
- `CreateWorkoutSet` gana la columna `note` en el INSERT.
- `ListSetsByWorkout` y `ListSetsByWorkoutIDs` agregan `ws.note` al SELECT (para
  el detalle del workout y para el contexto del agente).

### Servicio y handler
- `SetInput` gana `Note string`; `setReq` gana `Note string` con validación de
  largo (máx 200 runes; se valida en el handler tras `TrimSpace`, → 400 si
  excede). El `WorkoutSet` (vista, `types.go`) gana `Note string`
  (`json:"note"`). `CreateWorkout` pasa `note` al crear cada serie.
- `buildSuggestionContext` (slice B, `suggestion.go`): al renderizar cada serie
  del historial, si tiene nota la agrega entre paréntesis
  (`· Sentadilla 8 reps @ 80.0 kg (leí pesado)`), para que el agente la
  considere. (El row de `ListSetsByWorkoutIDs` ahora trae `Note`.)

### Vista (JSON)
`WorkoutSet { exercise, reps, weight_grams, note }` — se suma `note` (string) a
la vista existente.

## 4. Frontend (`web/src/routes/entrenamiento.tsx`)

- `SetRow` (tipo local) gana `note: string`; `emptyRow()` lo inicializa en `""`.
- En el form, cada serie pasa a **dos líneas**: la línea 1 actual (ejercicio /
  reps / kg) + una línea 2 con un `<Input>` de **nota** opcional, ancho,
  `aria-label="Nota serie N"`, placeholder "nota de la serie (opcional)".
- Al guardar, cada serie envía `note: row.note.trim()`.
- En el **historial**, debajo de cada `<li>` de serie, si `s.note` no está vacío,
  se muestra en una línea chica (`text-xs text-muted`).
- `lib/training.ts`: el tipo `SetInput`/`WorkoutSet` (o equivalente) gana `note`.

## 5. Manejo de errores

- Nota de serie > 200 runes → `400`. Nota vacía → se guarda `''` y no se muestra.
- El resto del flujo de registro/validación de workouts no cambia.

## 6. Testing

- **Backend (store):** crear una serie con nota; `ListSetsByWorkout(IDs)` la
  devuelve.
- **Backend (handler):** crear un workout con notas por serie → el `GET
  /training/workouts` (o el detalle) las incluye en cada serie; nota > 200 runes
  → 400.
- **Frontend:** el input de nota por serie está y se envía; el historial muestra
  la nota de una serie mockeada.
- **E2E producción:** registrar un workout con una nota por serie y verificar que
  vuelve en el listado.

## 7. Criterios de aceptación

- Al registrar un entreno, cada serie acepta una nota opcional; se guarda y se ve
  en el historial.
- Las notas se incluyen en el contexto del Entrenador IA (B).
- Nota demasiado larga → 400.
- Suites en verde; smoke de producción OK.
