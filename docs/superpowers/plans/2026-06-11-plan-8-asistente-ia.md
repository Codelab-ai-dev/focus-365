# Plan 8 — Asistente IA (insight proactivo) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activar la banda de IA del dashboard con un insight proactivo real generado por Groq, cacheado uno por usuario y día, que degrada a un placeholder cuando no hay IA disponible.

**Architecture:** Un paquete `ai` reutiliza `dashboard.Service.Snapshot` para el contexto del día (DRY) y llama a Groq detrás de una interfaz `Completer` (testeable con fakes). El servicio cachea en una tabla `ai_insights` (1 por usuario/día), degrada a `available:false` sin clave o ante fallo de IA, y nunca rompe el dashboard. El frontend agrega una query independiente para la banda.

**Tech Stack:** Go 1.23 (chi v5, pgx/v5, sqlc, goose, google/uuid), PostgreSQL 16, React 18 + TanStack Query + Vitest, Groq (endpoint OpenAI-compatible, `llama-3.3-70b-versatile`).

**Convenciones del repo:** commits y comentarios en español. Comandos `go`/`sqlc` con `GOTOOLCHAIN=local` desde `api/`. Tests de integración requieren `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"` (si falta, `testutil.NewDB` hace skip). Nunca editar `go.mod`/`go.sum` ni el código generado por sqlc a mano.

---

### Task 1: Migración `ai_insights` + queries + store round-trip

**Files:**
- Create: `api/db/migrations/0007_ai_insights.sql`
- Create: `api/db/queries/ai_insights.sql`
- Test: `api/internal/store/ai_insights_test.go`
- Generated (por sqlc, no editar a mano): `api/internal/store/ai_insights.sql.go`, `api/internal/store/models.go`

- [ ] **Step 1: Escribir el test del store (round-trip Create/Get + ErrNoRows)**

Crear `api/internal/store/ai_insights_test.go`:

```go
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestCreateAndGetInsight(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "ai@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	day := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	snap := []byte(`{"streak":{"best_current":12}}`)

	created, err := q.CreateInsight(ctx, store.CreateInsightParams{
		UserID:          user.ID,
		InsightDate:     day,
		Kind:            "proactive",
		Content:         "Tu racha de 12 días está en juego: cierra el hábito hoy.",
		ContextSnapshot: snap,
	})
	if err != nil {
		t.Fatalf("CreateInsight: %v", err)
	}
	if created.Content == "" || created.GeneratedAt.IsZero() {
		t.Errorf("insight creado incompleto: %+v", created)
	}

	got, err := q.GetInsight(ctx, store.GetInsightParams{
		UserID: user.ID, InsightDate: day, Kind: "proactive",
	})
	if err != nil {
		t.Fatalf("GetInsight: %v", err)
	}
	if got.ID != created.ID || got.Content != created.Content {
		t.Errorf("get != create: %+v vs %+v", got, created)
	}

	// Sin insight para otro día → ErrNoRows (señal de "sin cache").
	other := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	if _, err := q.GetInsight(ctx, store.GetInsightParams{
		UserID: user.ID, InsightDate: other, Kind: "proactive",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("esperaba ErrNoRows para día sin insight, got %v", err)
	}
}
```

- [ ] **Step 2: Correr el test para verificar que NO compila (tipos generados ausentes)**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/store/ -run TestCreateAndGetInsight`
Expected: FAIL de compilación — `undefined: store.CreateInsightParams` / `store.GetInsightParams`.

- [ ] **Step 3: Escribir la migración**

Crear `api/db/migrations/0007_ai_insights.sql`:

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

- [ ] **Step 4: Escribir las queries sqlc**

Crear `api/db/queries/ai_insights.sql`:

```sql
-- name: GetInsight :one
SELECT * FROM ai_insights
WHERE user_id = $1 AND insight_date = $2 AND kind = $3;

-- name: CreateInsight :one
INSERT INTO ai_insights (user_id, insight_date, kind, content, context_snapshot)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
```

- [ ] **Step 5: Generar el código del store**

Run: `cd api && GOTOOLCHAIN=local sqlc generate`
Expected: sin errores; aparece `api/internal/store/ai_insights.sql.go` y `models.go` gana el struct `AiInsight` (campos `ID`, `UserID`, `InsightDate time.Time`, `Kind string`, `Content string`, `ContextSnapshot []byte`, `GeneratedAt time.Time`).

- [ ] **Step 6: Correr el test contra la DB para verificar que pasa**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/store/ -run TestCreateAndGetInsight -v`
Expected: PASS (`testutil.NewDB` aplica las migraciones, incluida la 0007). Requiere el contenedor `db` arriba (`docker compose up -d db`).

- [ ] **Step 7: Commit**

```bash
cd api && git add db/migrations/0007_ai_insights.sql db/queries/ai_insights.sql internal/store/ai_insights.sql.go internal/store/models.go internal/store/ai_insights_test.go
git commit -m "feat(ai): tabla ai_insights con cache diario (migración + queries + store)"
```

