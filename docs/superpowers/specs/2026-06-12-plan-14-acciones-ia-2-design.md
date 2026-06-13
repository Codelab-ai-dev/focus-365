# Plan 14 — Acciones de la IA, parte 2 (crear hábito/meta + registrar entrenamiento) — Diseño

**Fecha:** 2026-06-12
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Tres capacidades nuevas para el asistente, sobre el mecanismo de acciones de la
R11 (tool call → tarjeta `proposed` → Confirmar/Cancelar → ejecutor):

1. **Crear hábito** — «crea un hábito de leer 30 min» (`habits.Create`).
2. **Crear meta** — «nueva meta: ahorrar 50k para diciembre» (`goals.Create`).
3. **Registrar entrenamiento completo** — «registra fuerza: press banca 3x8
   con 60kg, sentadilla 3x5 con 80» (`training.CreateWorkout` con series;
   los ejercicios se auto-crean por nombre — el servicio ya lo hace).

Sin cambios de flujo, UI genérica ni modelo de datos, salvo ampliar el CHECK
de `action_kind`. Decisiones de brainstorming: división confirmada (R15 hará
multi-acción por turno y deshacer); captura de entrenamiento con **sesión
completa** (tipo + series), no solo el tipo.

**Fuera de alcance (R15):** deshacer acciones ejecutadas, múltiples acciones
por turno, editar propuesta, borrar/archivar desde el chat.

## 2. Modelo de datos (migración `0010_ai_action_kinds.sql`)

La 0009 restringe `action_kind IN ('checkin','movimiento','habito','meta')`.
La 0010 reemplaza el constraint para sumar `'habito_nuevo'`, `'meta_nueva'`,
`'entrenamiento'` (DROP CONSTRAINT + ADD con la lista completa; Down revierte
a la lista de 0009).

## 3. Tools (definiciones para Groq)

| Tool | Parámetros (JSON Schema) | Mapea a |
|------|--------------------------|---------|
| `crear_habito` | `name` (string, required), `target_days` (int ≥1, opcional) | `habits.Create(HabitInput{Name, TargetDays})` |
| `crear_meta` | `title` (string, required), `dimension` (enum `checkin\|finanzas\|entrenamiento\|mente\|general`, required), `deadline` (string YYYY-MM-DD, opcional) | `goals.Create(GoalInput{Title, Dimension, Deadline})` |
| `registrar_entrenamiento` | `type` (string, required), `note` (string, opcional), `sets` (array de `{exercise: string, reps?: int ≥1, weight_kg?: number >0}`, required, 1–20 items) | `training.CreateWorkout(WorkoutInput{Date: today, Type, Note, Sets})` con `WeightGrams = round(weight_kg*1000)` |

Las descripciones (en español) instruyen: usarlas solo ante pedido explícito
con los datos dados; en metas, `dimension` se infiere del tema si el usuario
no la dice (p. ej. ahorro → finanzas) y `general` como fallback; nunca
inventar pesos/reps no dichos (las series pueden venir sin reps/peso).

## 4. Backend (paquete `ai`, `actions.go`)

- **Kinds:** `actionHabitoNuevo = "habito_nuevo"`, `actionMetaNueva =
  "meta_nueva"`, `actionEntrenamiento = "entrenamiento"`; entradas en
  `toolNameToKind`.
- **Payloads** (tags JSON = parámetros de la tool):
  - `habitoNuevoPayload{Name string, TargetDays *int32}`
  - `metaNuevaPayload{Title, Dimension string, Deadline string}` (deadline
    "" = sin fecha)
  - `entrenamientoPayload{Type, Note string, Sets []setPayload}` con
    `setPayload{Exercise string, Reps *int32, WeightKg *float64}`
- **Validación en `parseActionPayload`:** name/title/type no vacíos tras trim;
  dimension ∈ {checkin, finanzas, entrenamiento, mente, general}; deadline
  vacío o `time.Parse("2006-01-02")`; target_days nil o ≥1; sets 1–20, cada
  uno con exercise no vacío, reps nil o ≥1, weight_kg nil o >0.
- **`actionSummary`:** «Propongo crear el hábito "Leer 30 min".» /
  «Propongo crear la meta "Ahorrar 50k" (finanzas) para 2026-12-01.» /
  «Propongo registrar un entrenamiento de fuerza con 6 series.»
- **Ejecutor:** tres interfaces estrechas nuevas (`habitCreator`,
  `goalCreator`, `workoutCreator` sobre los servicios reales);
  `NewActionExecutor` pasa de 4 a 7 dependencias (wiring en `server.go` y
  `handler_test.go`). Conversión kg→gramos en el caso entrenamiento; deadline
  a `*time.Time`.
- **Prompt (`chatprompt.go`):** una línea más en el bloque de acciones.

## 5. Frontend (`asistente.tsx`)

- `ACTION_TITLES`: `habito_nuevo: "Nuevo hábito"`, `meta_nueva: "Nueva meta"`,
  `entrenamiento: "Entrenamiento"`.
- `actionDetails`: hábito → `"Leer 30 min · objetivo 21 días"` (u «sin
  objetivo»); meta → `"Ahorrar 50k · finanzas · para 2026-12-01"`; entreno →
  `"fuerza · press banca 3×8 @60kg · sentadilla 3×5 @80kg"` (· entre series;
  reps/peso solo si vienen).
- Nada más: tarjeta, botones, mutaciones y caché son genéricos por kind.

## 6. Manejo de errores

Idéntico a R11: tool desconocida/args inválidos al proponer → 503 (nada se
persiste); payload que el dominio rechaza al confirmar → 400 y la acción sigue
`proposed`; doble confirm → 409. Sin casos nuevos.

## 7. Testing

- **Payloads:** casos válidos/ inválidos por kind (deadline malformado,
  dimension inválida, sets vacíos/21, weight_kg negativo…).
- **Ejecutor:** con fakes de los 3 servicios — hábito con/sin target_days;
  meta con/sin deadline; entrenamiento convierte kg→gramos y pasa las series
  en orden.
- **Handler (integración DB):** proponer un hábito vía chat (fake streamer con
  tool call) → confirmar → el hábito existe (`habits.List`); kind nuevo pasa
  el CHECK de la 0010.
- **Frontend:** `actionDetails` por kind nuevo; tarjeta de entrenamiento
  muestra las series.
- **E2E producción (cierre):** push → auto-deploy → smoke contra prod:
  crear un hábito por chat → confirmar → aparece en `/habits`.

## 8. Criterios de aceptación

- Las tres frases de ejemplo del §1 producen tarjetas correctas; confirmar
  crea el dato real; las 4 acciones de la R11 siguen funcionando.
- `make check` backend + Vitest/build frontend en verde; smoke de producción
  OK tras el deploy.
