# Plan 8 — Asistente IA (insight proactivo) — Diseño

**Fecha:** 2026-06-11
**Rebanada:** 8 de 8 del roadmap (`docs/superpowers/specs/2026-06-09-focus-365-design.md` §5, §7).
**Dimensión:** Asistente IA — insight proactivo del día sobre datos reales.

## Objetivo

Activar la **banda de IA** del dashboard (hoy un placeholder estático) con un
**insight proactivo real**: un párrafo corto, cálido y accionable que detecta
patrones del día (racha en riesgo, gasto, ánimo alto = aprovecha) a partir del
snapshot agregado de las 5 dimensiones. El insight se genera con **Groq**
(clave server-side), se cachea **uno por usuario y día**, y degrada de forma
elegante a un placeholder cuando no hay IA disponible.

El **chat on-demand** (segundo modo descrito en el macro §5) queda **fuera de
alcance** de esta rebanada; la tabla deja la puerta abierta (`kind=on_demand`)
sin construirlo ahora.

## Decisiones de diseño (locked)

- **Alcance:** solo el **insight proactivo**. Sin chat on-demand (futuro).
- **Generación:** **perezosa + cache diario**. Al cargar el dashboard, si no hay
  insight de hoy, el backend arma el snapshot, llama a Groq, persiste y lo
  devuelve. Las siguientes cargas del día lo leen de la DB. Sin scheduler.
- **Forma del contenido:** **un párrafo corto (1-3 frases)** en español, texto
  plano. Sin título/detalle ni parsing de estructura (robustez).
- **Degradación:** **banda placeholder de respaldo**. Sin clave configurada o
  ante fallo de Groq, el endpoint responde `200 {available:false}` y la banda
  muestra el texto estático actual. Nunca 500; nunca rompe el dashboard.
- **Arquitectura (Enfoque A):** paquete `ai` que **reutiliza
  `dashboard.Service.Snapshot`** para el contexto (DRY) + cliente Groq detrás de
  una interfaz (`Completer`) para testeo. Sin duplicar la agregación.

## Sección 1 — Modelo de datos (`ai_insights`)

Nueva migración **`api/db/migrations/0007_ai_insights.sql`** (goose), siguiendo
el patrón de §3 del macro con un ajuste para el cache diario:

```sql
-- +goose Up
CREATE TABLE ai_insights (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    insight_date     DATE NOT NULL,                     -- día del insight (cache key)
    kind             TEXT NOT NULL DEFAULT 'proactive', -- proactive|on_demand (futuro)
    content          TEXT NOT NULL,                     -- el párrafo generado
    context_snapshot JSONB NOT NULL,                    -- snapshot enviado a Groq (auditoría)
    generated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_insights_kind_valid  CHECK (kind IN ('proactive','on_demand')),
    CONSTRAINT ai_insights_unique_day  UNIQUE (user_id, insight_date, kind)
);
CREATE INDEX idx_ai_insights_user_date ON ai_insights (user_id, insight_date DESC);

-- +goose Down
DROP TABLE ai_insights;
```

Decisiones:
- **`insight_date DATE`** explícito (no derivado de `generated_at`) → lookup y
  unicidad del cache diario limpios.
- **`UNIQUE (user_id, insight_date, kind)`** hace cumplir "1 por día" en la DB.
- **`context_snapshot JSONB`** guarda el snapshot exacto enviado a Groq
  (auditoría/debug y, a futuro, base para el chat).
- **`kind`** ya admite `on_demand` para no re-migrar cuando llegue el chat; esta
  rebanada solo escribe `proactive`.

Queries sqlc en **`api/db/queries/ai_insights.sql`**:

```sql
-- name: GetInsight :one
SELECT * FROM ai_insights
WHERE user_id = $1 AND insight_date = $2 AND kind = $3;

-- name: CreateInsight :one
INSERT INTO ai_insights (user_id, insight_date, kind, content, context_snapshot)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
```

Tras escribir queries: `GOTOOLCHAIN=local sqlc generate` (desde `api/`) regenera
`internal/store`. Nunca editar el código generado a mano.

## Sección 2 — Backend: config, cliente Groq, servicio, handler

### Config (`api/internal/config/config.go`)

Leer las dos env que **ya están** en `docker-compose.yml` (api):

```go
GroqAPIKey string // GROQ_API_KEY  (vacío permitido → modo degradado)
GroqModel  string // GROQ_MODEL, default "llama-3.3-70b-versatile"
```

Clave vacía **no es error**: habilita el modo degradado. `GroqModel` cae al
default si la env está vacía.

### Cliente Groq (`api/internal/ai/groq.go`)

