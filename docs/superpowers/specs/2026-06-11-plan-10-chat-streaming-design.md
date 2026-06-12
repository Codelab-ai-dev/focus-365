# Plan 10 — Streaming de tokens en el chat IA — Diseño

**Fecha:** 2026-06-11
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

La respuesta del asistente en `/asistente` aparece **palabra por palabra** en vez
de esperar la respuesta completa, mejorando la velocidad percibida del chat
(rebanada 9). Groq ya soporta streaming estilo OpenAI (`"stream": true` →
chunks SSE); el backend lo re-emite al navegador y el frontend pinta los deltas
en vivo.

**Decisiones de alcance (acordadas en brainstorming):**
- **Transporte:** SSE sobre un POST con `fetch` + `ReadableStream` (sin
  `EventSource`, que no soporta POST; sin WebSocket, sobredimensionado).
- **Corte a medias:** se mantiene la semántica «persistir solo ante éxito». Si
  el stream de Groq falla a mitad de la respuesta, **no se guarda nada**; el
  frontend descarta el parcial y muestra el error habitual con reintento.
- **Endpoint nuevo** `POST /ai/chat/stream`; el `POST /ai/chat` existente queda
  intacto (fallback y compatibilidad con tests).
- **Solo el chat:** el insight proactivo del dashboard (rebanada 8) no streamea
  (es una frase corta y cacheada por día).

**Fuera de alcance:** acciones/tool-use, múltiples hilos, búsqueda en el
historial, adjuntos, reanudación de un stream cortado.

## 2. Arquitectura

Se extiende el paquete `api/internal/ai` con un camino de streaming paralelo al
existente, capa por capa: `GroqClient.ChatStream` (cliente), interfaz
`chatStreamer` + `ChatService.SendStream` (servicio) y `handleChatStream`
(handler SSE con `http.Flusher`). Nada del camino bloqueante cambia. En el
frontend, `sendMessageStream` en la lib y estado de streaming en la página.

nginx (contenedor web) bufferea las respuestas del proxy por defecto; el
handler envía el header `X-Accel-Buffering: no`, que nginx respeta por
respuesta, así no se toca `nginx.conf`. El proxy de vite en dev streamea sin
config extra.

## 3. Contrato HTTP

`POST /api/v1/ai/chat/stream` — mismo body (`{"message":"..."}`), misma
validación (auth, trim, ≤2000 runes) y mismos errores previos que `/ai/chat`:
`400`, `401` y `503` (sin clave) son respuestas HTTP normales **antes** de
empezar el stream.

Si la validación pasa y hay clave, responde `200` con
`Content-Type: text/event-stream`, `Cache-Control: no-cache` y
`X-Accel-Buffering: no`, y emite:

```
event: delta
data: {"text":"palabra "}        ← n veces, en orden

event: done
data: {"reply":{"role":"assistant","content":"...","created_at":"..."}}   ← 1 vez, par ya persistido

event: error
data: {"error":"asistente no disponible por ahora"}   ← solo si Groq falla ya iniciado el stream; cierra sin `done`
```

Tras `done` o `error` el servidor cierra la respuesta. `error` implica que no
se persistió nada.

## 4. Backend (paquete `ai`)

### 4.1 Cliente Groq (`groq.go`)
```go
func (c *GroqClient) ChatStream(ctx context.Context, system string, history []ChatMsg, onDelta func(string)) (string, error)
```
- Mismo armado de mensajes que `Chat`, request con `"stream": true`.
- Lee el body línea a línea (`bufio.Scanner`), parsea los chunks SSE de Groq
  (`data: {...}` con `choices[0].delta.content`, fin con `data: [DONE]`),
  invoca `onDelta(texto)` por cada delta no vacío y devuelve el contenido
  completo acumulado.
- Si el stream se corta antes de `[DONE]` (error de red, status no-2xx,
  scanner err) → devuelve error.
- **Cliente HTTP propio para streaming** con timeout total de 60s: el
  `http.Client` actual (10s) cubre la lectura completa del body y quedaría
  justo para un stream. El camino bloqueante conserva su cliente de 10s.

### 4.2 Servicio (`chat.go`)
```go
type chatStreamer interface {
    ChatStream(ctx context.Context, system string, history []ChatMsg, onDelta func(string)) (string, error)
}
func (s *ChatService) SendStream(ctx context.Context, userID uuid.UUID, text string, today time.Time, onDelta func(string)) (*Message, error)
```
- Misma orquestación que `Send`: sin clave → `ErrUnavailable`; contexto
  (`ctxb.build`) → historial (`ListMessages` + cola de 10) → Groq → persistir.
- Re-emite los deltas de Groq vía `onDelta`.
- **Persiste el par (`CreatePair`, atómico) solo si `ChatStream` devolvió sin
  error.** Fallo de Groq (incluido corte a medias) → `ErrUnavailable`, nada
  persistido. Error de DB al persistir → error interno (el handler emite
  `event: error` genérico).
- `ChatService` recibe el streamer por inyección (el `GroqClient` real
  implementa ambas interfaces; los tests usan fakes).

