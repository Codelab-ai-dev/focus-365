# Plan 25 — Entrenador IA: sugerencias (entrenamiento, slice B) — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 0. Contexto: descomposición del módulo de entrenamiento

Segundo slice (**B**) de la expansión de entrenamiento (ver el spec del slice A,
`2026-06-17-plan-24-perfil-fitness-design.md`). Sub-proyectos:
- A — Perfil de fitness (hecho, rebanada 24).
- **B — Agente de sugerencias** (este spec): el agente propone rutinas/ejercicios
  según el perfil + historial.
- C — Notas por ejercicio + ajustes del agente.
- D — Evolución / progreso.

## 1. Visión y alcance

En `/entrenamiento`, un panel **"Entrenador IA"** con: un campo opcional de
enfoque ("pierna", "30 min", "sin saltos por la rodilla"), un botón **"Sugerir"**,
y debajo la última sugerencia. Al apretar, el agente lee tu **perfil de fitness**
(slice A) + tu **historial reciente** de entrenos + el enfoque, y devuelve una
**rutina/ejercicios explicados en texto libre**, pensada para tu equipo y
objetivo. La última sugerencia se **guarda** (sobrevive recarga); regenerar la
reemplaza.

**Decisiones (brainstorming):**
- **Texto libre** (prosa explicada), no estructurado.
- **On-demand** con botón (cada vez regenera), no cacheada automática.
- **Campo de enfoque opcional**; vacío = sugiere desde perfil + historial.
- **Se guarda la última** (1 por usuario; regenerar reemplaza).

**Fuera de alcance (C/D):** notas por ejercicio y ajustes post-entreno (C);
evolución/progreso (D); cargar la rutina sugerida al registro de entreno (sería
formato estructurado — acá es texto libre); historial de sugerencias (solo la
última).

## 2. Modelo de datos (migración `0021_training_suggestions.sql`)

```sql
-- +goose Up
CREATE TABLE training_suggestions (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    focus      TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE training_suggestions;
```

- **1 fila por usuario** (PK = user_id): se guarda solo la última sugerencia;
  regenerar hace upsert. Cascada al borrar el usuario.

## 3. Backend

### Queries (`api/db/queries/training_suggestions.sql`)
- `GetTrainingSuggestion(user_id)` → la fila o `ErrNoRows`.
- `UpsertTrainingSuggestion(user_id, focus, content)` →
  `INSERT ... ON CONFLICT (user_id) DO UPDATE SET focus, content, created_at = now()`,
  `RETURNING *`.
- Para el contexto del prompt se reusan las queries existentes de entrenamiento
  (`ListWorkouts`, `ListSetsByWorkoutIDs`) y del perfil (`GetFitnessProfile`).

### Servicio (paquete `training`)
- `training.Service` gana un `Completer` de Groq (interfaz `Complete(ctx,
  system, user) (string, error)`) y `hasKey bool`, inyectados en `server.go`
  (el `GroqClient` ya implementa `Complete`). Se agregan como campos nuevos sin
  romper la firma existente: `NewService(q, pool, completer, hasKey)`.
- `Suggestion(ctx, userID)` → `*Suggestion` (la última) o `nil`.
- `SuggestTraining(ctx, userID, focus string) (*Suggestion, error)`:
  1. Si `!hasKey` → `ErrUnavailable` (el handler lo traduce a 503).
  2. Arma el **contexto**: perfil (edad calculada de `birthdate`, peso en kg,
     altura, sexo, objetivo, equipo, lugar, nivel, días/semana, limitaciones) +
     **últimos 8 entrenos** (`ListWorkouts` acotado, con sus series vía
     `ListSetsByWorkoutIDs`: ejercicio, reps, peso en kg) + el `focus`.
  3. Llama `Complete(systemPrompt, userContext)`. Error de Groq → `ErrUnavailable`.
  4. Upsert de la sugerencia y la devuelve.
- **System prompt:** entrenador personal en español; prioriza el **equipo
  disponible** y el **lugar** (mayormente casa), apunta al **objetivo**, respeta
  las **limitaciones**, ajusta a **nivel/frecuencia**; si el perfil está
  incompleto, sugiere genérico y recomienda completar el perfil. Devuelve una
  rutina/ejercicios con series×reps y descansos, explicada brevemente.
- `ErrUnavailable` se reutiliza si existe en `training`; si no, se define
  (`var ErrUnavailable = errors.New("entrenador no disponible")`).

### Rutas (`training.Routes`, ya bajo `RequireAuth`)
- `GET /training/suggestion` → `200` con `{focus, content, created_at}` o `null`.
- `POST /training/suggestion` `{ focus?: string }` → `200` con la sugerencia
  generada; `503` si `ErrUnavailable`. `focus` opcional, **máx 200 runes** (→ 400
  si excede); se hace `TrimSpace`.

### Vista (JSON)
```
Suggestion { focus: string, content: string, created_at: string }
```

## 4. Frontend

- **`web/src/lib/trainingSuggestion.ts`:** `type TrainingSuggestion = { focus,
  content, created_at }`; `getSuggestion(): Promise<TrainingSuggestion | null>`;
  `generateSuggestion(focus: string): Promise<TrainingSuggestion>`.
- **`web/src/routes/entrenamiento.tsx`:** un panel/`Card` **"Entrenador IA"**
  (arriba del historial):
  - `<input>` de enfoque (opcional, placeholder con ejemplos) + botón
    **"Sugerir"** (deshabilitado mientras genera, texto "Generando…").
  - La última sugerencia se renderiza con `whitespace-pre-wrap` (saltos de línea
    preservados; **sin** librería de markdown) + la fecha relativa.
  - Query `["training-suggestion"]` con `getSuggestion` precarga la última; la
    mutación de generar actualiza el caché (setQueryData) con el resultado.
  - Error 503 → mensaje "El entrenador no está disponible por ahora".
  - Estado vacío (sin sugerencia) → invitación a pedir una.

## 5. Manejo de errores

- Sin clave de Groq o fallo de IA → `503`; el front muestra el mensaje de no
  disponible y conserva la última sugerencia.
- `focus` > 200 runes → `400`.
- Perfil vacío / sin historial → genera igual (el prompt lo contempla).
- Ownership por PK: cada usuario ve/genera solo su sugerencia.

## 6. Testing

- **Backend (servicio, con Completer fake):** `SuggestTraining` incluye en el
  prompt los datos del perfil, el historial y el `focus`; persiste y devuelve;
  `!hasKey` → `ErrUnavailable`; un fallo del Completer → `ErrUnavailable`;
  regenerar reemplaza (sigue una fila); `Suggestion` devuelve la última o nil.
- **Backend (handler):** `POST` válido → 200; sin clave → 503; `focus` largo →
  400; `GET` última o `null`; ownership (otro usuario no la ve).
- **Frontend:** "Sugerir" llama `generateSuggestion` y muestra el texto; la última
  precarga al entrar; estado "Generando…"; 503 muestra el mensaje; estado vacío.
- **E2E producción:** generar una sugerencia (con y sin enfoque), leerla con un
  `GET`, regenerar y verificar que reemplaza; confirmar que no se filtra a otro
  usuario.

## 7. Criterios de aceptación

- Desde `/entrenamiento` se pide una sugerencia (con enfoque opcional) y el
  agente devuelve una rutina/ejercicios en texto, acorde al perfil e historial.
- La última sugerencia se guarda y se muestra al volver; regenerar la reemplaza.
- Sin IA disponible → 503 manejado con gracia. Estrictamente por usuario.
- Suites en verde; smoke de producción OK.
