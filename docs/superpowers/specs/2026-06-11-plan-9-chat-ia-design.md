# Plan 9 — Asistente IA on-demand (chat) — Diseño

**Fecha:** 2026-06-11
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Completar el asistente de IA del diseño original (§5.2, §7.8): un **chat conversacional on-demand** donde el usuario pregunta sobre sus datos ("¿cómo voy en junio?", "¿qué hábito estoy descuidando?") y la IA responde con su contexto real. La rebanada 8 entregó el insight proactivo; ésta entrega la otra mitad.

**Decisiones de alcance (acordadas en brainstorming):**
- **Read-only:** la IA solo responde; no modifica datos ni ejecuta acciones (YAGNI; tool-use queda para una rebanada futura).
- **Multi-turno + persistido:** la IA recuerda los mensajes previos y el historial sobrevive a recargas (tabla nueva `ai_messages`).
- **Conversación única por usuario:** una sola charla continua por usuario (sin hilos múltiples).
- **Contexto enriquecido:** snapshot del día actual **+** histórico resumido (ciclos financieros + check-ins recientes).
- **Ubicación:** ruta dedicada `/asistente`, accesible desde la nav del TopBar.

**Fuera de alcance:** acciones/tool-use, streaming de tokens, múltiples conversaciones/hilos, búsqueda en el historial, adjuntos.

## 2. Arquitectura

Se extiende el paquete `api/internal/ai` existente (ya montado en `/api/v1/ai` bajo `RequireAuth`). Se reutiliza el cliente Groq agregándole un modo "chat". El servicio de chat compone el contexto desde servicios ya existentes (`dashboard.Service.Snapshot`, `finance.Service.Cycles`, `checkin.Service.List`) detrás de interfaces estrechas, y persiste la conversación en una tabla nueva. El frontend agrega una ruta dedicada con su lib.

Principio de aislamiento: el chat es independiente del insight proactivo; comparten el cliente Groq pero no se acoplan. Toda query sigue scoped por `user_id`.

## 3. Modelo de datos (migración `0008_ai_messages.sql`)

```sql
-- +goose Up
CREATE TABLE ai_messages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,            -- 'user' | 'assistant'
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_messages_role_valid CHECK (role IN ('user','assistant'))
);
CREATE INDEX idx_ai_messages_user_created ON ai_messages (user_id, created_at);
-- +goose Down
DROP TABLE ai_messages;
```

**Queries (`db/queries/ai_messages.sql`):**
- `ListMessages :many` — `WHERE user_id = $1 ORDER BY created_at ASC`. Devuelve el historial completo (uso personal, acotado en la práctica). Se usa para mostrar la charla y, tomando la cola en Go, para el contexto.
- `CreateMessage :one` — `INSERT (user_id, role, content) ... RETURNING *`.

## 4. Backend (paquete `ai`)

### 4.1 Cliente Groq (`groq.go`)
Agregar al `GroqClient` un método de chat que envía el array de mensajes estilo OpenAI:
```go
type ChatMsg struct { Role, Content string }
func (c *GroqClient) Chat(ctx context.Context, system string, history []ChatMsg) (string, error)
```
Construye `messages = [{system}, ...history]`, mismo endpoint `/chat/completions`, mismos parámetros base (temperatura, max_tokens un poco mayor para respuestas conversacionales, p. ej. 400). El insight proactivo (`Complete`) queda intacto.

### 4.2 Constructor de contexto (`chatcontext.go`)
Arma un JSON compacto con el contexto que recibe la IA:
- **Snapshot del día** (reutiliza `dashboard.Service.Snapshot`).
- **Histórico financiero** (reutiliza `finance.Service.Cycles`): lista de ciclos con `cycle_label, income, expense, net, status`.
- **Check-ins recientes** (reutiliza `checkin.Service.List(limit)`, últimos 14): fecha, mood, energy, discipline.

Define interfaces estrechas para cada dependencia (testeable con fakes). Salida: `string` JSON listo para incrustar en el system prompt.

### 4.3 Prompt de chat (`chatprompt.go`)
`buildChatSystemPrompt(contextJSON string) string`: instruye a la IA a responder en **español**, como coach cálido y conciso, usando **solo** los datos provistos, y a decir explícitamente cuando un dato no está disponible (no inventar). Incrusta el JSON de contexto.

