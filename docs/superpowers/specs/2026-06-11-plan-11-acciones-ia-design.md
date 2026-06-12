# Plan 11 — Acciones de la IA (tool-use con confirmación) — Diseño

**Fecha:** 2026-06-11
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

El chat de `/asistente` deja de ser read-only: el usuario pide «registra mi
check-in: ánimo 8, energía 6, disciplina 9» y la IA **propone** la acción como
tarjeta estructurada en el chat. **Solo al pulsar Confirmar se escribe** en la
base, vía los servicios existentes y sus validaciones. Nada se ejecuta sin
confirmación explícita.

**Decisiones de alcance (acordadas en brainstorming):**
- **Cuatro acciones** (una por módulo de escritura existente):
  1. Registrar/actualizar el **check-in** de hoy (`checkin.Upsert`).
  2. Registrar un **movimiento financiero** (`finance.Create`).
  3. **Marcar un hábito** como hecho hoy (`habits.SetCheck`).
  4. Actualizar el **progreso de una meta** (`goals.Patch`).
- **Confirmación en UI:** la propuesta es una tarjeta con Confirmar/Cancelar.
- **Enfoque A:** tools en el endpoint de streaming existente; la propuesta se
  persiste como un mensaje más de `ai_messages` (columnas nuevas opcionales) y
  sobrevive recargas. Una acción por turno.
- **Sin nuevo evento SSE:** el evento `done` existente carga el mensaje
  completo, que ahora puede incluir la acción propuesta. (En turnos de acción
  el modelo no emite deltas de texto; el frontend muestra «Pensando…» hasta el
  `done`.)

**Fuera de alcance:** ejecutar sin confirmar, deshacer acciones ejecutadas,
múltiples acciones por turno, crear/borrar hábitos o metas, acciones sobre
entrenamiento, editar la propuesta antes de confirmar (se cancela y se pide de
nuevo).

## 2. Arquitectura

Se extiende el camino de streaming de la rebanada 10 capa por capa:
`GroqClient.ChatStream` gana soporte de tools (acumula los fragmentos de
`tool_calls` del stream); `ChatService.SendStream` interpreta el tool call,
valida su forma y persiste el par con la acción en estado `proposed`; dos
endpoints nuevos (`confirm`/`cancel`) cierran el ciclo ejecutando vía un
`actionExecutor` que delega en los servicios existentes. El contexto del chat
se amplía con hábitos y metas (con IDs) para que el modelo pueda referenciarlos.

El camino bloqueante `POST /ai/chat` **no** recibe tools (queda como está).

## 3. Modelo de datos (migración `0009_ai_actions.sql`)

```sql
-- +goose Up
ALTER TABLE ai_messages
    ADD COLUMN action_kind    TEXT,
    ADD COLUMN action_payload JSONB,
    ADD COLUMN action_status  TEXT,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN ('checkin','movimiento','habito','meta')
    ),
    ADD CONSTRAINT ai_messages_action_status_valid CHECK (
        action_status IS NULL OR action_status IN ('proposed','done','cancelled')
    ),
    ADD CONSTRAINT ai_messages_action_consistente CHECK (
        (action_kind IS NULL AND action_payload IS NULL AND action_status IS NULL)
        OR (action_kind IS NOT NULL AND action_payload IS NOT NULL AND action_status IS NOT NULL)
    );
-- +goose Down
ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_consistente,
    DROP CONSTRAINT ai_messages_action_status_valid,
    DROP CONSTRAINT ai_messages_action_kind_valid,
    DROP COLUMN action_status,
    DROP COLUMN action_payload,
    DROP COLUMN action_kind;
```

**Queries (`db/queries/ai_messages.sql`, ampliación):**
- `CreateMessageWithAction :one` — INSERT con los tres campos de acción.
- `GetMessageForAction :one` — `WHERE id = $1 AND user_id = $2` (ownership).
- `SetActionStatus :one` — `UPDATE ... SET action_status = $3 WHERE id = $1 AND user_id = $2 AND action_status = 'proposed' RETURNING *` (la condición `proposed` hace la transición atómica; 0 filas = conflicto).

El `pgChatStore.CreatePair` gana una variante `CreatePairWithAction` (misma
transacción: mensaje user normal + mensaje assistant con acción).

## 4. Tools (definiciones para Groq)

Formato OpenAI (`tools: [{type:"function", function:{name, description, parameters}}]`).
Una sola acción por turno (instrucción del prompt + se toma el primer tool call).

| Tool | Parámetros (JSON Schema) | Mapea a |
|------|--------------------------|---------|
| `registrar_checkin` | `mood`, `energy`, `discipline` (int 1–10, required), `note` (string, opcional) | `checkin.Upsert` con `Date: today` |
| `registrar_movimiento` | `type` (`income`\|`expense`, los valores del dominio existente), `amount_centavos` (int > 0), `category` (string), `remark` (opcional) | `finance.Create` con `OccurredOn: today` |
| `marcar_habito` | `habit_id` (string UUID, de la lista del contexto) | `habits.SetCheck(done=true, day=today)` |
| `actualizar_meta` | `goal_id` (string UUID, del contexto), `progress` (int 0–100) | `goals.Patch{Progress}` |

