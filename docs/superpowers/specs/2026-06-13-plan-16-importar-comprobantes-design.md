# Plan 16 — Subir comprobantes a Finanzas (la IA extrae los movimientos) — Diseño

**Fecha:** 2026-06-13
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

En la página de **Finanzas**, el usuario sube una **foto (jpg/png), CSV o
PDF**; la IA extrae los movimientos y aparecen como **tarjetas de acción** (las
mismas `movimiento` de la R11/R15) ahí mismo, para confirmar uno a uno o todos.
El archivo se **procesa y se descarta** (no se almacena). Reemplaza al
importador de la app externa (descartado por su costo mensual): aquí la captura
sigue siendo manual/asistida, sin nuevas suscripciones (usa la clave Groq ya
configurada).

**Decisiones (brainstorming con mockups):**
- **Tres tipos:** imagen, CSV y PDF (este vía extracción de texto; escaneados
  sin texto → se pide subir como foto).
- **Confirmación:** tarjetas de acción (reusa R15), con botón «Confirmar todos».
- **Almacenamiento:** procesar y descartar.
- **Ubicación:** en Finanzas; las tarjetas se renderizan ahí (no en el chat).
- **Fecha:** la detectada en el documento; si no hay, hoy.

**Fuera de alcance:** guardar el archivo original, editar un movimiento antes de
confirmar (se cancela y se re-sube), OCR de PDF escaneado, otros módulos que no
sean finanzas.

## 2. Modelo de datos (migración `0012_ai_actions_source.sql`)

Las acciones hoy cuelgan de un mensaje del chat (`ai_actions.message_id NOT
NULL`). Para que existan acciones nacidas de una subida (sin conversación):

```sql
-- +goose Up
ALTER TABLE ai_actions ALTER COLUMN message_id DROP NOT NULL;
ALTER TABLE ai_actions
    ADD COLUMN source TEXT NOT NULL DEFAULT 'chat'
        CHECK (source IN ('chat','upload'));
-- +goose Down
ALTER TABLE ai_actions DROP COLUMN source;
-- (message_id se deja nullable en el Down: revertir a NOT NULL fallaría si hay
--  filas con message_id NULL; aceptable para un Down.)
```

Las extracciones se insertan con `source='upload'`, `message_id=NULL`,
`status='proposed'`. El historial del chat (`ListActionsByMessages`) sigue
filtrando por `message_id`, así que las acciones de upload **no** contaminan el
chat.

**`movimientoPayload` gana `OccurredOn string` opcional** (`occurred_on`,
YYYY-MM-DD). `parseActionPayload("movimiento")` lo valida si viene
(`time.Parse`). El ejecutor usa esa fecha si está, o `today` si está vacía
(retrocompatible: los movimientos del chat no la traen → hoy). El tool del chat
`registrar_movimiento` **no** cambia (sigue sin fecha; los uploads arman el
payload server-side).

## 3. Queries y store (`db/queries/ai_actions.sql`, `chatstore.go`)

- `CreateUploadAction :one` — INSERT con `message_id = NULL`, `source='upload'`,
  `status='proposed'`. (O reusar `CreateAction` con message_id nullable — el
  plan decide según lo que genere sqlc.)
- `ListPendingUploadActions :many` — `WHERE user_id=$1 AND source='upload' AND
  status='proposed' ORDER BY created_at`.
- Métodos nuevos en el store: `CreateUploadActions(userID, []ProposedAction)
  []store.AiAction` (transacción) y `ListPendingUploadActions(userID)`.

## 4. Extracción (paquete `ai`: `import.go`, `extract.go`, `pdftext.go`)

`POST /ai/import` (multipart, campo `file`, ≤ **8 MB**). Detecta el tipo por
content-type + extensión y produce un JSON
`{"movimientos":[{type, amount_centavos, category, remark, occurred_on}]}`:

- **Imagen** (image/jpeg, image/png) → cliente de **visión** de Groq: la imagen
  va en base64 dentro del content array estilo OpenAI
  (`[{type:"text",...},{type:"image_url",image_url:{url:"data:...;base64,..."}}]`),
  modelo `GROQ_VISION_MODEL` (env nueva), `response_format: json_object`.
- **CSV** (text/csv, .csv) → `encoding/csv`, se serializa el texto (tope **50
  filas** + cabecera para acotar tokens; si hay más, se trunca y se reporta) y
  se manda al modelo de texto actual con el prompt de extracción + JSON mode.
- **PDF** (application/pdf) → extracción de texto con `github.com/ledongthuc/pdf`
  (pura-Go, compila en distroless). Si hay texto → modelo de texto; si el texto
  es vacío/insignificante → error 422 «el PDF parece escaneado, súbelo como
  foto».

**Validación lenient:** cada movimiento extraído se valida con
`parseActionPayload("movimiento", ...)` (type income/expense, amount > 0,
category no vacía, occurred_on parseable o vacío). Los inválidos **se
descartan** y se cuenta cuántos (la extracción es difusa; no all-or-nothing
como el chat). Cero válidos → 422. Los válidos se crean como acciones
`upload`/`proposed` y se devuelven.