### 4.4 Servicio de chat (`chat.go`)
```go
type ChatService struct { /* ctxBuilder, store (messages), groq chatCompleter, hasKey bool */ }
func (s *ChatService) History(ctx, userID) ([]Message, error)
func (s *ChatService) Send(ctx, userID, text) (*Message, error)
```
Flujo de `Send`:
1. (Validación de longitud se hace en el handler/validator.)
2. Si `!hasKey` → error degradado (`ErrUnavailable`).
3. Construir contexto (snapshot + histórico). Propaga errores reales (DB) como error interno.
4. Cargar historial (`ListMessages`, tomar los últimos ~10 como `[]ChatMsg`), agregar el mensaje nuevo del usuario al final.
5. Llamar `groq.Chat(system, history)`. Si falla → `ErrUnavailable` (no persiste nada).
6. Solo ante éxito: `CreateMessage(user)` y `CreateMessage(assistant)`; devolver el mensaje del asistente.

`History` simplemente devuelve `ListMessages` mapeado a la vista.

> No persistir ante fallo evita mensajes de usuario huérfanos (mismo criterio que el insight de la rebanada 8).

### 4.5 Tipos (`types.go`, ampliación)
```go
type Message struct {
    Role      string    `json:"role"`
    Content   string    `json:"content"`
    CreatedAt time.Time `json:"created_at"`
}
```

## 5. API

Montadas en `Routes` del paquete `ai` (bajo `RequireAuth`):
- `GET /api/v1/ai/messages` → `{ "messages": [ {role, content, created_at}, ... ] }`.
- `POST /api/v1/ai/chat` body `{ "message": "..." }` → `200 { "reply": {role, content, created_at} }`.

**Validación** (`POST /ai/chat`): `message` requerido, no vacío tras trim, longitud máxima razonable (p. ej. 2000 chars) vía validator.

## 6. Frontend

- **`web/src/lib/ai.ts`** (ampliación): tipos `Message`, funciones `getMessages(): Promise<Message[]>` y `sendMessage(text: string): Promise<Message>`.
- **`web/src/routes/asistente.tsx`**: lista de mensajes (usuario a la derecha, asistente a la izquierda) + caja de input con botón enviar. `useQuery(["ai-messages"])` para el historial; `useMutation` para `sendMessage` que invalida `["ai-messages"]` al éxito. Estado de carga al enviar y estado de error inline con reintento (el texto tecleado no se pierde).
- **Nav:** entrada "Asistente" en el TopBar hacia `/asistente`. La banda de IA del dashboard linkea a `/asistente` (sin cambiar su contenido actual).

## 7. Manejo de errores

| Caso | Respuesta API | UX frontend |
|------|---------------|-------------|
| Sin clave Groq o fallo de IA | `503` (no persiste nada) | error inline + reintento, mensaje preservado |
| `message` vacío / muy largo | `400` | validación inline, no se envía |
| Sin token | `401` (middleware) | redirige a login (patrón existente) |
| Error de DB/Snapshot | `500` | error genérico + reintento |

## 8. Testing

- **Store:** round-trip `CreateMessage`/`ListMessages` (orden ASC, scoping por usuario).
- **Constructor de contexto:** compone snapshot + ciclos + check-ins en el JSON esperado (con fakes de los servicios).
- **Prompt de chat:** incluye instrucción de español/solo-datos y el JSON de contexto.
- **Servicio:** éxito persiste pregunta+respuesta y devuelve la del asistente; fallo de Groq no persiste; sin clave degrada (`ErrUnavailable`); historial multi-turno se pasa a Groq.
- **Handler (integración con DB + fake completer):** `POST /ai/chat` feliz; `GET /ai/messages` devuelve lo persistido en orden; auth 401; validación 400; degradación 503.
- **Frontend:** lib (`getMessages` hace GET correcto; `sendMessage` hace POST con el body correcto); ruta (renderiza historial, enviar agrega el par de mensajes, estado de error no rompe la página).

## 9. Criterios de aceptación

- Con clave Groq: el usuario abre `/asistente`, ve su historial, escribe una pregunta y recibe una respuesta coherente con sus datos; al recargar, la conversación sigue ahí.
- La IA usa contexto enriquecido (responde sobre el ciclo actual y meses anteriores).
- Sin clave / IA caída: el chat degrada con error claro y reintento, sin perder el mensaje ni romper la app.
- `make check` (backend) y la suite Vitest + build (frontend) en verde; smoke E2E del camino feliz y/o degradado OK.