### 4.3 Handler (`handler.go`)
`handleChatStream(chat *ChatService)`:
- Reusa la validación de `handleChat` (extraer a un helper para no duplicar):
  auth → decode → trim → runes. Errores → respuestas HTTP normales.
- `!hasKey` se detecta llamando a `SendStream` **antes de escribir headers
  SSE**: el primer delta dispara la escritura de headers + status 200. Si
  `SendStream` falla sin haber emitido ningún delta y el error es
  `ErrUnavailable` con cero bytes escritos → `503` normal; si ya se emitieron
  deltas → `event: error`.
- Cada delta se escribe como evento SSE y se hace `Flush()` (vía
  `http.Flusher`; si el ResponseWriter no lo soporta, error 500 antes de
  empezar).
- Éxito → `event: done` con el reply persistido y cierre.
- Ruta montada en `Routes`: `r.Post("/chat/stream", handleChatStream(chat))`.

## 5. Frontend

### 5.1 Lib (`web/src/lib/ai.ts`)
```ts
export function sendMessageStream(
  message: string,
  onDelta: (text: string) => void
): Promise<Message>
```
- `fetch("/api/v1/ai/chat/stream", {method:"POST", body, headers})` con
  `Authorization: Bearer` (reusa `getAccessToken()`) y `credentials:"include"`
  — no puede reusar `apiFetch` (que hace `res.json()`), pero replica sus
  convenciones.
- HTTP no-ok → rechaza con `ApiError(message, status)` leyendo el body JSON
  (mismos mensajes que hoy).
- 200 → lee `res.body.getReader()` decodificando UTF-8, parsea eventos SSE
  separados por línea en blanco (`event:` + `data:`), acumula buffer parcial
  entre chunks:
  - `delta` → `onDelta(text)`.
  - `done` → resuelve con el `reply`.
  - `error` → rechaza con `ApiError(error, 503)`.
  - stream cerrado sin `done` ni `error` → rechaza (`ApiError` genérico).

### 5.2 Página (`web/src/routes/asistente.tsx`)
- Estado nuevo: `streaming: {question: string, partial: string} | null`.
- Al enviar: la mutación usa `sendMessageStream`; se muestra la burbuja del
  usuario optimista (desde `streaming.question`) y la del asistente creciendo
  con `partial` (mientras `partial === ""` se muestra el "Pensando…" actual).
- `onSuccess`: limpia `streaming` y el input, invalida `["ai-messages"]` (igual
  que hoy; el par persistido reemplaza las burbujas optimistas).
- `onError`: limpia `streaming` (descarta el parcial), muestra el error inline
  actual y conserva el texto del input para reintentar.

## 6. Manejo de errores

| Caso | Respuesta | UX frontend |
|------|-----------|-------------|
| Sin clave Groq | `503` HTTP normal (sin SSE) | error inline + reintento, texto preservado |
| Groq falla antes del primer delta (cero bytes escritos) | `503` HTTP normal (sin SSE) | ídem |
| `message` vacío / >2000 runes | `400` | igual que hoy |
| Sin token | `401` (middleware) | redirige a login |
| Groq falla a medias del stream | `event: error`, nada persistido | parcial descartado, error inline + reintento |
| Error de DB al persistir | `event: error` genérico | ídem |
| Stream cortado sin evento final | promesa rechazada en la lib | ídem |

## 7. Testing

- **Cliente (`groq_test.go`):** `ChatStream` contra `httptest.Server` que emite
  chunks SSE de Groq → deltas en orden + contenido completo; corte antes de
  `[DONE]` → error; status no-2xx → error.
- **Servicio (`chat_test.go`):** con fake `chatStreamer` — éxito re-emite
  deltas y persiste el par; fallo a medias no persiste y devuelve
  `ErrUnavailable`; sin clave degrada sin llamar a Groq.
- **Handler (`chat_handler_test.go`):** `POST /ai/chat/stream` feliz → headers
  SSE (`text/event-stream`, `X-Accel-Buffering: no`) y secuencia
  `delta...done` con el par en DB; fallo de Groq → `event: error` y DB vacía;
  validación 400 / sin token 401 / sin clave 503 como HTTP normal.
- **Lib frontend:** `sendMessageStream` con `Response` cuyo body es un
  `ReadableStream` de prueba — deltas acumulados vía `onDelta`, `done`
  resuelve con el reply, `error` rechaza, cierre sin evento final rechaza,
  evento partido entre dos chunks se reensambla.
- **Página:** la burbuja del asistente crece con los deltas; ante error no
  queda texto fantasma y el input conserva el mensaje.
- **E2E:** smoke con `curl -N` contra docker (clave real): eventos `delta` +
  `done`, y el par queda en el historial (`GET /ai/messages`).

## 8. Criterios de aceptación

- Con clave Groq: en `/asistente` la respuesta aparece progresivamente
  (primer texto visible antes de que termine la respuesta completa) y al
  recargar la conversación persiste igual que antes.
- Si el stream falla a medias: el parcial desaparece, hay error claro con
  reintento y el historial no contiene mensajes a medias.
- `POST /ai/chat` (no-stream) sigue funcionando sin cambios.
- `make check` (backend) y Vitest + build estricto (frontend) en verde; smoke
  E2E del streaming OK en docker (a través del proxy nginx, no directo al api).
