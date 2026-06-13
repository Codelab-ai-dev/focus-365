# Plan 15 — Multi-acción por turno + deshacer — Diseño

**Fecha:** 2026-06-12
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Dos capacidades estructurales para las acciones de la IA:

1. **Multi-acción por turno:** «registra mi check-in: 8 7 9 y marca meditación»
   produce un mensaje con **N tarjetas** (una por acción), cada una con su
   Confirmar/Cancelar independiente. Tope: 5 acciones por turno.
2. **Deshacer:** toda tarjeta `done` gana un botón **Deshacer** (una vez, sin
   límite de tiempo). La reversa es por kind y *best-effort*: si el dato se
   editó a mano después, deshacer revierte sobre el estado actual; si el dato
   ya no existe, la acción pasa a `undone` igual.

**Decisiones de brainstorming:** deshacer check-in **restaura el estado
previo** (snapshot al ejecutar; si no había check-in ese día, deshacer borra el
día — requiere el nuevo `checkin.Delete`); ventana de undo: **siempre, una
vez** (`done → undone`).

**Fuera de alcance:** editar una propuesta, deshacer un undo (redo), deshacer
en cascada, acciones de borrado/archivado propuestas por la IA.

## 2. Modelo de datos (migración `0011_ai_actions_table.sql`)

La relación pasa de 1:1 (columnas en `ai_messages`) a 1:N con tabla propia:

```sql
-- +goose Up
CREATE TABLE ai_actions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position   INT  NOT NULL DEFAULT 0,
    kind       TEXT NOT NULL CHECK (kind IN (
        'checkin','movimiento','habito','meta',
        'habito_nuevo','meta_nueva','entrenamiento')),
    payload    JSONB NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('proposed','done','cancelled','undone')),
    result     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_actions_message ON ai_actions (message_id, position);
CREATE INDEX idx_ai_actions_user ON ai_actions (user_id);

-- Migra las acciones existentes (1 por mensaje) a filas.
INSERT INTO ai_actions (message_id, user_id, position, kind, payload, status, created_at)
SELECT id, user_id, 0, action_kind, action_payload, action_status, created_at
FROM ai_messages
WHERE action_kind IS NOT NULL;

ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_consistente,
    DROP CONSTRAINT ai_messages_action_status_valid,
    DROP CONSTRAINT ai_messages_action_kind_valid,
    DROP COLUMN action_status,
    DROP COLUMN action_payload,
    DROP COLUMN action_kind;
-- +goose Down (recrea columnas, repuebla con la acción position=0 y dropea la tabla)
```

(El Down restaura las columnas y constraints de 0009/0010 y repuebla desde la
fila `position = 0` de cada mensaje; las acciones extra se pierden — aceptable
para un Down.)

**Queries (`db/queries/ai_actions.sql`):**
- `CreateAction :one` — INSERT (message_id, user_id, position, kind, payload, status).
- `ListActionsByMessages :many` — `WHERE message_id = ANY($1::uuid[]) ORDER BY message_id, position` (para armar el historial sin N+1).
- `GetAction :one` — `WHERE id = $1 AND user_id = $2`.
- `SetActionStatusFrom :one` — `UPDATE ai_actions SET status = $3, result = COALESCE($4, result) WHERE id = $1 AND user_id = $2 AND status = $5 RETURNING *` (transición atómica genérica: `proposed→done/cancelled` y `done→undone`; `$4` permite guardar el result al confirmar).

Las queries viejas de acciones sobre `ai_messages`
(`CreateMessageWithAction`, `GetMessageForAction`, `SetActionStatus`) se
eliminan junto con el código que las usa.

## 3. Cliente Groq (`groq.go`)

`ChatStream` devuelve **todas** las tool calls del turno:

```go
func (c *GroqClient) ChatStream(ctx, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, []ToolCall, error)
```

Reensamblado por `index` en un mapa (name llega una vez por index, arguments
fragmentado); al cierre se ordenan por index. Se eliminan la regla «first
wins» de la R11 y su test (reemplazado por uno multi-index). La interfaz
`chatStreamer` y los fakes cambian de firma.

## 4. Servicio (`chat.go`, `actions.go`)

- **Proponer (all-or-nothing):** `SendStream` valida TODAS las tool calls del
  turno (tope `maxActionsPerTurn = 5`; más de 5 o alguna inválida →
  `ErrUnavailable`, nada se persiste). Persiste con
  `CreatePairWithActions(userID, userText, assistantText, actions []ProposedAction)`
  (transacción: mensaje user + mensaje assistant + N filas `proposed`).
  El `assistantContent` de fallback con varias acciones: «Propongo N acciones:»
  + los summaries concatenados.
- **Vista:** `Message.Action *ActionView` → `Message.Actions []ActionView`
  (cada una con `ID`, `Kind`, `Payload`, `Status`). `History` arma los
  mensajes y cuelga sus acciones con `ListActionsByMessages`.
- **Confirmar:** `ConfirmAction(userID, actionID, today)` — `GetAction`,
  exige `proposed`, ejecuta, y `SetActionStatusFrom(done, result, from=proposed)`.
  El **ejecutor devuelve result**: `execute(...) (json.RawMessage, error)`:

  | kind | result |
  |------|--------|
  | checkin | `{"prev": {mood,energy,discipline,note} \| null}` (lee `checkin.Today` antes del upsert) |
  | movimiento | `{"tx_id": "..."}` |
  | habito | `{}` |
  | meta | `{"prev_progress": N}` (lee la meta antes del patch) |
  | habito_nuevo | `{"habit_id": "..."}` |
  | meta_nueva | `{"goal_id": "..."}` |
  | entrenamiento | `{"workout_id": "..."}` |