Cliente HTTP mínimo contra el endpoint OpenAI-compatible de Groq
(`https://api.groq.com/openai/v1/chat/completions`), detrás de una interfaz:

```go
type Completer interface {
    Complete(ctx context.Context, system, user string) (string, error)
}

type GroqClient struct {
    baseURL string // inyectable; default "https://api.groq.com/openai/v1"
    apiKey  string
    model   string
    http    *http.Client
}

func NewGroqClient(apiKey, model string) *GroqClient
```

- `baseURL` inyectable (un setter o campo) → en tests un `httptest.Server` hace
  de Groq.
- Timeout corto (~10s) en el `http.Client`.
- Request: body `{model, messages:[{role:"system",content},{role:"user",content}], temperature, max_tokens}`,
  header `Authorization: Bearer <apiKey>`.
- Respuesta: extrae `choices[0].message.content`. HTTP no-2xx, body inválido o
  sin choices → `error`.

### Servicio (`api/internal/ai/service.go`)

```go
type Service struct {
    dash   *dashboard.Service
    q      *store.Queries
    groq   Completer
    hasKey bool
}

func NewService(dash *dashboard.Service, q *store.Queries, c Completer, hasKey bool) *Service

func (s *Service) DailyInsight(ctx context.Context, userID uuid.UUID, today time.Time) (*Insight, error)
```

Lógica de `DailyInsight`:

1. **Cache:** `q.GetInsight(userID, today, "proactive")`. Si existe →
   `{Content, GeneratedAt, Available:true}, nil`. (`pgx.ErrNoRows` = sin cache,
   se sigue; cualquier otro error de DB se propaga.)
2. **Sin clave** (`!hasKey`) → `{Available:false}, nil`. No llama, no persiste.
3. **Contexto:** `dash.Snapshot(ctx, userID, today)` → serializar a JSON. Si
   `Snapshot` falla, propagar el error (problema real de datos, no de IA).
4. **Prompt:** `buildPrompt(snapshotJSON)` arma `system` (entrenador cálido,
   español, 1-3 frases, detectar patrones: racha en riesgo, finanzas, ánimo) y
   `user` (el snapshot JSON).
5. `s.groq.Complete(ctx, system, user)`. **Si falla** → `{Available:false}, nil`
   (sin persistir; el dashboard no se rompe).
6. **Éxito** → `q.CreateInsight(userID, today, "proactive", content, snapshotJSON)`
   y devolver `{Content, GeneratedAt, Available:true}, nil`.

Tipo de vista en `api/internal/ai/types.go`:

```go
type Insight struct {
    Content     string    `json:"content"`      // "" cuando !Available
    Available   bool      `json:"available"`
    GeneratedAt time.Time `json:"generated_at"` // zero cuando !Available
}
```

> Nota de serialización: el handler responde `content:null` y `generated_at:null`
> cuando `Available=false` (no `""`/zero). Para lograrlo, el handler construye el
> JSON de respuesta con punteros/`map` cuando `!Available` en vez de serializar
> el struct crudo. El espejo TS lo refleja (`content: string | null`).

### Handler (`api/internal/ai/handler.go`)

`Routes(svc *Service) http.Handler` con `r.Get("/insight", handleInsight(svc))`:

- `GET /api/v1/ai/insight?today=YYYY-MM-DD` bajo `RequireAuth`, scopeado por el
  `user_id` del contexto (`auth.UserIDFromContext` → 401 defensivo si falta).
- `parseTodayParam(r)` idéntico al de dashboard/metas: `?today=` o UTC midnight.
- **Nunca 500 por fallo de IA:** `DailyInsight` ya degrada a `Available:false`.
  Solo propaga 500 si `Snapshot`/DB fallan (problema real). 401 sin token.
- Respuesta `200`:
  - disponible: `{ "content": "…", "available": true, "generated_at": "…" }`
  - degradado: `{ "content": null, "available": false, "generated_at": null }`

### Montaje (`api/internal/server/server.go`)

Agregar `GroqAPIKey`/`GroqModel` a `Deps` y propagarlos desde `cmd/server`
(que ya carga `config`). Dentro del grupo `RequireAuth`, tras el dashboard:

```go
groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel)
aiSvc := ai.NewService(dashboardSvc, q, groq, d.GroqAPIKey != "")
r.Mount("/ai", ai.Routes(aiSvc))
```

`q` (el `*store.Queries`) y `dashboardSvc` ya existen en `New`.

## Sección 3 — Frontend: lib `ai.ts` + banda real

### `web/src/lib/ai.ts` (nuevo)