**`GROQ_VISION_MODEL`:** env nueva; el operador la fija al modelo de visión
vigente de Groq (los modelos «preview» cambian). Default a un modelo multimodal
actual de Groq, documentado en `.env.example` y el compose. El cliente Groq gana
`ExtractVision(ctx, system, b64, mime) (string, error)` (vision) y reusa la
ruta de texto para CSV/PDF (método de extracción con max_tokens mayor + JSON
mode).

Respuesta de `POST /ai/import`:
`200 {"created": [ActionView...], "dropped": N, "truncated": bool}`.

`GET /ai/import/pending` → `{"actions": [ActionView...]}` (las `upload`
`proposed` del usuario).

## 5. API

Montados bajo `RequireAuth` en `Routes` del paquete `ai`:
- `POST /api/v1/ai/import` (multipart) — extrae y crea acciones upload.
- `GET /api/v1/ai/import/pending` — lista las pendientes.
- `POST /api/v1/ai/actions/{id}/confirm|cancel|undo` — **sin cambios**, reusados.

## 6. Frontend

- **`web/src/ui/ActionCard.tsx`** (refactor): se extrae la `ActionCard` y los
  helpers `ACTION_TITLES`/`actionDetails` desde `asistente.tsx` a `ui/` para
  compartirlos entre el chat y Finanzas, sin cambiar comportamiento ni textos.
  `asistente.tsx` importa de ahí.
- **`web/src/lib/finances.ts`** (o `ai.ts`): `importFile(file): Promise<{created:
  Action[]; dropped: number; truncated: boolean}>` (multipart) y
  `getPendingUploads(): Promise<Action[]>`.
- **Finanzas (`finanzas.tsx`)** gana una `Card` de subida: input
  `accept="image/*,.csv,.pdf"` (drop zone), estado de carga, y la lista de
  tarjetas extraídas (`ActionCard` por cada acción `proposed`) con su
  Confirmar/Cancelar/Deshacer + botón **«Confirmar todos»** (recorre las
  `proposed` llamando `confirmAction` existente). Al montar, `getPendingUploads`
  restaura pendientes. Tras confirmar, se invalida la query de transacciones de
  Finanzas para que el movimiento aparezca en la lista normal. Mensajes de
  error (415/413/422/503) inline.

## 7. Manejo de errores

| Caso | Respuesta | UX |
|------|-----------|-----|
| Sin clave Groq | 503 | banda danger, reintento |
| Tipo no soportado | 415 | «formato no soportado (usa foto, CSV o PDF)» |
| Archivo > 8 MB | 413 | «archivo demasiado grande» |
| PDF escaneado (sin texto) | 422 | «parece escaneado, súbelo como foto» |
| Cero movimientos válidos | 422 | «no pude leer movimientos en el archivo» |
| Fallo de Groq (visión/texto) | 503 | reintento |
| Filas/ítems inválidos parciales | 200 con `dropped: N` | «extraje X (N no se pudieron leer)» |

## 8. Testing

- **Migración:** `source` default 'chat'; insertar acción upload con message_id
  NULL; `ListPendingUploadActions` filtra por source+status; las upload no
  aparecen en `ListActionsByMessages`.
- **occurred_on:** `parseActionPayload` acepta vacío/válido, rechaza
  malformado; ejecutor usa la fecha del payload si viene, hoy si no.
- **Extractores (Groq fakeado):** imagen/CSV/PDF-texto → movimientos; ítems
  inválidos descartados con conteo; PDF sin texto → error; CSV truncado a 50
  filas marca `truncated`.
- **PDF/CSV reales:** archivos de muestra en `testdata/` (un CSV y un PDF de
  texto) para el parser y la extracción de texto (la parte determinista, sin
  Groq).
- **Endpoints:** import feliz crea N acciones upload y devuelve created/dropped;
  pending lista; 415/413/422/503; sin token 401.
- **Frontend:** zona de subida dispara el POST multipart; render de tarjetas;
  «Confirmar todos» recorre; error inline; `ActionCard` extraído sigue pasando
  los tests del chat.
- **E2E producción (cierre):** subir un CSV de muestra → tarjetas → confirmar
  todos → los movimientos aparecen en `GET /finance/transactions`.

## 9. Criterios de aceptación

- Subir una foto de recibo, un CSV o un PDF de texto produce tarjetas de
  movimiento con monto, categoría y fecha correctos; confirmar las registra en
  el ciclo correcto (según la fecha detectada).
- Las acciones de upload no aparecen en el chat; las del chat no aparecen en
  Finanzas.
- PDF escaneado y archivos sin movimientos dan errores claros.
- Las 7 acciones del chat (R11–R15) y el deshacer siguen funcionando.
- Suites completas en verde; smoke de producción del flujo CSV.
