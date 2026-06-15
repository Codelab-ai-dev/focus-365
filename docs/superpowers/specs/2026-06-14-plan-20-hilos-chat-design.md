# Plan 20 — Hilos en el asistente (chat) — Diseño

**Fecha:** 2026-06-14
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

El chat del asistente hoy es **una sola conversación plana por usuario**
(`ai_messages` por `user_id`, ordenada por fecha). Esta rebanada lo divide en
**hilos** (conversaciones separadas): el usuario puede tener varios temas en
paralelo, cambiar entre ellos, crear, renombrar y borrar hilos. Cada hilo es su
propia conversación de cara al modelo.

Es la **R20**. La **búsqueda** dentro del chat queda para una rebanada
posterior (R21), apoyada sobre esta estructura.

**Decisiones (brainstorming):**
- **Hilos como conversaciones independientes.** El modelo ve solo los mensajes
  de ese hilo como historial conversacional. El snapshot de estado del usuario
  (finanzas, hábitos, compromisos vía `chatcontext`) sigue siendo **global** e
  igual que hoy.
- **Titulado automático del primer mensaje** (recortado ~60 chars). Renombrable
  después. Sin llamada extra a la IA.
- **Creación lazy, sin hilos vacíos.** El botón `[+]` abre una pantalla de chat
  vacía; el hilo se persiste recién al enviar el primer mensaje.
- **Navegación lista → hilo** (patrón mensajería): `/asistente` es la lista de
  hilos; `/asistente/$threadId` es el chat de un hilo.

**Fuera de alcance:** la búsqueda (R21); cambiar el insight proactivo
(`GET /ai/insight`, es por-usuario y no conversacional); el flujo de subida de
archivos a Finanzas (`source='upload'`, acciones sin mensaje, ajenas a hilos).

## 2. Modelo de datos (migración `0017_ai_threads.sql`)

Nueva tabla y FK en mensajes:

```sql
-- +goose Up
CREATE TABLE ai_threads (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_threads_user_updated ON ai_threads (user_id, updated_at DESC);

-- Un hilo "General" por cada usuario que ya tenga mensajes, con las fechas
-- derivadas de su historial.
INSERT INTO ai_threads (user_id, title, created_at, updated_at)
SELECT user_id, 'General', MIN(created_at), MAX(created_at)
FROM ai_messages
GROUP BY user_id;

-- thread_id en los mensajes: primero nullable, backfill, luego NOT NULL.
ALTER TABLE ai_messages ADD COLUMN thread_id UUID REFERENCES ai_threads(id) ON DELETE CASCADE;
UPDATE ai_messages m
SET thread_id = t.id
FROM ai_threads t
WHERE t.user_id = m.user_id;       -- (cada usuario tiene exactamente un hilo en este punto)
ALTER TABLE ai_messages ALTER COLUMN thread_id SET NOT NULL;

DROP INDEX IF EXISTS idx_ai_messages_user_created;
CREATE INDEX idx_ai_messages_thread_created ON ai_messages (thread_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_messages_thread_created;
CREATE INDEX idx_ai_messages_user_created ON ai_messages (user_id, created_at);
ALTER TABLE ai_messages DROP COLUMN thread_id;
DROP TABLE ai_threads;
```

Notas:
- Se conserva `ai_messages.user_id` (sirve para chequear dueño sin un join, y
  para las queries de contexto por-usuario que ya existen). `thread_id` se suma.
- `ai_actions` **no cambia**: su FK `message_id` ya hace cascada vía el mensaje,
  y este vía el hilo. Borrar un hilo borra sus mensajes y sus acciones.
- El `updated_at` del hilo se toca con cada mensaje nuevo → ordena la lista por
  actividad reciente.

## 3. Backend

### Queries (`ai_threads.sql`, ajustes en `ai_messages.sql`)
- `CreateThread(user_id, title)` → fila.
- `ListThreads(user_id)` → hilos del usuario ordenados por `updated_at DESC`,
  con el **preview** del último mensaje (subconsulta lateral sobre
  `ai_messages`).
- `GetThread(id, user_id)` → fila o `ErrNoRows` (ownership).
- `RenameThread(id, user_id, title)` → fila actualizada (solo si es del dueño).
- `DeleteThread(id, user_id)` → borra (cascada). Cuenta filas para distinguir
  404.
- `TouchThread(id)` → `updated_at = now()`.
- `ListMessages` pasa a filtrar por `thread_id` **validando** que el hilo sea del
  usuario (join o chequeo de dueño previo). `CreateMessage` gana `thread_id`.

### Servicio (`ChatService`, `messageStore`)
- `messageStore` gana los métodos de hilos y `ListMessages`/`CreatePair*`
  reciben `threadID`.
- `History` → `HistoryByThread(ctx, userID, threadID)`: valida dueño del hilo
  (404 si no), trae los mensajes del hilo y cuelga sus acciones (igual que hoy).
