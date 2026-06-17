# Plan 21 — Búsqueda en el chat — Diseño

**Fecha:** 2026-06-16
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

Buscar dentro del asistente. Una barra de búsqueda arriba de la lista de hilos
(`/asistente`); al escribir (≥2 caracteres, con debounce), los resultados
**reemplazan** la lista. Se apoya en los hilos de la R20.

**Decisiones (brainstorming):**
- **Dos secciones de resultados:** arriba los **hilos cuyo título coincide**
  (una fila por hilo); abajo los **mensajes cuyo contenido coincide** (tuyos o
  del asistente), cada uno con el hilo al que pertenece. Tocar cualquiera abre
  el hilo.
- **Motor: subcadena insensible a mayúsculas y acentos** (`unaccent` + `lower`
  + `LIKE`). Sin ranking, sin stemming. A escala personal alcanza.
- **Al tocar un resultado: abrir el hilo** (`/asistente/$threadId`). Sin salto al
  mensaje exacto.

**Fuera de alcance:** full-text/stemming/ranking; resaltar y scrollear hasta el
mensaje exacto dentro del hilo (el resaltado es solo en el fragmento del
resultado); buscar en acciones, insight o finanzas.

## 2. Backend

### Migración `0018_unaccent.sql`
```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS unaccent;
-- +goose Down
-- (no se elimina la extensión: puede usarla otra cosa)
```
Sin tabla ni índice nuevos. A escala personal el filtro corre como seq scan sin
problema. (`unaccent()` es STABLE, no IMMUTABLE, por eso no se indexa; sólo se
usa en el `WHERE`.)

### Queries (`api/db/queries/ai_search.sql`)
```sql
-- name: SearchThreadsByTitle :many
SELECT t.id, t.title, t.updated_at,
       COALESCE(lm.content, '') AS preview
FROM ai_threads t
LEFT JOIN LATERAL (
    SELECT content FROM ai_messages m
    WHERE m.thread_id = t.id
    ORDER BY m.created_at DESC
    LIMIT 1
) lm ON true
WHERE t.user_id = $1
  AND unaccent(lower(t.title)) LIKE '%' || unaccent(lower($2)) || '%'
ORDER BY t.updated_at DESC
LIMIT $3;

-- name: SearchMessages :many
SELECT m.id, m.thread_id, m.role, m.content, m.created_at,
       t.title AS thread_title
FROM ai_messages m
JOIN ai_threads t ON t.id = m.thread_id
WHERE m.user_id = $1
  AND unaccent(lower(m.content)) LIKE '%' || unaccent(lower($2)) || '%'
ORDER BY m.created_at DESC
LIMIT $3;
```
El `$2` que llega ya viene con los comodines escapados (ver abajo); el `'%'||..||'%'`
agrega los comodines de envoltura reales.

### Escape de comodines
El término del usuario se escapa en Go antes de pasarlo a la query: `\` → `\\`,
`%` → `\%`, `_` → `\_`. Así `50%` busca el literal "50%" y no "50<cualquier
cosa>". (Postgres `LIKE` usa `\` como escape por defecto.)

### Servicio y endpoint
- `ChatService.Search(ctx, userID, query string, limit int) (SearchResults, error)`
  donde `SearchResults{ Threads []ThreadHitView; Messages []MessageHitView }`.
  Escapa el término, corre ambas queries, mapea a vistas. Ownership garantizado
  por `user_id` en el `WHERE`.
- `GET /ai/search?q=<term>` → `{ "threads": [...], "messages": [...] }`. Valida
  `q` con trim y **≥2 runes**; si no, `400` "término demasiado corto". Límites:
  **20 hilos, 50 mensajes**.

### Vistas (JSON)
```
ThreadHitView  { id, title, preview, updated_at }
MessageHitView { id, thread_id, thread_title, role, content, created_at }
```

## 3. Frontend

- `/asistente` (lista de hilos) gana un `<input>` de búsqueda arriba, con estado
  local `query` y **debounce ~250ms** (un pequeño hook o `setTimeout`).
- Con `query.trim().length >= 2`: `useQuery(["ai-search", q], () =>
  searchChat(q))` y se renderiza la **vista de resultados** en lugar de la
  lista de hilos:
  - Sección **«Hilos»** (si hay): fila por hilo (título + preview + fecha), tap
    → `/asistente/$threadId`.
  - Sección **«Mensajes»** (si hay): fila por mensaje (título del hilo en chico,
    «Vos»/«Asistente», fragmento con el término resaltado en `<mark>`, fecha),
    tap → `/asistente/$threadId`.
  - Si ambas vacías → «Sin resultados para «q»».
- Con el término vacío/<2 → la lista de hilos normal (R20).
- **Resaltado:** best-effort, case-insensitive sobre el literal buscado; si por
  acentos no se encuentra la subcadena exacta, se muestra el fragmento sin
  `<mark>` (el resaltado es una ayuda, no crítico).
- **lib `ai.ts`:** `searchChat(q): Promise<SearchResults>` con
  `type SearchResults = { threads: ThreadHit[]; messages: MessageHit[] }`.

## 4. Manejo de errores

- `q` vacío o <2 runes → el front no dispara la query; el backend, si lo llaman,
  responde 400.
- Comodines `% _ \` escapados server-side → se buscan literalmente.
- Cero resultados → estado vacío claro (no error).
- Resultados solo del usuario autenticado (filtro `user_id`).

## 5. Testing

- **Backend (store/servicio):** encuentra por subcadena en contenido; insensible
  a mayúsculas y acentos (`gaste` ↔ `gasté`); match por título devuelve el hilo
  (una fila, no sus mensajes); scopeado al usuario (sin fuga cruzada); ordena por
  reciente; respeta los límites; `%`/`_` tratados literalmente (un mensaje con
  "50%" se encuentra buscando "50%" y no con "50x"); `q` corto → 400.
- **Frontend:** escribir ≥2 muestra resultados (hilos y mensajes); tocar un
  resultado navega al hilo; borrar el término vuelve a la lista de hilos; estado
  «sin resultados».
- **E2E producción:** sembrar un par de mensajes/hilos, buscar un término con
  acento y comprobar match insensible; buscar por título; confirmar que no
  aparecen datos de otro usuario.

## 6. Criterios de aceptación

- Buscás un término y ves, en `/asistente`, los hilos cuyo título coincide y los
  mensajes cuyo contenido coincide, insensible a acentos y mayúsculas.
- Tocar un resultado abre su hilo.
- Solo aparecen tus propios hilos y mensajes.
- Suites en verde; smoke de producción OK.