Las descripciones de los tools (en español) instruyen al modelo a usar los IDs
del contexto y a no inventar valores que el usuario no dio.

## 5. Backend (paquete `ai`)

### 5.1 Contexto ampliado (`chatcontext.go`)
Dos interfaces nuevas (mismo patrón que `cycler`/`checkinLister`):
`habitLister` (porción de `habits.Service.List(userID, archived=false, today)`)
y `goalLister` (porción de `goals.Service.List(userID, status="activa", today)`).
El JSON de contexto gana `"habits": [{id, name, done_today...}]` y
`"goals": [{id, title, progress, status}]` (las vistas de dominio existentes ya
serializan esos campos). `NewChatContextBuilder` pasa de 3 a 5 dependencias;
wiring en `server.go` y `handler_test.go`.

### 5.2 Cliente Groq (`groq.go`)
`ChatStream` gana un parámetro de tools y un resultado de tool call:

```go
type ToolCall struct {
    Name      string
    Arguments string // JSON crudo del modelo
}
func (c *GroqClient) ChatStream(ctx, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, *ToolCall, error)
```
- `tools` vacío → comportamiento actual (request sin campo `tools`).
- Acumula fragmentos de `choices[0].delta.tool_calls[0]` (name llega una vez,
  `arguments` llega fragmentado) además de los deltas de texto.
- Devuelve `(texto, toolCall, nil)`: toolCall nil en turnos de solo-texto. Un
  turno con tool call puede traer texto vacío.
- Reglas de éxito: `[DONE]` visto y (texto no vacío **o** toolCall completo).
- La interfaz `chatStreamer` y el call site del camino sin tools se actualizan
  (la firma cambia; el fake de tests también).

### 5.3 Prompt (`chatprompt.go`)
El system prompt gana un bloque: cuándo proponer una acción (solo si el usuario
pide registrar/marcar/actualizar algo explícitamente), una sola por turno, usar
IDs del contexto, nunca asumir montos/valores no dichos, y que la acción será
confirmada por el usuario antes de ejecutarse.

### 5.4 Servicio (`chat.go` + `actions.go` nuevo)
`SendStream` pasa las tools y maneja el resultado:
- Solo texto → flujo actual (`CreatePair`).
- Tool call → valida `Name` contra los 4 kinds y parsea `Arguments` al payload
  tipado del kind (structs en `actions.go` con tags JSON). Tool desconocido o
  JSON inválido → `ErrUnavailable` (nada se persiste, como un fallo de Groq).
  Si es válido → `CreatePairWithAction(user, assistantContent, kind, payload,
  "proposed")`. El `assistantContent` es el texto del modelo o, si vino vacío,
  un resumen generado determinísticamente (p. ej. «Propongo registrar tu
  check-in de hoy: ánimo 8, energía 6, disciplina 9»).

`actions.go` define además el ejecutor:

```go
type actionExecutor struct { checkin checkinUpserter; finance txCreator; habits habitChecker; goals goalPatcher }
func (e *actionExecutor) execute(ctx, userID uuid.UUID, kind string, payload json.RawMessage, today time.Time) error
```
Interfaces estrechas sobre los 4 servicios (testeable con fakes). La ejecución
re-parsea y re-valida el payload (rangos, type, UUIDs) y delega; los servicios
ya validan ownership y reglas de dominio.

`ChatService` gana:
```go
func (s *ChatService) ConfirmAction(ctx, userID, messageID uuid.UUID, today time.Time) (*Message, error)
func (s *ChatService) CancelAction(ctx, userID, messageID uuid.UUID) (*Message, error)
```
`ConfirmAction`: lee el mensaje (ownership), exige `proposed`, ejecuta, y solo
ante éxito hace `SetActionStatus('done')`. `CancelAction`: `SetActionStatus('cancelled')`.

### 5.5 Vista `Message` (`types.go`)
```go
type ActionView struct {
    Kind    string          `json:"kind"`
    Payload json.RawMessage `json:"payload"`
    Status  string          `json:"status"`
}
type Message struct {
    ID        string      `json:"id"`
    Role      string      `json:"role"`
    Content   string      `json:"content"`
    Action    *ActionView `json:"action,omitempty"`
    CreatedAt time.Time   `json:"created_at"`
}
```
(`ID` es nuevo también para mensajes sin acción; `GET /ai/messages` lo incluye.)

## 6. API

- `POST /ai/chat/stream` — sin cambios de contrato salvo que `done.reply`
  ahora trae `id` y, si aplica, `action`. (`POST /ai/chat` no propone acciones.)
- `POST /api/v1/ai/actions/{id}/confirm` → `200 {"message": Message}` con
  status `done`. Errores: `404` (no existe / no es tuyo / no tiene acción),
  `409` (no está `proposed`), `400` (payload inválido o el servicio de dominio
  rechaza, p. ej. progress fuera de rango; la acción queda `proposed`), `500`.
