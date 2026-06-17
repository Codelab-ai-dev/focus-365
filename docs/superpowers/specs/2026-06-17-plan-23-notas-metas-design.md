# Plan 23 — Notas de avance por meta — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Cada meta gana una **bitácora de notas fechadas** para registrar avances. Desde
un botón **📝 Notas** en la card de la meta se abre el `Modal` (el componente
reutilizable de la R22) con un campo para agregar una nota (texto + fecha, que
arranca en hoy pero es elegible) y la lista de notas en orden cronológico
descendente. Podés tener varias notas, incluso del mismo día.

**Decisiones (brainstorming):**
- **Bitácora**, no diario: varias entradas fechadas por meta (no una por día).
- **Fecha elegible** con `<input type="date">`, default hoy.
- **UI en Modal** abierto desde la card; reutiliza `ui/Modal.tsx`.
- **Operaciones: agregar y borrar** (sin editar; si te equivocás, borrás y
  reescribís).

**Fuera de alcance:** editar notas; que la IA agregue notas; adjuntar archivos a
una nota; agrupar visualmente por fecha (lista plana ordenada).

## 2. Modelo de datos (migración `0019_goal_notes.sql`)

```sql
-- +goose Up
CREATE TABLE goal_notes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id    UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_date  DATE NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_goal_notes_goal ON goal_notes (goal_id, note_date DESC, created_at DESC);

-- +goose Down
DROP TABLE goal_notes;
```

- Borrar una meta arrastra sus notas (FK `ON DELETE CASCADE`), igual que al
  borrar el usuario.
- `user_id` denormalizado para chequear dueño sin join, consistente con el resto
  del código (commitments, ai_messages, etc.).
- `note_date` es `DATE` → sqlc lo genera como `time.Time` por el override de
  `sqlc.yaml` (mismo caso que `goals.deadline`).

## 3. Backend

### Queries (`api/db/queries/goal_notes.sql`)
- **`CreateGoalNote`** inserta **solo si la meta es del usuario**, evitando colgar
  notas en metas ajenas en una sola query:
  ```sql
  -- name: CreateGoalNote :one
  INSERT INTO goal_notes (goal_id, user_id, note_date, body)
  SELECT @goal_id, @user_id, @note_date, @body
  WHERE EXISTS (SELECT 1 FROM goals WHERE id = @goal_id AND user_id = @user_id)
  RETURNING *;
  ```
  Si la meta no es del usuario, no inserta ninguna fila → `:one` devuelve
  `pgx.ErrNoRows` → el servicio lo traduce a 404.
- **`ListGoalNotes`** ordena por fecha y luego por inserción:
  ```sql
  -- name: ListGoalNotes :many
  SELECT n.* FROM goal_notes n
  JOIN goals g ON g.id = n.goal_id
  WHERE n.goal_id = @goal_id AND g.user_id = @user_id
  ORDER BY n.note_date DESC, n.created_at DESC;
  ```
- **`DeleteGoalNote`** (`:execrows`) borra validando dueño:
  ```sql
  -- name: DeleteGoalNote :execrows
  DELETE FROM goal_notes WHERE id = @id AND user_id = @user_id;
  ```

### Servicio y rutas (paquete `goals`)
- El servicio gana `Notes(ctx, userID, goalID)`, `AddNote(ctx, userID, goalID,
  noteDate time.Time, body)` y `DeleteNote(ctx, userID, goalID, noteID)`.
  `AddNote` traduce `pgx.ErrNoRows` (meta ajena/inexistente) a un error que el
  handler mapea a 404; `DeleteNote` a 404 si afecta 0 filas.
- Rutas nuevas en `goals.Routes` (ya bajo `RequireAuth`):
  - `GET /goals/{id}/notes` → `{ notes: [...] }`.
  - `POST /goals/{id}/notes` `{ note_date, body }` → `{ note }` (201).
  - `DELETE /goals/{id}/notes/{noteId}` → 204; 404 si no es del usuario.
- Validación en el handler: `body` no vacío tras `TrimSpace` (máx 1000 runes →
  400 si excede); `note_date` parseable como `YYYY-MM-DD` (400 si no); ids
  parseables (404 si no).

### Vista (JSON)
```
NoteView { id, goal_id, note_date (YYYY-MM-DD), body, created_at }
```
`note_date` se serializa como `YYYY-MM-DD` (string), consistente con cómo el
resto de la app maneja fechas de día.

## 4. Frontend

- **`web/src/lib/goalNotes.ts`:** `type GoalNote = { id, goal_id, note_date,
  body, created_at }`; `listGoalNotes(goalId)`, `createGoalNote(goalId, { note_date,
  body })`, `deleteGoalNote(goalId, noteId)`.
- **`web/src/routes/metas.tsx`:**
  - Botón **📝 Notas** en cada card de meta → setea `notesGoal: Goal | null`.
  - Un `GoalNotesModal` (en `metas.tsx`) abre el `Modal` con `open = notesGoal
    !== null`, título = el de la meta. Contenido:
    - Form: `<textarea>` + `<input type="date">` (default `todayString()`) +
      botón "Agregar". Al enviar, `createGoalNote` e invalidar la query.
    - Lista de notas (`useQuery(["goal-notes", goalId], …, { enabled })`) en orden
      del backend, cada una con la fecha legible (parseada en local, como
      `formatDay` del check-in), el body y 🗑️ (`deleteGoalNote` + invalidar).
    - Estado vacío: "Sin notas todavía".
- El selector de fecha usa el mismo `todayString()` de `lib/goals`/`checkins`.

## 5. Manejo de errores

- Crear/listar/borrar nota en meta ajena → 404 (ownership por `WHERE EXISTS` /
  `user_id`), sin filtrar existencia.
- Body vacío tras trim → 400; body > 1000 runes → 400.
- `note_date` con formato inválido → 400.
- Borrar la meta arrastra sus notas (cascada) — sin acción del frontend.
- Si falla la creación, el texto queda en el campo (no se limpia hasta el éxito).

## 6. Testing

- **Backend (store):** crear nota en meta propia (ok) y en meta ajena (0 filas →
  ErrNoRows); listar ordenado por `note_date DESC, created_at DESC`; borrar por
  dueño (1 fila) y ajeno (0 filas); cascada: borrar la meta elimina sus notas.
- **Backend (handler):** `POST` 201, body vacío → 400, fecha inválida → 400, meta
  ajena → 404; `GET` lista (200 propio, 404/empty ajeno); `DELETE` 204 propio,
  404 ajeno.
- **Frontend:** abrir el modal lista las notas mockeadas (orden + fecha legible);
  agregar postea con la fecha por defecto (hoy) y refresca; borrar quita la nota;
  estado vacío.
- **E2E producción:** crear una meta, agregarle dos notas con fechas distintas,
  verificar el orden, borrar una; confirmar que no aparecen notas de otra meta/
  usuario.

## 7. Criterios de aceptación

- Cada meta tiene su bitácora de notas fechadas; agregás (con fecha elegible,
  default hoy) y borrás; se ven en orden cronológico descendente.
- Las notas son estrictamente del usuario dueño de la meta (404 cruzado).
- Borrar una meta elimina sus notas.
- Suites en verde; smoke de producción OK.