---

### Task 2: Config lee `GROQ_API_KEY` / `GROQ_MODEL`

**Files:**
- Modify: `api/internal/config/config.go`
- Test: `api/internal/config/config_test.go`

- [ ] **Step 1: Agregar los tests de config (default del modelo + valores)**

Agregar al final de `api/internal/config/config_test.go`:

```go
func TestLoadGroqDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/focus")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("GROQ_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "" {
		t.Errorf("GroqAPIKey = %q, want empty (modo degradado)", cfg.GroqAPIKey)
	}
	if cfg.GroqModel != "llama-3.3-70b-versatile" {
		t.Errorf("GroqModel default = %q", cfg.GroqModel)
	}
}

func TestLoadGroqValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/focus")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("GROQ_API_KEY", "gsk_abc")
	t.Setenv("GROQ_MODEL", "llama-custom")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "gsk_abc" {
		t.Errorf("GroqAPIKey = %q", cfg.GroqAPIKey)
	}
	if cfg.GroqModel != "llama-custom" {
		t.Errorf("GroqModel = %q", cfg.GroqModel)
	}
}
```

- [ ] **Step 2: Correr los tests para verificar que fallan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/config/ -run TestLoadGroq`
Expected: FAIL de compilación — `cfg.GroqAPIKey`/`cfg.GroqModel` no existen.

- [ ] **Step 3: Agregar los campos y la lectura en config.go**

En `api/internal/config/config.go`, agregar los campos al struct:

```go
type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
	CORSOrigin  string
	GroqAPIKey  string
	GroqModel   string
}
```

Dentro de `Load()`, agregar las lecturas al literal de `cfg` (junto a las demás):

```go
		GroqAPIKey:  os.Getenv("GROQ_API_KEY"),
		GroqModel:   os.Getenv("GROQ_MODEL"),
```

Y antes del `return cfg, nil`, agregar el default del modelo (clave vacía NO es error: habilita el modo degradado):

```go
	if cfg.GroqModel == "" {
		cfg.GroqModel = "llama-3.3-70b-versatile"
	}
```

- [ ] **Step 4: Correr los tests para verificar que pasan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/config/`
Expected: PASS (todos los tests de config, viejos y nuevos).

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(ai): config lee GROQ_API_KEY y GROQ_MODEL (clave vacía = degradado)"
```

---

### Task 3: Cliente Groq detrás de la interfaz `Completer`

**Files:**
- Create: `api/internal/ai/groq.go`
- Test: `api/internal/ai/groq_test.go`

- [ ] **Step 1: Escribir los tests del cliente (httptest: OK / 500 / sin choices / body inválido)**

Crear `api/internal/ai/groq_test.go`:

```go
package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroqCompleteOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Buen ritmo hoy."}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
	got, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "Buen ritmo hoy." {
		t.Errorf("content = %q", got)
	}
}

func TestGroqCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}

func TestGroqCompleteNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error sin choices")
	}
}

func TestGroqCompleteInvalidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`no-json`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error con body inválido")
	}
}
```

- [ ] **Step 2: Correr los tests para verificar que NO compilan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/`
Expected: FAIL — el paquete `ai` no existe todavía (`newGroqClient` undefined).

- [ ] **Step 3: Implementar el cliente Groq**

Crear `api/internal/ai/groq.go`:

```go
// Package ai genera el insight proactivo diario a partir del snapshot del
// dashboard, usando Groq (clave server-side) detrás de una interfaz testeable.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultGroqBaseURL = "https://api.groq.com/openai/v1"

// Completer abstrae la llamada al LLM para testear el servicio con un fake
// (sin red). GroqClient es la implementación real.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// GroqClient habla con el endpoint OpenAI-compatible de Groq.
type GroqClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewGroqClient crea el cliente real contra la API pública de Groq.
func NewGroqClient(apiKey, model string) *GroqClient {
	return newGroqClient(defaultGroqBaseURL, apiKey, model)
}

// newGroqClient permite inyectar baseURL (httptest.Server) en los tests.
func newGroqClient(baseURL, apiKey, model string) *GroqClient {
	return &GroqClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete envía system+user a Groq y devuelve choices[0].message.content.
func (c *GroqClient) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
		MaxTokens:   200,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("groq status %d: %s", res.StatusCode, string(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("groq sin choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
```

- [ ] **Step 4: Correr los tests para verificar que pasan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/`
Expected: PASS (4 tests del cliente; no necesitan DB ni red real).

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/ai/groq.go internal/ai/groq_test.go
git commit -m "feat(ai): cliente Groq detrás de la interfaz Completer"
```

---

### Task 4: Construcción del prompt

**Files:**
- Create: `api/internal/ai/prompt.go`
- Test: `api/internal/ai/prompt_test.go`