- `SendStream`/`Send` reciben un `threadID` **opcional**:
  - Si viene un `threadID`, valida que sea del usuario (404 si no) y persiste el
    par ahí; toca `updated_at`.
  - Si **no** viene (hilo nuevo, lazy), crea el hilo con título derivado del
    texto (trim + recorte a 60 runes; si queda vacío, `"Nuevo hilo"`) **dentro
    de la misma transacción** que el par, y devuelve su `id`.
  - El título solo se setea al crear el hilo; un hilo ya titulado no se
    re-titula.
- La cola de historial a Groq (`buildHistory`) ahora son los mensajes **del
  hilo** (mismos `chatHistoryLimit`).

### Endpoints (bajo `/ai`, ya con `RequireAuth`)
- `GET /threads` → `{ threads: [{ id, title, preview, updated_at }] }`.
- `GET /threads/{id}/messages` → `{ messages: [...] }` (con acciones colgadas);
  404 si el hilo no es del usuario.
- `PATCH /threads/{id}` `{ title }` → `{ thread }`; valida no-vacío tras trim y
  recorta a 60 runes; 404 si no es del dueño.
- `DELETE /threads/{id}` → 204; 404 si no es del dueño.
- `POST /chat/stream` (y `POST /chat`): el body gana `thread_id` (opcional, UUID
  o ausente/null). El evento `done` del SSE incluye, además de `reply`, el
  `thread_id` del hilo (el nuevo si fue creación lazy) para que el front
  redirija. `GET /messages` (historial plano global) se **elimina/reemplaza** por
  el endpoint por-hilo.

El `chatcontext` (snapshot de estado por-usuario) **no se toca**.

## 4. Frontend

### Rutas
- `/asistente` → **lista de hilos**. Cada fila: título, preview del último
  mensaje, fecha relativa. Ordenada por actividad. Botón `[+]` → `/asistente/new`.
  Estado vacío: invitación a empezar.
- `/asistente/$threadId` → el chat actual, con los mensajes del hilo. Header con
  título del hilo + acciones **renombrar (✏️)** y **borrar (🗑️)**. Borrar
  devuelve a `/asistente`.
- `/asistente/new` → chat vacío. El primer envío crea el hilo (sin `thread_id`),
  lee el `thread_id` del evento `done` y navega (replace) a `/asistente/$id`.

### `lib/ai.ts`
- Nuevos: `getThreads()`, `getThreadMessages(threadId)`, `renameThread(id,
  title)`, `deleteThread(id)`.
- `sendMessageStream` gana un `threadId?: string` y su resultado incluye el
  `threadId` devuelto por el `done`.
- Las llaves de query de TanStack pasan a `["ai-threads"]` y
  `["ai-thread", threadId, "messages"]`. Tras renombrar/borrar/enviar se
  invalida/actualiza la lista.

### Navegación
- El link del home/banda al asistente sigue apuntando a `/asistente` (la lista).

## 5. Manejo de errores

- Acceder, enviar, renombrar o borrar un hilo ajeno → **404** (sin filtrar
  existencia). El front, ante 404 al abrir un hilo, vuelve a la lista.
- Primer envío de un hilo nuevo que falla (Groq caído, etc.) → no se crea ni
  hilo ni mensajes (todo en una transacción); el texto queda en el input, igual
  que hoy.
- Renombrar con título vacío tras trim → 400.
- Borrar el hilo activo → el front navega a la lista; si la lista queda vacía,
  muestra el estado vacío.

## 6. Testing

- **Migración:** sembrar mensajes de 2 usuarios, migrar; cada usuario queda con
  un hilo "General" con todos sus mensajes y fechas correctas; ningún mensaje
  cruza de usuario.
- **Store/servicio:** CRUD de hilos con ownership (404 cruzado); `ListThreads`
  ordena por `updated_at` y trae el preview correcto; `HistoryByThread` solo
  trae los mensajes del hilo; envío con `thread_id` existente persiste ahí y toca
  `updated_at`; envío sin `thread_id` crea el hilo con título derivado y lo
  devuelve; cascada al borrar elimina mensajes y acciones; renombrar.
- **Handler:** `GET /threads`, `GET /threads/{id}/messages` (200 propio, 404
  ajeno), `PATCH` (200/400/404), `DELETE` (204/404); el `done` del stream lleva
  `thread_id`.
- **Frontend:** la lista renderiza hilos con preview/fecha; `[+]` navega a
  `/new`; enviar en `/new` crea y navega a `/$id`; renombrar refleja el título;
  borrar vuelve a la lista; los mensajes mostrados son los del hilo.
- **E2E producción:** crear dos hilos distintos, comprobar que cada uno mantiene
  su propia conversación; renombrar uno; borrar uno; el historial viejo aparece
  bajo "General".

## 7. Criterios de aceptación

- El asistente tiene múltiples hilos por usuario; cada uno es una conversación
  independiente de cara al modelo.
- El historial previo queda intacto bajo un hilo "General".
- Crear (lazy, auto-titulado), renombrar y borrar hilos funciona; borrar hace
  cascada de mensajes y acciones.
- Ownership estricto (404 cruzado) en todos los endpoints de hilo.
- Suites en verde; smoke de producción OK.