- **Deshacer:** `UndoAction(userID, actionID, today)` — `GetAction`, exige
  `done`, ejecuta la reversa con el `result` guardado y
  `SetActionStatusFrom(undone, nil, from=done)`:
  - checkin: `prev != null` → `checkin.Upsert(prev)`; `prev == null` →
    `checkin.Delete(userID, today del payload… )` — **la fecha de la reversa es
    la fecha en que se ejecutó la acción**: se guarda en el result
    (`{"prev":…, "date":"YYYY-MM-DD"}`) para no depender del `today` del undo.
  - movimiento: `finance.Delete(tx_id)`; si devuelve «no existía», se ignora
    (best-effort) y se transiciona igual.
  - habito: `SetCheck(habit_id del payload, date del result, false)` — el
    result guarda `{"date": "..."}` también.
  - meta: `Patch{Progress: prev_progress}`; si la meta ya no existe → ignorar.
  - habito_nuevo/meta_nueva/entrenamiento: `Delete` del id del result;
    inexistente → ignorar.
  - Regla general: **errores reales de DB abortan** (la acción sigue `done`);
    «ya no existe» transiciona a `undone`.
- **`checkin.Delete(ctx, userID, date)`** se agrega al servicio de check-in
  (query `DeleteCheckIn` por user+date) con test propio.

## 5. API

- `GET /ai/messages` → mensajes con `"actions": [...]` (array, puede ser vacío;
  el campo `action` singular desaparece — el frontend se actualiza en la misma
  rebanada, no hay consumidores externos).
- `POST /ai/actions/{id}/confirm|cancel` — igual contrato, `id` ahora es el de
  la **acción**.
- `POST /ai/actions/{id}/undo` → `200 {"action": ActionView}` con `undone`.
  Errores: `404` (no existe/no es tuya), `409` (no está `done`), `500`.

## 6. Prompt

«Una sola acción por turno» pasa a: «Puedes proponer hasta 5 acciones en un
turno, solo las que el usuario pidió explícitamente; si pide varias cosas,
propón una acción por cada una.»

## 7. Frontend

- `Message.action?` → `Message.actions?: Action[]`; `Action` gana `id` propio
  (el `id` del mensaje ya no se usa para confirmar). `undoAction(id)` en la lib.
- La burbuja del asistente renderiza una `ActionCard` por elemento de
  `actions`. Tarjeta `done` → botón **Deshacer** (`Button variant="ghost"`
  pequeño); `undone` → marca «Deshecha» atenuada. Pending por tarjeta
  (`actionMutation.variables?.id === action.id`).
- `setQueryData` al resolver: reemplaza la acción dentro del mensaje (buscar
  mensaje por `message.id`… la respuesta del undo/confirm trae solo la acción:
  el caché se actualiza mapeando mensajes y reemplazando la acción por su id).

## 8. Manejo de errores

| Caso | Respuesta | UX |
|------|-----------|-----|
| >5 tool calls o alguna inválida | `503` (turno completo descartado) | error inline + reintento |
| Confirmar/cancelar/deshacer en estado equivocado | `409` | tarjeta se refresca |
| Reversa de dato ya inexistente | `200` y `undone` (best-effort) | tarjeta «Deshecha» |
| Error real de DB en la reversa | `500`, sigue `done` | error inline, reintento |
| Doble undo (carrera) | el `WHERE status='done'` de la query lo hace atómico → `409` | — |

## 9. Testing

- **Migración:** datos pre-existentes en columnas → filas (sembrar con SQL de
  la 0010, migrar, verificar); round-trip de `ai_actions` y de
  `SetActionStatusFrom` (transiciones válidas/ inválidas, scoping).
- **Cliente:** dos tool calls con index 0/1 fragmentadas → ambas reensambladas
  en orden; (reemplaza el test first-wins).
- **Servicio:** turno con 2 acciones persiste 2 filas `proposed`; 6 acciones →
  `ErrUnavailable`; una inválida de tres → nada persistido; confirm guarda
  result por kind; undo por kind con fakes (checkin con/sin prev; movimiento
  inexistente transiciona igual; error de DB no transiciona); doble undo.
- **checkin.Delete:** test de servicio (borra, scoping por usuario).
- **Handler:** historial con `actions[]`; confirm/cancel/undo felices y 404/409;
  undo de acción `cancelled` → 409.
- **Frontend:** mensaje con 2 tarjetas; confirmar una no toca la otra; botón
  Deshacer solo en `done`; deshacer pasa a «Deshecha»; pending por tarjeta.
- **E2E producción (cierre):** «registra mi check-in: 8 7 9 y marca meditación»
  → 2 tarjetas → confirmar ambas → deshacer el check-in → el check-in del día
  desaparece (o vuelve al previo).

## 10. Criterios de aceptación

- Un turno con varios pedidos produce varias tarjetas independientes; máximo 5.
- Toda tarjeta `done` puede deshacerse exactamente una vez y la reversa es
  correcta por kind (incluido el snapshot del check-in).
- Las acciones históricas (pre-migración) siguen visibles con su estado.
- Suites completas en verde; smoke de producción del flujo multi-acción + undo.