- [ ] **Step 1: Escribir el test del prompt**

Crear `api/internal/ai/prompt_test.go`:

```go
package ai

import (
	"strings"
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	snap := `{"streak":{"best_current":12},"finance":{"net":320000},"checkin":{"mood":8}}`
	system, user := buildPrompt(snap)

	if !strings.Contains(strings.ToLower(system), "español") {
		t.Errorf("system no fija el idioma: %q", system)
	}
	if !strings.Contains(system, "1 a 3 frases") {
		t.Errorf("system no fija el largo: %q", system)
	}
	if !strings.Contains(user, snap) {
		t.Errorf("user no incluye el snapshot: %q", user)
	}
}
```

- [ ] **Step 2: Correr el test para verificar que NO compila**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestBuildPrompt`
Expected: FAIL — `buildPrompt` undefined.

- [ ] **Step 3: Implementar `buildPrompt`**

Crear `api/internal/ai/prompt.go`:

```go
package ai

// buildPrompt arma los mensajes system+user para Groq. El system fija idioma
// (español), tono (entrenador cálido) y largo (1-3 frases); el user lleva el
// snapshot del día en JSON para que el modelo detecte patrones reales.
func buildPrompt(snapshotJSON string) (system, user string) {
	system = "Eres un entrenador personal cálido y directo. " +
		"Escribe SIEMPRE en español, en un solo párrafo de 1 a 3 frases, en texto plano (sin markdown ni listas). " +
		"A partir de los datos del día del usuario, detecta el patrón más accionable " +
		"(racha en riesgo, gasto alto, ánimo o energía altos para aprovechar, metas vencidas) " +
		"y dale un consejo concreto y motivador para hoy. No saludes ni te despidas."
	user = "Estos son mis datos de hoy (JSON):\n" + snapshotJSON
	return system, user
}
```

- [ ] **Step 4: Correr el test para verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestBuildPrompt`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/ai/prompt.go internal/ai/prompt_test.go
git commit -m "feat(ai): prompt del insight (español, 1-3 frases, accionable)"
```

---

### Task 5: Servicio `DailyInsight` (cache → sin clave → snapshot → Groq → persistir)

**Files:**
- Create: `api/internal/ai/types.go`
- Create: `api/internal/ai/service.go`
- Test: `api/internal/ai/service_test.go`

- [ ] **Step 1: Escribir los tests unitarios del servicio (con fakes, sin DB ni red)**

Crear `api/internal/ai/service_test.go`:

```go
package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeSnap struct {
	snap *dashboard.Snapshot
	err  error
}

func (f fakeSnap) Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*dashboard.Snapshot, error) {
	return f.snap, f.err
}

type fakeStore struct {
	got       *store.AiInsight // nil → ErrNoRows en GetInsight
	created   *store.CreateInsightParams
	createErr error
}

func (f *fakeStore) GetInsight(ctx context.Context, arg store.GetInsightParams) (store.AiInsight, error) {
	if f.got == nil {
		return store.AiInsight{}, pgx.ErrNoRows
	}
	return *f.got, nil
}

func (f *fakeStore) CreateInsight(ctx context.Context, arg store.CreateInsightParams) (store.AiInsight, error) {
	if f.createErr != nil {
		return store.AiInsight{}, f.createErr
	}
	f.created = &arg
	return store.AiInsight{
		ID: uuid.New(), UserID: arg.UserID, InsightDate: arg.InsightDate,
		Kind: arg.Kind, Content: arg.Content, ContextSnapshot: arg.ContextSnapshot,
		GeneratedAt: time.Now(),
	}, nil
}

type fakeCompleter struct {
	out    string
	err    error
	called int
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.called++
	return f.out, f.err
}

var (
	testUser = uuid.New()
	testDay  = time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
)

func emptySnap() *dashboard.Snapshot { return &dashboard.Snapshot{} }

func TestDailyInsightCacheHit(t *testing.T) {
	st := &fakeStore{got: &store.AiInsight{Content: "cacheado", GeneratedAt: testDay}}
	comp := &fakeCompleter{out: "nuevo"}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, true)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if !got.Available || got.Content != "cacheado" {
		t.Errorf("esperaba cache hit, got %+v", got)
	}
	if comp.called != 0 {
		t.Errorf("no debía llamar a Groq con cache, called=%d", comp.called)
	}
}

func TestDailyInsightNoKey(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{out: "x"}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, false)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if got.Available {
		t.Errorf("sin clave debería degradar, got %+v", got)
	}
	if comp.called != 0 || st.created != nil {
		t.Errorf("sin clave no debe llamar Groq ni persistir")
	}
}

func TestDailyInsightGroqError(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{err: errors.New("boom")}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, true)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if got.Available {
		t.Errorf("fallo de Groq debería degradar, got %+v", got)
	}
	if st.created != nil {
		t.Errorf("fallo de Groq no debe persistir")
	}
}