```ts
import { apiFetch } from "./api";

export type Insight = {
  content: string | null;
  available: boolean;
  generated_at: string | null;
};

export function todayString(date = new Date()): string {
  // mismo patrón que las otras libs (YYYY-MM-DD local)
}

export function getInsight(): Promise<Insight> {
  return apiFetch<Insight>(`/api/v1/ai/insight?today=${todayString()}`);
}
```

### `web/src/routes/index.tsx`

La `AIBand` (hoy estática) pasa a tener **su propia query**, independiente del
snapshot del dashboard:

```ts
const insightQ = useQuery({
  queryKey: ["ai-insight", todayString()],
  queryFn: getInsight,
  enabled: !!user,
});
```

Render de la banda (fondo ámbar tenue + ✦, full-width, igual layout actual):

- **Cargando** (`insightQ.isLoading`) → "Generando tu insight…".
- **`data.available && data.content`** → muestra el párrafo generado.
- **`!available`, error, o sin data** → texto de respaldo actual:
  "Tu insight del día llega pronto".

Clave: la banda **falla sola**. Su error/`available:false` no afecta la query
`['dashboard', …]` separada, así que el resto del dashboard sigue intacto. No
hay estado de error que rompa la página: la banda siempre degrada al placeholder.

Sin cambios en `TopBar` ni otras rutas. Sin colores nuevos en `tailwind.config.js`.

## Sección 4 — Testing y criterios de aceptación

### Backend — unit (`ai` package, sin DB ni red)
- `buildPrompt(snapshotJSON)`: el `user` contiene los campos del snapshot
  (racha, net, ánimo, metas); el `system` fija idioma/tono/largo.
- Cliente Groq contra `httptest.Server`: respuesta OK → extrae
  `choices[0].message.content`; HTTP 500 / body inválido / sin choices → error.
- `DailyInsight` con `Completer` fake:
  - `hasKey=false` → `Available:false`, no invoca el fake, no persiste.
  - fake devuelve error → `Available:false`, no persiste.
  - fake OK → `Available:true`, content correcto.

### Backend — integración (`handler_test.go`, `testutil.NewDB` + fake `Completer`)
1. `TestGeneratesAndCaches` — primera llamada genera vía fake + persiste;
   segunda **no** invoca el fake (lee cache); mismo content.
2. `TestNoKeyDegrades` — service sin clave → `200 {available:false, content:null}`,
   sin filas en `ai_insights`.
3. `TestGroqFailureDegrades` — fake que falla → `200 {available:false}`, sin
   persistir, sin romper.
4. `TestRequiresAuth` — sin token → 401.
5. `TestUserIsolation` — el insight cacheado de A no aparece para B (a B se le
   genera el suyo con su propio fake).

> Nota: los tests de integración construyen el `ai.Service` con un fake
> `Completer` (no el `GroqClient` real). El handler se monta igual que en
> `dashboard/handler_test.go` (chi + `RequireAuth` + token de prueba).

### Frontend — Vitest
- `ai.test.ts`: `getInsight` arma la URL `/api/v1/ai/insight?today=…` con método
  GET (patrón `vi.fn((_url, _opts?) => …)` + `mock.calls[0][0]`).
- `index.test.tsx` (extender): con insight mock `available:true` la banda muestra
  el content; con `available:false` muestra el placeholder; estado de carga
  muestra "Generando tu insight…"; un error de la query de IA **no** rompe el
  resto del dashboard (racha/superávit siguen visibles).

### E2E docker smoke (bash, como Plan 7)
Como `GROQ_API_KEY` viene **vacío** por defecto (sin secretos en el repo), el
smoke verifica el **camino degradado**: registrar usuario → `GET /ai/insight` →
`200 {available:false}` y tabla `ai_insights` sin filas → `SMOKE OK`. No se hace
una llamada real a Groq; no se filtra ninguna clave.

### Criterios de aceptación
- `GET /ai/insight` genera 1 insight proactivo por usuario/día, lo cachea en
  `ai_insights` y lo reusa; scopeado por usuario.
- Degrada a `available:false` sin clave o ante fallo de Groq; **nunca** 500 ni
  rompe el dashboard.
- La banda IA muestra el insight real, el estado de carga, o el placeholder.
- `make check` verde + Vitest verde + smoke `SMOKE OK`.

## Fuera de alcance
- **Chat on-demand** (segundo modo del macro §5) — rebanada futura; la tabla ya
  lo contempla (`kind=on_demand`).
- Botón "regenerar", scheduler/cron de generación, notificaciones push.
- Streaming de la respuesta de Groq (se devuelve el texto completo).
