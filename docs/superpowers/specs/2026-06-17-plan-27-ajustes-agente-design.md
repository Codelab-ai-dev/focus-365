# Plan 27 — Análisis / ajustes del agente (entrenamiento, slice C2) — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 0. Contexto: descomposición del módulo de entrenamiento

Cierre del agente de entrenamiento. Sub-proyectos:
- A — Perfil de fitness (hecho, R24).
- B — Agente de sugerencias / rutina (hecho, R25).
- C1 — Notas por serie (hecho, R26).
- **C2 — Análisis / ajustes del agente** (este spec): el agente lee los entrenos
  recientes + las notas y propone ajustes. Depende de A, B (infra IA) y C1 (notas).
- D — Evolución / progreso (pendiente).

## 1. Visión y alcance

En `/entrenamiento`, un panel propio **"Análisis del agente"** (debajo de
"Entrenador IA"): un **toggle de alcance** (Último entreno / Última semana) + un
botón **"Analizar"**. Al apretar, el agente lee el perfil + los entrenos del
alcance elegido **con las notas por serie** (C1) y devuelve **ajustes concretos**
en texto (progresión o descarga, qué cambiar por molestias/notas, técnica,
resumen). Se guarda el último análisis (sobrevive recarga); regenerar lo
reemplaza.

**Decisiones (brainstorming):**
- **Dos alcances** vía toggle: `last` (las **3 sesiones más recientes** — el
  agente se centra en la última y usa las anteriores para comparar progresión) y
  `week` (entrenos de los últimos 7 días).
- **Panel propio** "Análisis del agente", separado del de sugerencias (B).
- **On-demand** con botón; **se guarda el último** (1 por usuario, upsert).
- **Texto libre** (como B).

Es el espejo de B (slice R25) con un prompt de ajustes y un alcance configurable.

**Fuera de alcance:** disparo automático (es on-demand); D (evolución/progreso);
cargar los ajustes al registro de entreno; historial de análisis (solo el último).

## 2. Modelo de datos (migración `0023_training_adjustments.sql`)

```sql
-- +goose Up
CREATE TABLE training_adjustments (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    scope      TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE training_adjustments;
```

- **1 fila por usuario** (PK = user_id), espejo de `training_suggestions`.
  `scope` guarda el alcance del último análisis (`last` | `week`).

## 3. Backend (paquete `training`, reusando lo de B)

### Queries (`api/db/queries/training_adjustments.sql`)
- `GetTrainingAdjustment(user_id)` → fila o `ErrNoRows`.
- `UpsertTrainingAdjustment(user_id, scope, content)` →
  `INSERT ... ON CONFLICT (user_id) DO UPDATE SET scope, content, created_at = now()`,
  `RETURNING *`.

### Servicio (`api/internal/training/adjustment.go`)
- Constantes: `adjustmentLastSessions = 3`, `adjustmentWeekDays = 7`.
- `Adjustment(ctx, userID)` → `*Adjustment` (`{scope, content, created_at}`) o `nil`.
- `SuggestAdjustments(ctx, userID, scope string, today time.Time) (*Adjustment, error)`:
  1. `scope` debe ser `last` o `week` (validado también en el handler; defensa
     en profundidad: si llega otro, error → 400 / o se asume default en el handler).
  2. `!hasKey` → `ErrUnavailable` (503).
  3. Perfil (tolera `ErrNoRows`).
  4. `ListWorkouts` (orden date DESC) y **filtra por alcance**:
     - `last`: las primeras `adjustmentLastSessions` (3) sesiones.
     - `week`: las sesiones con `date >= today − 6 días` (últimos 7 días).
  5. Series de esas sesiones (`ListSetsByWorkoutIDs`, con notas).
  6. **Reutiliza `buildSuggestionContext`** (perfil + historial + notas; `focus`
     vacío).
  7. Llama `Complete(adjustmentsSystemPrompt, contexto)`. Fallo/empty → `ErrUnavailable`.
  8. Upsert `{scope, content}` y devuelve.
- **System prompt de ajustes** (`adjustmentsSystemPrompt`): entrenador que
  **analiza** el historial reciente + las notas de cada serie y propone **ajustes
  accionables** para la próxima sesión/semana: progresión o descarga (peso/reps),
  qué cambiar por molestias o por lo anotado, técnica, y un resumen breve; en
  español, concreto. Si no hay entrenos en el alcance, lo dice y sugiere empezar
  a registrar.
- Reutiliza `completer`, `ErrUnavailable`, `ageFrom`, `buildSuggestionContext` de
  `suggestion.go` (slice B).

### Vista (JSON)
```
Adjustment { scope: string, content: string, created_at: string }
```

### Rutas (`training.Routes`, ya bajo `RequireAuth`)
- `GET /training/adjustment` → `200` con la vista o `null`.
- `POST /training/adjustment` `{ scope }` → `200` con el análisis; `400` si
  `scope ∉ {last, week}`; `503` si `ErrUnavailable`.

## 4. Frontend

- **`web/src/lib/trainingAdjustment.ts`:** `type TrainingAdjustment = { scope,
  content, created_at }`; `getAdjustment()`; `generateAdjustment(scope: "last" |
  "week")`.
- **`web/src/routes/entrenamiento.tsx`:** un `<Card>` **"Análisis del agente"**
  debajo del panel "Entrenador IA":
  - Toggle de alcance: dos botones/chips "Último entreno" (`last`) y "Última
    semana" (`week`); estado local `adjustScope`.
  - Botón **"Analizar"** (deshabilitado + "Analizando…" mientras corre).
  - El último análisis renderizado con `whitespace-pre-wrap` + etiqueta del
    alcance + fecha relativa. Query `["training-adjustment"]` precarga el último;
    la mutación actualiza el caché (`setQueryData`).
  - 503 → mensaje "El entrenador no está disponible por ahora". Estado vacío →
    invitación a pedir un análisis.

## 5. Manejo de errores

- Sin clave/fallo de IA → `503` (se conserva el último análisis). `scope`
  inválido → `400`. Sin entrenos en el alcance → genera igual. Ownership por PK.

## 6. Testing

- **Backend (servicio, Completer fake):** `SuggestAdjustments` con `scope=last`
  pasa hasta 3 sesiones al contexto; con `scope=week` filtra por fecha (incluye
  las de los últimos 7 días, excluye las más viejas); el contexto incluye las
  notas de serie; persiste con el `scope`; `!hasKey` → `ErrUnavailable`; fallo del
  Completer → `ErrUnavailable`; `Adjustment` devuelve el último o nil.
- **Backend (handler):** `POST` válido → 200; `scope` inválido → 400; sin clave →
  503; `GET` último o `null`; ownership.
- **Frontend:** elegir alcance + "Analizar" llama `generateAdjustment(scope)` y
  muestra el texto; precarga el último; estados "Analizando…"/503; estado vacío.
- **E2E producción:** generar un análisis con `scope=last` y con `scope=week`,
  leerlo con `GET`, regenerar y verificar que reemplaza; ownership.

## 7. Criterios de aceptación

- Desde `/entrenamiento` se pide un análisis (último entreno / última semana) y el
  agente devuelve ajustes concretos basados en el historial reciente y las notas.
- El último análisis se guarda y se muestra al volver; regenerar lo reemplaza.
- `scope` inválido → 400; sin IA → 503 con gracia. Estrictamente por usuario.
- Suites en verde; smoke de producción OK.