func TestDailyInsightGeneratesAndPersists(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{out: "Aprovecha tu energía alta hoy."}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, true)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if !got.Available || got.Content != "Aprovecha tu energía alta hoy." {
		t.Errorf("esperaba insight generado, got %+v", got)
	}
	if comp.called != 1 {
		t.Errorf("debía llamar a Groq una vez, called=%d", comp.called)
	}
	if st.created == nil || st.created.Content != "Aprovecha tu energía alta hoy." {
		t.Errorf("no persistió el insight: %+v", st.created)
	}
}

func TestDailyInsightSnapshotError(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{out: "x"}
	svc := NewService(fakeSnap{err: errors.New("db caída")}, st, comp, true)

	if _, err := svc.DailyInsight(context.Background(), testUser, testDay); err == nil {
		t.Fatal("error de Snapshot debe propagarse (500)")
	}
}
```

- [ ] **Step 2: Correr los tests para verificar que NO compilan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestDailyInsight`
Expected: FAIL — `NewService`, `Service`, `Insight` undefined.

- [ ] **Step 3: Implementar el tipo de vista**

Crear `api/internal/ai/types.go`:

```go
package ai

import "time"

// Insight es la vista que devuelve el servicio. Cuando Available es false (sin
// clave o fallo de IA), Content queda vacío y el handler serializa
// content/generated_at como null.
type Insight struct {
	Content     string    `json:"content"`
	Available   bool      `json:"available"`
	GeneratedAt time.Time `json:"generated_at"`
}
```

- [ ] **Step 4: Implementar el servicio**

Crear `api/internal/ai/service.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const kindProactive = "proactive"

// snapshotter es la porción de dashboard.Service que necesitamos (el contexto
// del día). Interfaz para testear el servicio sin la DB del dashboard.
type snapshotter interface {
	Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*dashboard.Snapshot, error)
}

// insightStore es la porción de store.Queries que usamos (cache diario).
type insightStore interface {
	GetInsight(ctx context.Context, arg store.GetInsightParams) (store.AiInsight, error)
	CreateInsight(ctx context.Context, arg store.CreateInsightParams) (store.AiInsight, error)
}

// Service genera y cachea el insight proactivo del día.
type Service struct {
	dash   snapshotter
	store  insightStore
	groq   Completer
	hasKey bool
}

// NewService inyecta el dashboard (contexto), el store (cache), el Completer
// (Groq o fake) y si hay clave configurada.
func NewService(dash snapshotter, q insightStore, c Completer, hasKey bool) *Service {
	return &Service{dash: dash, store: q, groq: c, hasKey: hasKey}
}

// DailyInsight devuelve el insight de hoy: lee cache, o lo genera vía Groq y lo
// persiste. Degrada a Available:false sin clave o ante fallo de Groq; solo
// propaga error si fallan Snapshot o la DB (problema real, no de IA).
func (s *Service) DailyInsight(ctx context.Context, userID uuid.UUID, today time.Time) (*Insight, error) {
	// 1. Cache.
	row, err := s.store.GetInsight(ctx, store.GetInsightParams{
		UserID: userID, InsightDate: today, Kind: kindProactive,
	})
	if err == nil {
		return &Insight{Content: row.Content, Available: true, GeneratedAt: row.GeneratedAt}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// 2. Sin clave → degradado, sin tocar Groq ni persistir.
	if !s.hasKey {
		return &Insight{Available: false}, nil
	}

	// 3. Contexto del día.
	snap, err := s.dash.Snapshot(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	ctxJSON, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}

	// 4-5. Prompt + Groq. Fallo de IA → degradado (no rompe el dashboard).
	system, user := buildPrompt(string(ctxJSON))
	content, err := s.groq.Complete(ctx, system, user)
	if err != nil {
		return &Insight{Available: false}, nil
	}

	// 6. Persistir y devolver.
	created, err := s.store.CreateInsight(ctx, store.CreateInsightParams{
		UserID:          userID,
		InsightDate:     today,
		Kind:            kindProactive,
		Content:         content,
		ContextSnapshot: ctxJSON,
	})
	if err != nil {
		return nil, err
	}
	return &Insight{Content: created.Content, Available: true, GeneratedAt: created.GeneratedAt}, nil
}
```

- [ ] **Step 5: Correr los tests para verificar que pasan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/`
Expected: PASS (cliente Groq + prompt + 5 tests del servicio).

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/ai/types.go internal/ai/service.go internal/ai/service_test.go
git commit -m "feat(ai): servicio DailyInsight con cache, degradación y persistencia"
```

---

### Task 6: Handler `GET /ai/insight` + montaje en el server + wiring en main + integración

**Files:**
- Create: `api/internal/ai/handler.go`
- Modify: `api/internal/server/server.go`
- Modify: `api/cmd/server/main.go`
- Test: `api/internal/ai/handler_test.go`