- `POST /api/v1/ai/actions/{id}/cancel` → `200 {"message": Message}` con
  status `cancelled`. Errores: `404`, `409`.

Ambos bajo `RequireAuth`, montados en `Routes` del paquete `ai`.

## 7. Frontend

- **`web/src/lib/ai.ts`:** `Message` gana `id` y `action?: {kind, payload, status}`.
  Funciones nuevas `confirmAction(id): Promise<Message>` y
  `cancelAction(id): Promise<Message>` (POST vía `apiFetch`).
- **`/asistente`:** si un mensaje del asistente trae `action`, la burbuja
  renderiza una tarjeta: título por kind («Check-in de hoy», «Movimiento»,
  «Hábito», «Meta»), los campos del payload legibles (montos en pesos, no
  centavos), y según `status`:
  - `proposed` → botones **Confirmar** / **Cancelar** (deshabilitados mientras
    la mutación corre).
  - `done` → marca «✓ Hecha».
  - `cancelled` → marca «Cancelada» atenuada.
  Confirmar/cancelar actualizan el mensaje en el caché (`setQueryData` sobre
  `["ai-messages"]`, reemplazando por `id`). Confirmar invalida además las
  queries del módulo afectado si están en caché (mínimo: ninguna otra vista
  está montada en `/asistente`; basta el caché del chat — YAGNI).
- El flujo de streaming no cambia: tras `done`, si el reply trae `action`, la
  tarjeta aparece como parte del mensaje.

## 8. Manejo de errores

| Caso | Respuesta | UX |
|------|-----------|-----|
| Modelo propone tool desconocido / args no parseables | `503` HTTP normal si no hubo deltas (caso típico: un turno de tool call no emite texto), o `event: error` si los hubo; nada persistido | error inline + reintento |
| Confirmar acción ya `done`/`cancelled` | `409` | tarjeta se refresca al estado real |
| Payload inválido al ejecutar (rangos, UUID, ownership) | `400`, acción sigue `proposed` | error inline; se puede cancelar |
| Fallo de DB al ejecutar | `500`, acción sigue `proposed` | error inline + reintento |
| Confirmar/cancelar sin token | `401` | redirige a login |

Nota de concurrencia: `SetActionStatus` exige `action_status='proposed'` en el
`WHERE`, así un doble click o dos pestañas no ejecutan dos veces (la segunda
recibe 409). La ejecución ocurre **antes** del update a `done`; si el update
fallara tras ejecutar (ventana mínima), el peor caso es una tarjeta `proposed`
de una acción ya aplicada — re-confirmar es idempotente para checkin (Upsert) y
hábito/meta (set absoluto), y duplicaría solo un movimiento financiero; se
acepta el riesgo (ventana ínfima, uso personal).

## 9. Testing

- **Migración/queries:** round-trip `CreatePairWithAction` + `SetActionStatus`
  (transiciones válidas e inválidas, scoping por usuario).
- **Cliente:** `ChatStream` con tools contra `httptest.Server` — request
  incluye `tools`; stream de tool_call fragmentado se acumula (name +
  arguments en varios chunks); turno de solo-texto devuelve toolCall nil;
  corte a medias falla.
- **Contexto:** incluye habits y goals con IDs.
- **Servicio:** tool call válido persiste par con `proposed`; tool desconocido
  / args inválidos → `ErrUnavailable` sin persistir; `ConfirmAction` ejecuta
  (fakes de los 4 servicios) y transiciona; `CancelAction`; doble confirm →
  conflicto; payload con valores fuera de rango → error y sigue `proposed`.
- **Handler:** `done.reply` con `action` en turno de tool call; confirm/cancel
  felices; 404/409/400/401.
- **Frontend lib:** confirm/cancel hacen el POST correcto y devuelven el
  mensaje.
- **Página:** tarjeta `proposed` muestra botones; confirmar la pasa a «✓
  Hecha» (mock); cancelar a «Cancelada»; mensajes sin acción no muestran
  tarjeta.
- **E2E (smoke docker, clave real):** pedir un check-in por chat → llega
  `done` con `action proposed` → confirm → `GET /checkins` (o el endpoint del
  módulo) muestra el dato escrito → segundo confirm → 409.

## 10. Criterios de aceptación

- «registra mi check-in: ánimo 8, energía 6, disciplina 9» produce una tarjeta
  con esos valores; Confirmar crea el check-in real; la tarjeta queda «✓ Hecha»
  y sobrevive recargas.
- Las cuatro acciones funcionan end-to-end con confirmación.
- Cancelar no escribe nada. Nada se escribe jamás sin confirmar.
- Preguntas normales (sin intención de acción) siguen streameando texto como
  en la rebanada 10.
- `make`-equivalentes en verde (vet + tests backend, Vitest + build frontend)
  y smoke E2E de acción confirmada OK.