- [ ] **Step 1: Escribir el test de integración (DB real + fake Completer)**

Crear `api/internal/ai/handler_test.go`:

```go
package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/focus365/api/internal/ai"
	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/focus365/api/internal/training"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const today = "2026-06-11"

// fakeCompleter cuenta llamadas y devuelve un texto fijo (o error).
type fakeCompleter struct {
	out    string
	err    error
	called int
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.called++
	return f.out, f.err
}

type env struct {
	h    http.Handler
	auth *auth.Service
	comp *fakeCompleter
	q    *store.Queries
}

func newEnv(t *testing.T, hasKey bool, comp *fakeCompleter) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")

	ci := checkin.NewService(q)
	fi := finance.NewService(q)
	tr := training.NewService(q, pool)
	ha := habits.NewService(q)
	go_ := goals.NewService(q)
	dash := dashboard.NewService(ci, fi, tr, ha, go_)

	svc := ai.NewService(dash, q, comp, hasKey)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/ai", ai.Routes(svc))
	})
	return &env{h: r, auth: auth.NewService(q, tm), comp: comp, q: q}
}

func (e *env) user(t *testing.T, email string) (uuid.UUID, string) {
	t.Helper()
	u, err := e.auth.Register(context.Background(), email, "p4ssword", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	access, _, err := e.auth.IssueTokens(u.ID)
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	return u.ID, access
}

func get(t *testing.T, h http.Handler, tok string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ai/insight?today="+today, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func dayTime(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", today)
	if err != nil {
		t.Fatalf("parse today: %v", err)
	}
	return d
}

func TestGeneratesAndCaches(t *testing.T) {
	comp := &fakeCompleter{out: "Aprovecha tu racha hoy."}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "gen@b.com")

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body["available"] != true || body["content"] != "Aprovecha tu racha hoy." {
		t.Errorf("primera respuesta = %v", body)
	}
	if comp.called != 1 {
		t.Errorf("Groq llamado %d veces, want 1", comp.called)
	}

	// Segunda llamada: lee cache, no invoca Groq otra vez.
	rec2, body2 := get(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("segunda code = %d", rec2.Code)
	}
	if body2["content"] != "Aprovecha tu racha hoy." {
		t.Errorf("cache content = %v", body2)
	}
	if comp.called != 1 {
		t.Errorf("Groq llamado %d veces tras cache, want 1", comp.called)
	}
}

func TestNoKeyDegrades(t *testing.T) {
	comp := &fakeCompleter{out: "no debería usarse"}
	e := newEnv(t, false, comp)
	uid, tok := e.user(t, "nokey@b.com")

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if body["available"] != false || body["content"] != nil {
		t.Errorf("degradado esperado, got %v", body)
	}
	if comp.called != 0 {
		t.Errorf("sin clave no debe llamar Groq")
	}
	if _, err := e.q.GetInsight(context.Background(), store.GetInsightParams{
		UserID: uid, InsightDate: dayTime(t), Kind: "proactive",
	}); err == nil {
		t.Errorf("no debería existir insight en DB")
	}
}

func TestGroqFailureDegrades(t *testing.T) {
	comp := &fakeCompleter{err: errors.New("groq caído")}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "fail@b.com")

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if body["available"] != false {
		t.Errorf("fallo de Groq debería degradar, got %v", body)
	}
	if _, err := e.q.GetInsight(context.Background(), store.GetInsightParams{
		UserID: uid, InsightDate: dayTime(t), Kind: "proactive",
	}); err == nil {
		t.Errorf("no debería persistir tras fallo de Groq")
	}
}

func TestRequiresAuth(t *testing.T) {
	comp := &fakeCompleter{out: "x"}
	e := newEnv(t, true, comp)
	rec, _ := get(t, e.h, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	comp := &fakeCompleter{out: "insight fresco"}
	e := newEnv(t, true, comp)
	_, tokA := e.user(t, "iA@b.com")
	_, tokB := e.user(t, "iB@b.com")

	// A genera su insight (primera llamada al fake).
	if rec, _ := get(t, e.h, tokA); rec.Code != http.StatusOK {
		t.Fatalf("A code = %d", rec.Code)
	}
	if comp.called != 1 {
		t.Errorf("tras A, called=%d want 1", comp.called)
	}

	// B no ve el cache de A: se le genera el suyo (segunda llamada al fake).
	rec, body := get(t, e.h, tokB)
	if rec.Code != http.StatusOK {
		t.Fatalf("B code = %d", rec.Code)
	}
	if body["available"] != true || body["content"] != "insight fresco" {
		t.Errorf("B respuesta = %v", body)
	}
	if comp.called != 2 {
		t.Errorf("B debió generar el suyo, called=%d want 2", comp.called)
	}
}
```

- [ ] **Step 2: Correr el test para verificar que NO compila**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestGeneratesAndCaches`
Expected: FAIL — `ai.Routes` undefined.

- [ ] **Step 3: Implementar el handler**

Crear `api/internal/ai/handler.go`:

```go
package ai

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

const dateLayout = "2006-01-02"

// Routes monta el endpoint del asistente (bajo RequireAuth en server.go).
func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/insight", handleInsight(svc))
	return r
}

// insightResponse usa punteros para serializar content/generated_at como null
// en el modo degradado (available:false), en vez de ""/zero.
type insightResponse struct {
	Content     *string    `json:"content"`
	Available   bool       `json:"available"`
	GeneratedAt *time.Time `json:"generated_at"`
}

func toResponse(in *Insight) insightResponse {
	if !in.Available {
		return insightResponse{Available: false}
	}
	return insightResponse{
		Content:     &in.Content,
		Available:   true,
		GeneratedAt: &in.GeneratedAt,
	}
}

func handleInsight(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		in, err := svc.DailyInsight(r.Context(), userID, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toResponse(in))
	}
}

// parseTodayParam lee ?today=YYYY-MM-DD; si falta o es inválido, usa el día UTC
// del server. Mismo patrón que dashboard/metas.
func parseTodayParam(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
```

- [ ] **Step 4: Montar la ruta en el server**

En `api/internal/server/server.go`:

Agregar el import del paquete `ai` (alfabético, antes de `auth`):

```go
	"github.com/focus365/api/internal/ai"
	"github.com/focus365/api/internal/auth"
```

Agregar los campos a `Deps`:

```go
type Deps struct {
	Pool       *pgxpool.Pool
	JWTSecret  string
	CORSOrigin string
	GroqAPIKey string
	GroqModel  string
}
```

Dentro del grupo `RequireAuth`, justo después del `r.Mount("/dashboard", ...)`, agregar:

```go
			groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel)
			aiSvc := ai.NewService(dashboardSvc, q, groq, d.GroqAPIKey != "")
			r.Mount("/ai", ai.Routes(aiSvc))
```

- [ ] **Step 5: Pasar la config en main**

En `api/cmd/server/main.go`, ampliar el literal `server.Deps`:

```go
	h := server.New(server.Deps{
		Pool:       pool,
		JWTSecret:  cfg.JWTSecret,
		CORSOrigin: cfg.CORSOrigin,
		GroqAPIKey: cfg.GroqAPIKey,
		GroqModel:  cfg.GroqModel,
	})
```

- [ ] **Step 6: Correr los tests de integración (DB) + build del server**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/ai/ ./internal/server/ -v && GOTOOLCHAIN=local go build ./...`
Expected: PASS (5 tests de integración + unit del paquete `ai`; server compila con el nuevo mount). Requiere el contenedor `db` arriba.

- [ ] **Step 7: Commit**

```bash
cd api && git add internal/ai/handler.go internal/ai/handler_test.go internal/server/server.go cmd/server/main.go
git commit -m "feat(ai): endpoint GET /ai/insight montado bajo RequireAuth"
```

---

### Task 7: Frontend lib `ai.ts`

**Files:**
- Create: `web/src/lib/ai.ts`
- Test: `web/src/lib/ai.test.ts`

- [ ] **Step 1: Escribir el test de la lib**

Crear `web/src/lib/ai.test.ts`:

```ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { getInsight, type Insight } from "./ai";

function okJson(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify(data), { status: 200 }));
}

describe("getInsight", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/ai/insight con el día de hoy", async () => {
    const insight: Insight = {
      content: "Aprovecha tu energía hoy.",
      available: true,
      generated_at: "2026-06-11T10:00:00Z",
    };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson(insight));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getInsight();
    expect(got).toEqual(insight);

    const url = fetchMock.mock.calls[0][0];
    expect(url).toMatch(/^\/api\/v1\/ai\/insight\?today=\d{4}-\d{2}-\d{2}$/);
    const opts = fetchMock.mock.calls[0][1];
    expect(opts?.method ?? "GET").toBe("GET");
  });
});
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd web && npx vitest run src/lib/ai.test.ts`
Expected: FAIL — no existe `./ai`.

- [ ] **Step 3: Implementar la lib**

Crear `web/src/lib/ai.ts`:

```ts
import { apiFetch } from "./api";
import { todayString } from "./dashboard";

export type Insight = {
  content: string | null;
  available: boolean;
  generated_at: string | null;
};

export function getInsight(): Promise<Insight> {
  return apiFetch<Insight>(`/api/v1/ai/insight?today=${todayString()}`);
}
```

- [ ] **Step 4: Correr el test para verificar que pasa**

Run: `cd web && npx vitest run src/lib/ai.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd web && git add src/lib/ai.ts src/lib/ai.test.ts
git commit -m "feat(ai): lib ai.ts (getInsight) en el frontend"
```

---

### Task 8: Banda de IA real en el dashboard

**Files:**
- Modify: `web/src/routes/index.tsx`
- Test: `web/src/routes/index.test.tsx`

- [ ] **Step 1: Actualizar el test del dashboard (mock URL-aware + casos de la banda)**

En `web/src/routes/index.test.tsx`:

Agregar el import del tipo `Insight` junto a los imports existentes:

```ts
import type { Insight } from "@/lib/ai";
```

Agregar los helpers (después de `makeSnap`):

```ts
function makeInsight(overrides: Partial<Insight> = {}): Insight {
  return {
    content: "Aprovecha tu energía alta hoy.",
    available: true,
    generated_at: "2026-06-11T10:00:00Z",
    ...overrides,
  };
}

function routeFetch(snap = makeSnap(), insight: Insight | "error" = makeInsight()) {
  return vi.fn((url: string, _opts?: RequestInit) => {
    if (url.includes("/ai/insight")) {
      if (insight === "error") {
        return Promise.resolve(new Response("boom", { status: 500 }));
      }
      return okJson(insight);
    }
    return okJson(snap);
  });
}
```

Reemplazar el `beforeEach` para que el mock global sirva ambas URLs:

```ts
  beforeEach(() => vi.stubGlobal("fetch", routeFetch()));
```

Reemplazar el test existente `"muestra la banda de IA placeholder"` por el caso de contenido real (la banda ahora muestra el insight por defecto):

```ts
  it("muestra el insight de IA cuando está disponible", async () => {
    renderPage();
    expect(await screen.findByText(/Aprovecha tu energía alta hoy/)).toBeInTheDocument();
  });
```

Reemplazar el override del test `"muestra 'Sin check-in hoy' cuando checkin es null"` para que use el mock URL-aware:

```ts
    vi.stubGlobal("fetch", routeFetch(makeSnap({ checkin: null, dimensions_active: 3 })));
```

Agregar estos tests nuevos al final del `describe`:

```ts
  it("muestra el placeholder cuando la IA no está disponible", async () => {
    vi.stubGlobal("fetch", routeFetch(makeSnap(), makeInsight({ available: false, content: null })));
    renderPage();
    expect(await screen.findByText(/Tu insight del día llega pronto/)).toBeInTheDocument();
  });

  it("muestra el estado de carga del insight", async () => {
    const pending = new Promise<Response>(() => {});
    vi.stubGlobal("fetch", vi.fn((url: string) => {
      if (url.includes("/ai/insight")) return pending;
      return okJson(makeSnap());
    }));
    renderPage();
    expect(await screen.findByText(/Generando tu insight…/)).toBeInTheDocument();
  });

  it("un error de la IA no rompe el resto del dashboard", async () => {
    vi.stubGlobal("fetch", routeFetch(makeSnap(), "error"));
    renderPage();
    expect(await screen.findByText(/Tu insight del día llega pronto/)).toBeInTheDocument();
    // El resto del dashboard sigue visible (la racha llega igual).
    expect(screen.getByText(/12/)).toBeInTheDocument();
  });
```

- [ ] **Step 2: Correr los tests para verificar que fallan**

Run: `cd web && npx vitest run src/routes/index.test.tsx`
Expected: FAIL — la `AIBand` estática no muestra el content ni "Generando tu insight…".

- [ ] **Step 3: Convertir `AIBand` en una banda con su propia query**

En `web/src/routes/index.tsx`:

Agregar el import de la lib de IA (después de los imports de `@/lib`):

```ts
import { getInsight } from "@/lib/ai";
```

Reemplazar la función `AIBand` completa (líneas 74-80) por:

```tsx
function AIBand() {
  const { user } = useAuth();
  const insightQ = useQuery({
    queryKey: ["ai-insight", todayString()],
    queryFn: getInsight,
    enabled: !!user,
  });

  const base =
    "rounded-lg border border-dashed border-amber-brand bg-amber-brand/10 px-4 py-3 text-sm font-bold text-amber-brand";

  if (insightQ.isLoading) {
    return <div className={base}>✦ Generando tu insight…</div>;
  }
  if (insightQ.data?.available && insightQ.data.content) {
    return <div className={base}>✦ {insightQ.data.content}</div>;
  }
  return <div className={base}>✦ Tu insight del día llega pronto</div>;
}
```

> `useQuery`, `useAuth` y `todayString` ya están importados en `index.tsx`. La banda tiene su propia query `["ai-insight", …]`, independiente de `["dashboard", …]`, así que su error/`available:false` nunca rompe el resto del dashboard.

- [ ] **Step 4: Correr los tests para verificar que pasan**

Run: `cd web && npx vitest run src/routes/index.test.tsx`
Expected: PASS (saludo, racha/superávit, insight real, placeholder, carga, aislamiento de error, check-in null, metas vencidas, links).

- [ ] **Step 5: Verificar el build estricto del frontend (tsc + vite)**

Run: `cd web && npm run build`
Expected: build OK (los archivos de test también typechequean con `tsc -b`).

- [ ] **Step 6: Commit**

```bash
cd web && git add src/routes/index.tsx src/routes/index.test.tsx
git commit -m "feat(ai): banda de IA con insight real, carga y degradación a placeholder"
```

---

### Task 9: E2E docker smoke (camino degradado) + verificación final

**Files:**
- Create: `/tmp/smoke_ai.sh` (no versionado)

> Nota de entorno: los comandos docker necesitan `dangerouslyDisableSandbox: true` y, en la MISMA línea, `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"`. Los scripts se corren con `bash /tmp/archivo.sh` (zsh rompe con UUIDs inline). `GROQ_API_KEY` viene vacío por defecto → el smoke verifica el camino degradado (sin llamadas reales a Groq, sin secretos en el repo).

- [ ] **Step 1: Reconstruir el stack para tomar la migración 0007 y el endpoint nuevo**

Run (con `dangerouslyDisableSandbox: true`):
`export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin" && cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build`
Expected: contenedores `db`, `api`, `web` arriba; el `api` aplica la migración 0007 al arrancar.

- [ ] **Step 2: Escribir el script de smoke**

Crear `/tmp/smoke_ai.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="http://localhost:8088/api/v1"
EMAIL="ai_smoke_$(date +%s)@b.com"

reg=$(curl -s -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"p4ssword\",\"name\":\"Smoke\"}")
TOKEN=$(python3 -c "import sys,json; print(json.loads(sys.argv[1])['access_token'])" "$reg")

today=$(date +%F)
resp=$(curl -s "$BASE/ai/insight?today=$today" -H "Authorization: Bearer $TOKEN")
python3 - "$resp" <<'PY'
import sys, json
d = json.loads(sys.argv[1])
assert d["available"] is False, d
assert d["content"] is None, d
assert d["generated_at"] is None, d
print("insight degradado OK")
PY

# Sin clave no se cachea nada: la tabla ai_insights queda vacía.
rows=$(docker compose exec -T db psql -U focus -d focus365 -tAc "SELECT count(*) FROM ai_insights;")
test "$(echo "$rows" | tr -d '[:space:]')" = "0" || { echo "ai_insights no está vacía: $rows"; exit 1; }

echo "SMOKE OK (4 checks)"
```

- [ ] **Step 3: Correr el smoke**

Run (con `dangerouslyDisableSandbox: true`):
`export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin" && cd /Users/gustavo/Desktop/focus-365 && bash /tmp/smoke_ai.sh`
Expected: `insight degradado OK` y `SMOKE OK (4 checks)`.

- [ ] **Step 4: Verificación final backend**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local make check`
Expected: `go vet ./...` + `go test -p 1 ./...` verde.

- [ ] **Step 5: Verificación final frontend**

Run: `cd web && npx vitest run && npm run build`
Expected: toda la suite Vitest verde + build OK.

- [ ] **Step 6: Commit final (si quedó algo sin commitear)**

```bash
cd /Users/gustavo/Desktop/focus-365 && git status
# Si hay cambios pendientes relevantes, commitearlos; el smoke en /tmp no se versiona.
```

---

## Self-Review

**Cobertura del spec:**
- §1 (modelo `ai_insights` + queries) → Task 1.
- §2 config → Task 2; cliente Groq → Task 3; prompt → Task 4; servicio `DailyInsight` → Task 5; handler + montaje + main → Task 6.
- §3 frontend (`ai.ts` + banda) → Tasks 7-8.
- §4 testing: unit (prompt/Groq/servicio) → Tasks 3-5; integración (5 tests) → Task 6; Vitest → Tasks 7-8; smoke degradado → Task 9; criterios de aceptación cubiertos por make check + Vitest + smoke.

**Consistencia de tipos:** `Completer.Complete(ctx, system, user) (string, error)`, `NewService(snapshotter, insightStore, Completer, bool)`, `DailyInsight(ctx, uuid.UUID, time.Time) (*Insight, error)`, `Insight{Content, Available, GeneratedAt}`, store `GetInsightParams{UserID, InsightDate, Kind}` / `CreateInsightParams{UserID, InsightDate, Kind, Content, ContextSnapshot []byte}` / struct `AiInsight` — usados de forma idéntica en service, handler y tests. Frontend `Insight{content|null, available, generated_at|null}` espeja la respuesta con punteros del handler.

**Sin placeholders:** todos los steps incluyen código y comandos completos con salida esperada.

---

## Execution Handoff

Plan completo y guardado en `docs/superpowers/plans/2026-06-11-plan-8-asistente-ia.md`. Dos opciones de ejecución:

1. **Subagent-Driven (recomendada)** — despacho un subagente fresco por tarea, reviso entre tareas, iteración rápida.
2. **Inline Execution** — ejecuto las tareas en esta sesión con checkpoints de revisión.

¿Qué enfoque preferís?
