# Plan 9 — Asistente IA on-demand (chat) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar un chat conversacional on-demand en `/asistente` donde el usuario pregunta sobre sus datos y la IA (Groq) responde con su contexto real (snapshot del día + histórico financiero + check-ins recientes), multi-turno y persistido, degradando con claridad cuando no hay IA.

**Architecture:** Se extiende el paquete `ai` ya montado en `/api/v1/ai`. El `GroqClient` gana un modo `Chat` (array de mensajes estilo OpenAI). Un `ChatContextBuilder` compone el contexto reutilizando `dashboard.Service.Snapshot`, `finance.Service.Cycles` y `checkin.Service.List` detrás de interfaces estrechas (DRY, testeable con fakes). Un `ChatService` carga el historial multi-turno, llama a Groq, y solo ante éxito persiste el par pregunta+respuesta en la tabla nueva `ai_messages` (1 conversación por usuario). El frontend agrega la ruta `/asistente`, su lib y la nav. El chat es independiente del insight proactivo: comparten el cliente Groq pero no se acoplan. Toda query va scoped por `user_id`.

**Tech Stack:** Go 1.23 (chi v5, pgx/v5, sqlc, goose, google/uuid, validator/v10), PostgreSQL 16, React 18 + TanStack Router + TanStack Query + Vitest, Groq (endpoint OpenAI-compatible, `llama-3.3-70b-versatile`).

**Convenciones del repo:** commits y comentarios en español. Comandos `go`/`sqlc` con `GOTOOLCHAIN=local` desde `api/`. Tests de integración requieren `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"` (si falta, `testutil.NewDB` hace skip; requiere el contenedor `db` arriba: `docker compose up -d db`). `make check` desde `api/` corre `go vet ./...` + `go test -p 1 ./...`. Nunca editar `go.mod`/`go.sum` ni el código generado por sqlc a mano. Frontend: `cd web && npm test` (Vitest) y `npm run build` (`tsc -b` estricto: los tests deben tipar).

---

### Task 1: Migración `ai_messages` + queries + store round-trip

**Files:**
- Create: `api/db/migrations/0008_ai_messages.sql`
- Create: `api/db/queries/ai_messages.sql`
- Test: `api/internal/store/ai_messages_test.go`
- Generated (por sqlc, no editar a mano): `api/internal/store/ai_messages.sql.go`, `api/internal/store/models.go`

- [ ] **Step 1: Escribir el test del store (round-trip Create/List + orden ASC + scoping)**

Crear `api/internal/store/ai_messages_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestCreateAndListMessages(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	ada, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "msg-a@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser Ada: %v", err)
	}
	bob, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "msg-b@b.com", PasswordHash: "h", Name: "Bob",
	})
	if err != nil {
		t.Fatalf("CreateUser Bob: %v", err)
	}

	// Ada escribe una pregunta y recibe una respuesta.
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: ada.ID, Role: "user", Content: "¿cómo voy en junio?",
	}); err != nil {
		t.Fatalf("CreateMessage user: %v", err)
	}
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: ada.ID, Role: "assistant", Content: "Vas verde este ciclo.",
	}); err != nil {
		t.Fatalf("CreateMessage assistant: %v", err)
	}
	// Mensaje de Bob: no debe aparecer en el historial de Ada (scoping).
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: bob.ID, Role: "user", Content: "hola",
	}); err != nil {
		t.Fatalf("CreateMessage Bob: %v", err)
	}

	rows, err := q.ListMessages(ctx, ada.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Ada tiene %d mensajes, want 2 (scoping falló)", len(rows))
	}
	// Orden ASC por created_at: primero la pregunta, luego la respuesta.
	if rows[0].Role != "user" || rows[0].Content != "¿cómo voy en junio?" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[1].Role != "assistant" || rows[1].Content != "Vas verde este ciclo." {
		t.Errorf("rows[1] = %+v", rows[1])
	}
	if rows[1].CreatedAt.Before(rows[0].CreatedAt) {
		t.Errorf("orden incorrecto: rows[1] antes que rows[0]")
	}
}
```

- [ ] **Step 2: Correr el test para verificar que NO compila (tipos generados ausentes)**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/store/ -run TestCreateAndListMessages`
Expected: FAIL de compilación — `undefined: store.CreateMessageParams` / `q.ListMessages` / `q.CreateMessage`.

- [ ] **Step 3: Escribir la migración**

Crear `api/db/migrations/0008_ai_messages.sql`:

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

- [ ] **Step 4: Escribir las queries sqlc**

Crear `api/db/queries/ai_messages.sql`:

```sql
-- name: ListMessages :many
SELECT * FROM ai_messages
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO ai_messages (user_id, role, content)
VALUES ($1, $2, $3)
RETURNING *;
```

- [ ] **Step 5: Generar el código del store**

Run: `cd api && GOTOOLCHAIN=local sqlc generate`
Expected: sin errores; aparece `api/internal/store/ai_messages.sql.go` y `models.go` gana el struct `AiMessage` (campos `ID uuid.UUID`, `UserID uuid.UUID`, `Role string`, `Content string`, `CreatedAt time.Time`). `ListMessages` se genera con firma `ListMessages(ctx, userID uuid.UUID) ([]AiMessage, error)` (un solo parámetro, sin struct Params). `CreateMessage` genera `CreateMessageParams{UserID uuid.UUID, Role string, Content string}` y devuelve `AiMessage`.

- [ ] **Step 6: Correr el test contra la DB para verificar que pasa**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/store/ -run TestCreateAndListMessages -v`
Expected: PASS (`testutil.NewDB` aplica las migraciones, incluida la 0008). Requiere el contenedor `db` arriba.

- [ ] **Step 7: Commit**

```bash
cd api && git add db/migrations/0008_ai_messages.sql db/queries/ai_messages.sql internal/store/ai_messages.sql.go internal/store/models.go internal/store/ai_messages_test.go
git commit -m "feat(ai): tabla ai_messages para el chat (migración + queries + store)"
```

---

### Task 2: `GroqClient.Chat` (modo conversacional)

**Files:**
- Modify: `api/internal/ai/groq.go`
- Test: `api/internal/ai/groq_test.go` (agregar)

- [ ] **Step 1: Escribir los tests de Chat (envía system + history; propaga errores)**

Agregar al final de `api/internal/ai/groq_test.go`:

```go
func TestGroqChatOK(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Vas bien."}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
	got, err := c.Chat(context.Background(), "sys", []ChatMsg{
		{Role: "user", Content: "hola"},
		{Role: "assistant", Content: "qué tal"},
		{Role: "user", Content: "¿cómo voy?"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "Vas bien." {
		t.Errorf("content = %q", got)
	}
	// El body debe llevar el system primero y luego el history en orden.
	body := string(gotBody)
	for _, want := range []string{`"role":"system"`, `"content":"sys"`, `"content":"hola"`, `"content":"¿cómo voy?"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Chat(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}
```

Agregar los imports `"io"` y `"strings"` al bloque `import` de `api/internal/ai/groq_test.go` si no están ya presentes.

- [ ] **Step 2: Correr los tests para verificar que fallan a compilación**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestGroqChat`
Expected: FAIL de compilación — `undefined: ChatMsg` / `c.Chat`.

- [ ] **Step 3: Refactor DRY del POST + agregar `Chat`**

En `api/internal/ai/groq.go`, reemplazar el cuerpo de `Complete` para que delegue en un helper privado, y agregar `ChatMsg` + `Chat`. Reemplazar el bloque desde `// Complete envía...` hasta el final del archivo por:

```go
// ChatMsg es un turno de la conversación (rol + contenido) para el modo chat.
type ChatMsg struct {
	Role    string
	Content string
}

// Complete envía system+user a Groq y devuelve choices[0].message.content.
func (c *GroqClient) Complete(ctx context.Context, system, user string) (string, error) {
	return c.send(ctx, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, 200)
}

// Chat envía el system + el historial completo (estilo OpenAI) y devuelve la
// respuesta. max_tokens un poco mayor que Complete para respuestas conversacionales.
func (c *GroqClient) Chat(ctx context.Context, system string, history []ChatMsg) (string, error) {
	msgs := make([]chatMessage, 0, len(history)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: system})
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	return c.send(ctx, msgs, 400)
}

// send hace el POST a /chat/completions y devuelve el contenido del primer choice.
func (c *GroqClient) send(ctx context.Context, msgs []chatMessage, maxTokens int) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.7,
		MaxTokens:   maxTokens,
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

- [ ] **Step 4: Correr todos los tests del paquete ai (Complete sigue verde + Chat pasa)**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestGroq -v`
Expected: PASS — los `TestGroqComplete*` existentes siguen verdes (comportamiento intacto) y `TestGroqChatOK`/`TestGroqChatHTTPError` pasan.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/ai/groq.go internal/ai/groq_test.go
git commit -m "feat(ai): GroqClient.Chat para conversación multi-turno"
```

---

### Task 3: Constructor de contexto del chat (`chatcontext.go`)

**Files:**
- Create: `api/internal/ai/chatcontext.go`
- Test: `api/internal/ai/chatcontext_test.go`

- [ ] **Step 1: Escribir el test del constructor (compone snapshot + ciclos + check-ins con fakes)**

Crear `api/internal/ai/chatcontext_test.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/finance"
	"github.com/google/uuid"
)

type fakeSnap struct {
	snap *dashboard.Snapshot
	err  error
}

func (f fakeSnap) Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*dashboard.Snapshot, error) {
	return f.snap, f.err
}

type fakeCycler struct {
	cycles []finance.CycleSummary
	err    error
}

func (f fakeCycler) Cycles(ctx context.Context, userID uuid.UUID, now time.Time) ([]finance.CycleSummary, error) {
	return f.cycles, f.err
}

type fakeLister struct {
	list  []checkin.CheckIn
	err   error
	limit int
}

func (f *fakeLister) List(ctx context.Context, userID uuid.UUID, limit int) ([]checkin.CheckIn, error) {
	f.limit = limit
	return f.list, f.err
}

func TestChatContextComposesJSON(t *testing.T) {
	snap := &dashboard.Snapshot{
		Streak:  dashboard.StreakView{BestCurrent: 12, DoneToday: 1, Total: 3},
		Finance: dashboard.FinanceView{Cycle: "2026-06", Net: 5000, Status: "pendiente"},
	}
	cyc := []finance.CycleSummary{
		{Cycle: "2026-05", Income: 10000, Expense: 8000, Net: 2000, Status: "verde"},
	}
	cks := []checkin.CheckIn{
		{ID: "c1", Date: "2026-06-10", Mood: 7, Energy: 6, Discipline: 8},
	}
	lister := &fakeLister{list: cks}

	b := newChatContextBuilder(fakeSnap{snap: snap}, fakeCycler{cycles: cyc}, lister)
	out, err := b.build(context.Background(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// Pide los últimos 14 check-ins.
	if lister.limit != 14 {
		t.Errorf("limit = %d, want 14", lister.limit)
	}

	var payload struct {
		Snapshot *dashboard.Snapshot     `json:"snapshot"`
		Cycles   []finance.CycleSummary  `json:"cycles"`
		Checkins []checkin.CheckIn       `json:"checkins"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, out)
	}
	if payload.Snapshot == nil || payload.Snapshot.Streak.BestCurrent != 12 {
		t.Errorf("snapshot mal compuesto: %s", out)
	}
	if len(payload.Cycles) != 1 || payload.Cycles[0].Cycle != "2026-05" {
		t.Errorf("cycles mal compuesto: %s", out)
	}
	if len(payload.Checkins) != 1 || payload.Checkins[0].Date != "2026-06-10" {
		t.Errorf("checkins mal compuesto: %s", out)
	}
}

func TestChatContextPropagatesError(t *testing.T) {
	b := newChatContextBuilder(
		fakeSnap{err: errors.New("db caída")},
		fakeCycler{},
		&fakeLister{},
	)
	if _, err := b.build(context.Background(), uuid.New(), time.Now()); err == nil {
		t.Fatal("esperaba propagar el error de Snapshot")
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla a compilación**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestChatContext`
Expected: FAIL de compilación — `undefined: newChatContextBuilder`.

- [ ] **Step 3: Escribir el constructor de contexto**

Crear `api/internal/ai/chatcontext.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/google/uuid"
)

// recentCheckins es cuántos check-ins recientes incluimos en el contexto.
const recentCheckins = 14

// cycler es la porción de finance.Service que usamos (histórico de ciclos).
type cycler interface {
	Cycles(ctx context.Context, userID uuid.UUID, now time.Time) ([]finance.CycleSummary, error)
}

// checkinLister es la porción de checkin.Service que usamos (check-ins recientes).
type checkinLister interface {
	List(ctx context.Context, userID uuid.UUID, limit int) ([]checkin.CheckIn, error)
}

// chatContextBuilder arma el JSON de contexto que recibe la IA: snapshot del día
// + histórico financiero + check-ins recientes. Reutiliza la interfaz
// snapshotter ya definida en service.go (DRY).
type chatContextBuilder struct {
	dash    snapshotter
	finance cycler
	checkin checkinLister
}

// NewChatContextBuilder inyecta el dashboard (snapshot), finanzas (ciclos) y
// check-ins. Exportado para el wiring en server.go.
func NewChatContextBuilder(d snapshotter, f cycler, c checkinLister) *chatContextBuilder {
	return &chatContextBuilder{dash: d, finance: f, checkin: c}
}

// newChatContextBuilder es el alias interno usado por los tests.
func newChatContextBuilder(d snapshotter, f cycler, c checkinLister) *chatContextBuilder {
	return NewChatContextBuilder(d, f, c)
}

// build compone el contexto en un JSON compacto. Propaga errores reales (DB).
func (b *chatContextBuilder) build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error) {
	snap, err := b.dash.Snapshot(ctx, userID, today)
	if err != nil {
		return "", err
	}
	cycles, err := b.finance.Cycles(ctx, userID, today)
	if err != nil {
		return "", err
	}
	checkins, err := b.checkin.List(ctx, userID, recentCheckins)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]any{
		"snapshot": snap,
		"cycles":   cycles,
		"checkins": checkins,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

- [ ] **Step 4: Correr el test para verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestChatContext -v`
Expected: PASS (no necesita DB: usa fakes).

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/ai/chatcontext.go internal/ai/chatcontext_test.go
git commit -m "feat(ai): constructor de contexto del chat (snapshot + ciclos + check-ins)"
```

---

### Task 4: Prompt de chat (`chatprompt.go`)

**Files:**
- Create: `api/internal/ai/chatprompt.go`
- Test: `api/internal/ai/chatprompt_test.go`

- [ ] **Step 1: Escribir el test del prompt (español, solo-datos, incrusta el JSON)**

Crear `api/internal/ai/chatprompt_test.go`:

```go
package ai

import (
	"strings"
	"testing"
)

func TestBuildChatSystemPrompt(t *testing.T) {
	ctxJSON := `{"snapshot":{"streak":{"best_current":12}}}`
	got := buildChatSystemPrompt(ctxJSON)

	// Incrusta el JSON de contexto literal.
	if !strings.Contains(got, ctxJSON) {
		t.Errorf("el prompt no incrusta el contexto:\n%s", got)
	}
	// Instruye español, concisión y no inventar.
	for _, want := range []string{"español", "ÚNICAMENTE", "inventes"} {
		if !strings.Contains(got, want) {
			t.Errorf("el prompt no menciona %q:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla a compilación**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestBuildChatSystemPrompt`
Expected: FAIL de compilación — `undefined: buildChatSystemPrompt`.

- [ ] **Step 3: Escribir el prompt**

Crear `api/internal/ai/chatprompt.go`:

```go
package ai

import "fmt"

// buildChatSystemPrompt arma el system prompt del chat: coach cálido y conciso,
// responde en español usando SOLO los datos del contexto, sin inventar.
func buildChatSystemPrompt(contextJSON string) string {
	return fmt.Sprintf(`Eres el asistente personal de Focus 365, un coach cálido y conciso.
Respondes SIEMPRE en español, en tono cercano y directo.
Usa ÚNICAMENTE los datos del contexto que sigue. Si un dato no está disponible, dilo con honestidad; nunca inventes cifras ni hechos.
Sé breve (2-4 frases) salvo que el usuario pida detalle.

Contexto del usuario (JSON):
%s`, contextJSON)
}
```

- [ ] **Step 4: Correr el test para verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestBuildChatSystemPrompt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/ai/chatprompt.go internal/ai/chatprompt_test.go
git commit -m "feat(ai): system prompt del chat (español, solo-datos)"
```

---

### Task 5: Tipo `Message` + `ChatService` (`chat.go`)

**Files:**
- Modify: `api/internal/ai/types.go`
- Create: `api/internal/ai/chat.go`
- Test: `api/internal/ai/chat_test.go`

- [ ] **Step 1: Escribir el test del servicio (éxito persiste par; fallo no persiste; sin clave degrada; multi-turno)**

Crear `api/internal/ai/chat_test.go`:

```go
package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// fakeChatGroq registra lo que recibió y devuelve out/err.
type fakeChatGroq struct {
	out         string
	err         error
	called      int
	lastSystem  string
	lastHistory []ChatMsg
}

func (f *fakeChatGroq) Chat(ctx context.Context, system string, history []ChatMsg) (string, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	return f.out, f.err
}

// fakeCtx devuelve un JSON fijo.
type fakeCtx struct {
	out string
	err error
}

func (f fakeCtx) build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error) {
	return f.out, f.err
}

// memStore es un messageStore en memoria (sin DB) por usuario.
type memStore struct {
	rows []store.AiMessage
}

func (m *memStore) ListMessages(ctx context.Context, userID uuid.UUID) ([]store.AiMessage, error) {
	out := make([]store.AiMessage, 0, len(m.rows))
	for _, r := range m.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) CreateMessage(ctx context.Context, arg store.CreateMessageParams) (store.AiMessage, error) {
	row := store.AiMessage{
		ID: uuid.New(), UserID: arg.UserID, Role: arg.Role, Content: arg.Content,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, row)
	return row, nil
}

func TestChatSendPersistsPairAndReturnsAssistant(t *testing.T) {
	groq := &fakeChatGroq{out: "Vas verde este ciclo."}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{"snapshot":{}}`}, st, groq, true)
	uid := uuid.New()

	msg, err := svc.Send(context.Background(), uid, "¿cómo voy?", time.Now())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg.Role != "assistant" || msg.Content != "Vas verde este ciclo." {
		t.Errorf("reply = %+v", msg)
	}
	// Persistió pregunta + respuesta (2 filas).
	if len(st.rows) != 2 {
		t.Fatalf("persistió %d filas, want 2", len(st.rows))
	}
	if st.rows[0].Role != "user" || st.rows[0].Content != "¿cómo voy?" {
		t.Errorf("fila 0 = %+v", st.rows[0])
	}
	if st.rows[1].Role != "assistant" {
		t.Errorf("fila 1 = %+v", st.rows[1])
	}
	// El system incrusta el contexto.
	if groq.lastSystem == "" {
		t.Error("system vacío")
	}
}

func TestChatSendMultiTurnPassesHistory(t *testing.T) {
	groq := &fakeChatGroq{out: "ok"}
	uid := uuid.New()
	st := &memStore{rows: []store.AiMessage{
		{ID: uuid.New(), UserID: uid, Role: "user", Content: "hola", CreatedAt: time.Now()},
		{ID: uuid.New(), UserID: uid, Role: "assistant", Content: "qué tal", CreatedAt: time.Now().Add(time.Millisecond)},
	}}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, true)

	if _, err := svc.Send(context.Background(), uid, "¿cómo voy?", time.Now()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// history = 2 previos + el nuevo del usuario = 3, y el último es el nuevo.
	if len(groq.lastHistory) != 3 {
		t.Fatalf("history len = %d, want 3", len(groq.lastHistory))
	}
	last := groq.lastHistory[2]
	if last.Role != "user" || last.Content != "¿cómo voy?" {
		t.Errorf("último turno = %+v", last)
	}
}

func TestChatSendNoKeyDegrades(t *testing.T) {
	groq := &fakeChatGroq{out: "no usar"}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, false)

	_, err := svc.Send(context.Background(), uuid.New(), "hola", time.Now())
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if groq.called != 0 {
		t.Error("sin clave no debe llamar a Groq")
	}
	if len(st.rows) != 0 {
		t.Error("sin clave no debe persistir nada")
	}
}

func TestChatSendGroqFailureDoesNotPersist(t *testing.T) {
	groq := &fakeChatGroq{err: errors.New("groq caído")}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, true)

	_, err := svc.Send(context.Background(), uuid.New(), "hola", time.Now())
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("fallo de Groq no debe dejar mensajes huérfanos")
	}
}

func TestChatHistoryMapsRows(t *testing.T) {
	uid := uuid.New()
	st := &memStore{rows: []store.AiMessage{
		{ID: uuid.New(), UserID: uid, Role: "user", Content: "hola", CreatedAt: time.Now()},
	}}
	svc := NewChatService(fakeCtx{}, st, &fakeChatGroq{}, true)

	msgs, err := svc.History(context.Background(), uid)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hola" {
		t.Errorf("history = %+v", msgs)
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla a compilación**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestChat`
Expected: FAIL de compilación — `undefined: NewChatService` / `ErrUnavailable` / `Message`.

- [ ] **Step 3: Agregar el tipo `Message` a types.go**

Agregar al final de `api/internal/ai/types.go`:

```go
// Message es un mensaje del chat (vista que se serializa a JSON).
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **Step 4: Escribir el servicio de chat**

Crear `api/internal/ai/chat.go`:

```go
package ai

import (
	"context"
	"errors"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// ErrUnavailable indica que la IA no está disponible (sin clave o fallo de Groq).
// El handler lo traduce a 503.
var ErrUnavailable = errors.New("asistente no disponible")

// chatHistoryLimit es cuántos turnos previos enviamos a Groq como contexto
// conversacional (la cola del historial).
const chatHistoryLimit = 10

// chatCompleter abstrae la llamada de chat a Groq (testeable con fake).
type chatCompleter interface {
	Chat(ctx context.Context, system string, history []ChatMsg) (string, error)
}

// messageStore es la porción de store.Queries para los mensajes del chat.
type messageStore interface {
	ListMessages(ctx context.Context, userID uuid.UUID) ([]store.AiMessage, error)
	CreateMessage(ctx context.Context, arg store.CreateMessageParams) (store.AiMessage, error)
}

// contextBuilder abstrae el armado del contexto (lo implementa chatContextBuilder).
type contextBuilder interface {
	build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error)
}

// ChatService orquesta la conversación: contexto + historial + Groq + persistencia.
type ChatService struct {
	ctxb   contextBuilder
	store  messageStore
	groq   chatCompleter
	hasKey bool
}

// NewChatService inyecta el constructor de contexto, el store de mensajes, el
// cliente de chat (Groq o fake) y si hay clave configurada.
func NewChatService(ctxb contextBuilder, q messageStore, c chatCompleter, hasKey bool) *ChatService {
	return &ChatService{ctxb: ctxb, store: q, groq: c, hasKey: hasKey}
}

// History devuelve el historial completo del usuario, mapeado a la vista.
func (s *ChatService) History(ctx context.Context, userID uuid.UUID) ([]Message, error) {
	rows, err := s.store.ListMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	return mapMessages(rows), nil
}

// Send procesa una pregunta: arma contexto, carga la cola del historial, llama a
// Groq y solo ante éxito persiste el par pregunta+respuesta. Devuelve la
// respuesta del asistente. Degrada a ErrUnavailable sin clave o ante fallo de IA.
func (s *ChatService) Send(ctx context.Context, userID uuid.UUID, text string, today time.Time) (*Message, error) {
	if !s.hasKey {
		return nil, ErrUnavailable
	}

	contextJSON, err := s.ctxb.build(ctx, userID, today)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.ListMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	history := buildHistory(rows, text)

	reply, err := s.groq.Chat(ctx, buildChatSystemPrompt(contextJSON), history)
	if err != nil {
		return nil, ErrUnavailable
	}

	// Solo ante éxito persistimos el par (evita mensajes de usuario huérfanos).
	if _, err := s.store.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "user", Content: text,
	}); err != nil {
		return nil, err
	}
	assistant, err := s.store.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "assistant", Content: reply,
	})
	if err != nil {
		return nil, err
	}
	v := Message{Role: assistant.Role, Content: assistant.Content, CreatedAt: assistant.CreatedAt}
	return &v, nil
}

// buildHistory toma la cola del historial (últimos chatHistoryLimit) y agrega el
// mensaje nuevo del usuario al final, en formato ChatMsg para Groq.
func buildHistory(rows []store.AiMessage, newText string) []ChatMsg {
	start := 0
	if len(rows) > chatHistoryLimit {
		start = len(rows) - chatHistoryLimit
	}
	out := make([]ChatMsg, 0, chatHistoryLimit+1)
	for _, r := range rows[start:] {
		out = append(out, ChatMsg{Role: r.Role, Content: r.Content})
	}
	out = append(out, ChatMsg{Role: "user", Content: newText})
	return out
}

func mapMessages(rows []store.AiMessage) []Message {
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, Message{Role: r.Role, Content: r.Content, CreatedAt: r.CreatedAt})
	}
	return out
}
```

- [ ] **Step 5: Correr los tests del servicio para verificar que pasan**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestChat -v`
Expected: PASS (no necesita DB: usa fakes/memStore).

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/ai/types.go internal/ai/chat.go internal/ai/chat_test.go
git commit -m "feat(ai): ChatService multi-turno con persistencia solo-ante-éxito"
```

---

### Task 6: Endpoints `GET /ai/messages` y `POST /ai/chat` + wiring

**Files:**
- Modify: `api/internal/ai/handler.go`
- Modify: `api/internal/ai/handler_test.go` (actualizar `newEnv` y el fake a la nueva firma de `Routes`)
- Modify: `api/internal/server/server.go`
- Test: `api/internal/ai/chat_handler_test.go`

- [ ] **Step 1: Actualizar el fake y `newEnv` de handler_test.go a la nueva firma**

En `api/internal/ai/handler_test.go`, hacer dos cambios:

(a) Extender `fakeCompleter` para que también implemente `Chat` (lo usará el chat). Reemplazar el método `Complete` existente y agregar campos/método de chat — reemplazar el bloque:

```go
type fakeCompleter struct {
	out    string
	err    error
	called int
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.called++
	return f.out, f.err
}
```

por:

```go
type fakeCompleter struct {
	out    string
	err    error
	called int

	chatOut    string
	chatErr    error
	chatCalled int
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.called++
	return f.out, f.err
}

func (f *fakeCompleter) Chat(ctx context.Context, system string, history []ai.ChatMsg) (string, error) {
	f.chatCalled++
	return f.chatOut, f.chatErr
}
```

(b) En `newEnv`, construir el `ChatService` y pasarlo a `Routes`. Reemplazar el bloque:

```go
	svc := ai.NewService(dash, q, comp, hasKey)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/ai", ai.Routes(svc))
	})
```

por:

```go
	svc := ai.NewService(dash, q, comp, hasKey)
	chatCtx := ai.NewChatContextBuilder(dash, fi, ci)
	chatSvc := ai.NewChatService(chatCtx, q, comp, hasKey)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/ai", ai.Routes(svc, chatSvc))
	})
```

- [ ] **Step 2: Escribir los tests de integración del chat**

Crear `api/internal/ai/chat_handler_test.go`:

```go
package ai_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/focus365/api/internal/ai"
)

func postChat(t *testing.T, h http.Handler, tok, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ai/chat?today="+today, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func getMessages(t *testing.T, h http.Handler, tok string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ai/messages", nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestChatHappyPathPersists(t *testing.T) {
	comp := &fakeCompleter{chatOut: "Vas verde este ciclo."}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "chat@b.com")

	rec, body := postChat(t, e.h, tok, `{"message":"¿cómo voy?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	reply, _ := body["reply"].(map[string]any)
	if reply["role"] != "assistant" || reply["content"] != "Vas verde este ciclo." {
		t.Errorf("reply = %v", body["reply"])
	}

	// GET /messages devuelve el par persistido en orden ASC.
	rec2, body2 := getMessages(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("messages code = %d", rec2.Code)
	}
	msgs, _ := body2["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "¿cómo voy?" {
		t.Errorf("primer mensaje = %v", msgs[0])
	}
}

func TestChatRequiresAuth(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	if rec, _ := postChat(t, e.h, "", `{"message":"hola"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST sin token code = %d, want 401", rec.Code)
	}
	if rec, _ := getMessages(t, e.h, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET sin token code = %d, want 401", rec.Code)
	}
}

func TestChatValidationRejectsEmpty(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	_, tok := e.user(t, "empty@b.com")

	// Falta el campo message.
	if rec, _ := postChat(t, e.h, tok, `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("body vacío code = %d, want 400", rec.Code)
	}
	// Solo espacios (vacío tras trim).
	if rec, _ := postChat(t, e.h, tok, `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("solo espacios code = %d, want 400", rec.Code)
	}
}

func TestChatNoKeyDegrades(t *testing.T) {
	e := newEnv(t, false, &fakeCompleter{chatOut: "no usar"})
	_, tok := e.user(t, "nokeychat@b.com")

	rec, _ := postChat(t, e.h, tok, `{"message":"hola"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("sin clave code = %d, want 503", rec.Code)
	}
	// No persistió nada: el historial sigue vacío.
	_, body := getMessages(t, e.h, tok)
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 0 {
		t.Errorf("degradado no debe persistir, got %d mensajes", len(msgs))
	}
}

func TestChatEmptyHistory(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	_, tok := e.user(t, "fresh@b.com")

	rec, body := getMessages(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 0 {
		t.Errorf("historial fresco = %v, want []", body["messages"])
	}
}
```

- [ ] **Step 3: Correr los tests para verificar que fallan a compilación**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/ai/ -run TestChatHappyPath`
Expected: FAIL de compilación — `ai.Routes` espera 1 argumento pero `newEnv` ahora pasa 2 (firma vieja), y faltan los handlers.

- [ ] **Step 4: Agregar los handlers y la nueva firma de `Routes`**

Reemplazar `api/internal/ai/handler.go` completo por:

```go
package ai

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

const dateLayout = "2006-01-02"

// Routes monta los endpoints del asistente (bajo RequireAuth en server.go):
// el insight proactivo y el chat on-demand.
func Routes(svc *Service, chat *ChatService) http.Handler {
	r := chi.NewRouter()
	r.Get("/insight", handleInsight(svc))
	r.Get("/messages", handleMessages(chat))
	r.Post("/chat", handleChat(chat))
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

// messagesResponse envuelve el historial del chat.
type messagesResponse struct {
	Messages []Message `json:"messages"`
}

func handleMessages(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		msgs, err := chat.History(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, messagesResponse{Messages: msgs})
	}
}

// chatRequestBody es el body de POST /ai/chat.
type chatRequestBody struct {
	Message string `json:"message" validate:"required,max=2000"`
}

// chatReplyResponse envuelve la respuesta del asistente.
type chatReplyResponse struct {
	Reply Message `json:"reply"`
}

func handleChat(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req chatRequestBody
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		// Rechazamos mensajes vacíos tras trim (el validator `required` deja pasar
		// cadenas de solo espacios).
		req.Message = strings.TrimSpace(req.Message)
		if req.Message == "" {
			httpx.WriteErr(w, http.StatusBadRequest, "Falta el mensaje")
			return
		}
		reply, err := chat.Send(r.Context(), userID, req.Message, parseTodayParam(r))
		if err != nil {
			if errors.Is(err, ErrUnavailable) {
				httpx.WriteErr(w, http.StatusServiceUnavailable, "asistente no disponible por ahora")
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, chatReplyResponse{Reply: *reply})
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

- [ ] **Step 5: Actualizar el wiring en server.go**

En `api/internal/server/server.go`, reemplazar el bloque:

```go
			groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel)
			aiSvc := ai.NewService(dashboardSvc, q, groq, d.GroqAPIKey != "")
			r.Mount("/ai", ai.Routes(aiSvc))
```

por:

```go
			groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel)
			aiSvc := ai.NewService(dashboardSvc, q, groq, d.GroqAPIKey != "")
			chatCtx := ai.NewChatContextBuilder(dashboardSvc, financeSvc, checkinSvc)
			chatSvc := ai.NewChatService(chatCtx, q, groq, d.GroqAPIKey != "")
			r.Mount("/ai", ai.Routes(aiSvc, chatSvc))
```

- [ ] **Step 6: Correr todo el paquete ai + el server contra la DB**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/ai/ ./internal/server/ -v`
Expected: PASS — los tests del insight (Task 8 previa) siguen verdes y los nuevos `TestChat*` pasan. Requiere el contenedor `db` arriba.

- [ ] **Step 7: `make check` completo**

Run: `cd api && make check`
Expected: `go vet ./...` limpio y `go test -p 1 ./...` todo verde.

- [ ] **Step 8: Commit**

```bash
cd api && git add internal/ai/handler.go internal/ai/handler_test.go internal/ai/chat_handler_test.go internal/server/server.go
git commit -m "feat(ai): endpoints GET /ai/messages y POST /ai/chat + wiring"
```

---

### Task 7: Lib frontend del chat (`web/src/lib/ai.ts`)

**Files:**
- Modify: `web/src/lib/ai.ts`
- Test: `web/src/lib/ai.test.ts` (agregar)

- [ ] **Step 1: Escribir los tests de `getMessages` y `sendMessage`**

Agregar al final de `web/src/lib/ai.test.ts`, dentro del archivo (después del `describe` existente):

```ts
import { getMessages, sendMessage, type Message } from "./ai";

describe("getMessages", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/ai/messages y devuelve el array", async () => {
    const messages: Message[] = [
      { role: "user", content: "hola", created_at: "2026-06-11T10:00:00Z" },
      { role: "assistant", content: "qué tal", created_at: "2026-06-11T10:00:01Z" },
    ];
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ messages })
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await getMessages();
    expect(got).toEqual(messages);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/ai/messages");
    const opts = fetchMock.mock.calls[0][1];
    expect(opts?.method ?? "GET").toBe("GET");
  });
});

describe("sendMessage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("hace POST a /api/v1/ai/chat con el mensaje y devuelve el reply", async () => {
    const reply: Message = {
      role: "assistant",
      content: "Vas verde este ciclo.",
      created_at: "2026-06-11T10:00:02Z",
    };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ reply })
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await sendMessage("¿cómo voy?");
    expect(got).toEqual(reply);

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/ai/chat");
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ message: "¿cómo voy?" });
  });
});
```

- [ ] **Step 2: Correr los tests para verificar que fallan**

Run: `cd web && npm test -- src/lib/ai.test.ts`
Expected: FAIL — `getMessages`/`sendMessage`/`Message` no exportados.

- [ ] **Step 3: Agregar los tipos y funciones a la lib**

Agregar al final de `web/src/lib/ai.ts`:

```ts
export type Message = {
  role: string;
  content: string;
  created_at: string;
};

export function getMessages(): Promise<Message[]> {
  return apiFetch<{ messages: Message[] }>("/api/v1/ai/messages").then(
    (r) => r.messages
  );
}

export function sendMessage(message: string): Promise<Message> {
  return apiFetch<{ reply: Message }>("/api/v1/ai/chat", {
    method: "POST",
    body: JSON.stringify({ message }),
  }).then((r) => r.reply);
}
```

- [ ] **Step 4: Correr los tests para verificar que pasan**

Run: `cd web && npm test -- src/lib/ai.test.ts`
Expected: PASS (los 3 tests: getInsight, getMessages, sendMessage).

- [ ] **Step 5: Commit**

```bash
cd web && git add src/lib/ai.ts src/lib/ai.test.ts
git commit -m "feat(web): lib del chat IA (getMessages, sendMessage)"
```

---

### Task 8: Ruta `/asistente` + nav + link del dashboard

**Files:**
- Create: `web/src/routes/asistente.tsx`
- Test: `web/src/routes/asistente.test.tsx`
- Modify: `web/src/components/TopBar.tsx`
- Modify: `web/src/routes/index.tsx`

- [ ] **Step 1: Escribir el test de la ruta (renderiza historial; enviar agrega el par; error no rompe)**

Crear `web/src/routes/asistente.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";

// Usuario autenticado falso para evitar el redirect a /login.
vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as AsistenteRoute } from "./asistente";

function renderPage() {
  const rootRoute = createRootRoute();
  const asistenteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/asistente",
    component: AsistenteRoute.options.component,
  });
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([asistenteRoute, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/asistente"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("AsistentePage", () => {
  afterEach(() => vi.restoreAllMocks());

  it("renderiza el historial existente", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              messages: [
                { role: "user", content: "¿cómo voy?", created_at: "2026-06-11T10:00:00Z" },
                { role: "assistant", content: "Vas verde.", created_at: "2026-06-11T10:00:01Z" },
              ],
            }),
            { status: 200 }
          )
        )
      )
    );
    renderPage();
    expect(await screen.findByText("¿cómo voy?")).toBeInTheDocument();
    expect(screen.getByText("Vas verde.")).toBeInTheDocument();
  });

  it("al enviar dispara un POST con el mensaje", async () => {
    const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
      if (opts?.method === "POST") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              reply: { role: "assistant", content: "Respuesta.", created_at: "2026-06-11T10:00:02Z" },
            }),
            { status: 200 }
          )
        );
      }
      return Promise.resolve(new Response(JSON.stringify({ messages: [] }), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    const input = await screen.findByLabelText("Mensaje");
    await userEvent.type(input, "hola");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    await waitFor(() => {
      const posted = fetchMock.mock.calls.some(
        ([url, opts]) => url === "/api/v1/ai/chat" && opts?.method === "POST"
      );
      expect(posted).toBe(true);
    });
  });

  it("muestra error inline sin romper la página cuando el POST falla", async () => {
    const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
      if (opts?.method === "POST") {
        return Promise.resolve(
          new Response(JSON.stringify({ error: "asistente no disponible por ahora" }), { status: 503 })
        );
      }
      return Promise.resolve(new Response(JSON.stringify({ messages: [] }), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    const input = (await screen.findByLabelText("Mensaje")) as HTMLInputElement;
    await userEvent.type(input, "hola");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

    expect(await screen.findByText(/no disponible/i)).toBeInTheDocument();
    // El texto tecleado no se pierde (permite reintentar).
    expect(input.value).toBe("hola");
  });
});
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd web && npm test -- src/routes/asistente.test.tsx`
Expected: FAIL — `./asistente` no existe.

- [ ] **Step 3: Escribir la ruta**

Crear `web/src/routes/asistente.tsx`:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getMessages, sendMessage, type Message } from "@/lib/ai";

export const Route = createFileRoute("/asistente")({ component: AsistentePage });

function AsistentePage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const historyQuery = useQuery({
    queryKey: ["ai-messages"],
    queryFn: getMessages,
    enabled: !!user,
  });

  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: (message: string) => sendMessage(message),
    onSuccess: () => {
      setError(null);
      setText("");
      qc.invalidateQueries({ queryKey: ["ai-messages"] });
    },
    onError: (err) =>
      // No limpiamos el input: el usuario puede reintentar sin reescribir.
      setError(err instanceof Error ? err.message : "No se pudo enviar"),
  });

  if (!user) return null;

  const messages = historyQuery.data ?? [];

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Asistente</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      <section className="flex flex-col gap-2">
        {messages.length === 0 ? (
          <p className="text-sm text-sand-400">
            Pregúntame sobre tu día, tus finanzas o tus hábitos.
          </p>
        ) : (
          messages.map((m: Message, i: number) => (
            <div
              key={i}
              className={
                m.role === "user"
                  ? "self-end rounded-lg bg-amber-brand/20 px-3 py-2 text-sm text-sand-100"
                  : "self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-100"
              }
            >
              {m.content}
            </div>
          ))
        )}
        {mutation.isPending && (
          <div className="self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-400">
            Pensando…
          </div>
        )}
      </section>

      {error && <p className="text-sm text-streak">{error}</p>}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          const t = text.trim();
          if (t) mutation.mutate(t);
        }}
        className="flex gap-2"
      >
        <input
          type="text"
          aria-label="Mensaje"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Escribe tu pregunta…"
          className="flex-1 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
        />
        <button
          type="submit"
          disabled={mutation.isPending || text.trim() === ""}
          className="rounded-lg bg-amber-brand px-4 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          Enviar
        </button>
      </form>
    </div>
  );
}
```

- [ ] **Step 4: Correr el test de la ruta para verificar que pasa**

Run: `cd web && npm test -- src/routes/asistente.test.tsx`
Expected: PASS (los 3 tests).

- [ ] **Step 5: Agregar la entrada de nav en el TopBar**

En `web/src/components/TopBar.tsx`, reemplazar el array `LINKS`:

```tsx
const LINKS: { to: string; label: string }[] = [
  { to: "/", label: "Inicio" },
  { to: "/check-in", label: "Check-in" },
  { to: "/finanzas", label: "Finanzas" },
  { to: "/entrenamiento", label: "Entreno" },
  { to: "/disciplina", label: "Disciplina" },
  { to: "/metas", label: "Metas" },
];
```

por:

```tsx
const LINKS: { to: string; label: string }[] = [
  { to: "/", label: "Inicio" },
  { to: "/check-in", label: "Check-in" },
  { to: "/finanzas", label: "Finanzas" },
  { to: "/entrenamiento", label: "Entreno" },
  { to: "/disciplina", label: "Disciplina" },
  { to: "/metas", label: "Metas" },
  { to: "/asistente", label: "Asistente" },
];
```

- [ ] **Step 6: Linkear la banda de IA del dashboard a `/asistente`**

En `web/src/routes/index.tsx`, hacer que la banda enlace a `/asistente` sin cambiar su contenido. Reemplazar la función `AIBand` por:

```tsx
function AIBand() {
  const { user } = useAuth();
  const insightQ = useQuery({
    queryKey: ["ai-insight", todayString()],
    queryFn: getInsight,
    enabled: !!user,
    // Si la IA falla, degradamos al placeholder sin reintentar: la banda nunca
    // debe quedarse cargando ni golpear repetidamente un endpoint caído.
    retry: false,
  });

  const base =
    "block rounded-lg border border-dashed border-amber-brand bg-amber-brand/10 px-4 py-3 text-sm font-bold text-amber-brand";

  let content = "✦ Tu insight del día llega pronto";
  if (insightQ.isLoading) {
    content = "✦ Generando tu insight…";
  } else if (insightQ.data?.available && insightQ.data.content) {
    content = `✦ ${insightQ.data.content}`;
  }
  return (
    <Link to="/asistente" className={base}>
      {content}
    </Link>
  );
}
```

`Link` ya está importado en `index.tsx` (línea 1: `import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";`).

- [ ] **Step 7: Correr la suite frontend completa + build (tsc estricto)**

Run: `cd web && npm test && npm run build`
Expected: todos los tests Vitest verdes (incluida la ruta nueva y `index.test.tsx` que sigue verificando el texto de la banda) y `tsc -b` + build sin errores de tipo.

> Nota: `index.test.tsx` busca el texto de la banda (p. ej. "Generando tu insight…" o el placeholder). Envolver el contenido en `<Link>` no cambia el texto, así que esos asserts siguen pasando. Si algún assert dependía de que la banda fuera un `<div>`, ajustarlo para que busque por texto/rol de link.

- [ ] **Step 8: Commit**

```bash
cd web && git add src/routes/asistente.tsx src/routes/asistente.test.tsx src/components/TopBar.tsx src/routes/index.tsx
git commit -m "feat(web): ruta /asistente (chat IA) + nav + link de la banda"
```

---

### Task 9: Smoke E2E (docker) + verificación final

**Files:**
- (sin archivos nuevos en el repo; el smoke vive en `/tmp`)

- [ ] **Step 1: Rebuild del stack docker**

Run (en una sola línea, con PATH de docker y sandbox deshabilitado):
```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin" && cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
```
Expected: contenedores `db`, `api`, `web` arriba; el `api` aplica la migración 0008 al arrancar (ver logs: `docker compose logs api | tail`).

- [ ] **Step 2: Escribir el script de smoke del chat**

Crear `/tmp/smoke_chat.sh` (usa `USERID`, no `UID`, que es readonly en el shell):

```bash
#!/usr/bin/env bash
set -euo pipefail
API="http://localhost:8088/api/v1"
EMAIL="smoke-chat-$(date +%s)@b.com"

# 1. Registro → access token.
REG=$(curl -s -X POST "$API/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"p4ssword\",\"name\":\"Smoke\"}")
TOKEN=$(echo "$REG" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$TOKEN" ] || { echo "FALLO: sin access token: $REG"; exit 1; }

# 2. Historial inicial vacío.
MSGS=$(curl -s "$API/ai/messages" -H "Authorization: Bearer $TOKEN")
echo "$MSGS" | grep -q '"messages":\[\]' && echo "historial inicial vacío OK" \
  || { echo "FALLO: historial inicial no vacío: $MSGS"; exit 1; }

# 3. Enviar una pregunta → respuesta del asistente (con clave real responde 200).
CHAT=$(curl -s -X POST "$API/ai/chat" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"message":"¿cómo voy este mes?"}')
echo "$CHAT" | grep -q '"reply"' && echo "respuesta del chat OK" \
  || { echo "FALLO: sin reply: $CHAT"; exit 1; }
echo "$CHAT" | grep -q '"role":"assistant"' \
  || { echo "FALLO: reply no es assistant: $CHAT"; exit 1; }

# 4. El par quedó persistido (historial ahora tiene 2 mensajes).
MSGS2=$(curl -s "$API/ai/messages" -H "Authorization: Bearer $TOKEN")
COUNT=$(echo "$MSGS2" | grep -o '"role"' | wc -l | tr -d ' ')
[ "$COUNT" = "2" ] && echo "par persistido OK (2 mensajes)" \
  || { echo "FALLO: esperaba 2 mensajes, got $COUNT: $MSGS2"; exit 1; }

# 5. Validación: mensaje vacío → 400.
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/ai/chat" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"message":"   "}')
[ "$CODE" = "400" ] && echo "validación vacío 400 OK" \
  || { echo "FALLO: vacío devolvió $CODE, want 400"; exit 1; }

# 6. Sin token → 401.
CODE2=$(curl -s -o /dev/null -w '%{http_code}' "$API/ai/messages")
[ "$CODE2" = "401" ] && echo "sin token 401 OK" \
  || { echo "FALLO: sin token devolvió $CODE2, want 401"; exit 1; }

echo "SMOKE CHAT OK (6 checks)"
```

- [ ] **Step 3: Correr el smoke**

Run: `bash /tmp/smoke_chat.sh`
Expected (si el `.env` tiene `GROQ_API_KEY` real, como en este entorno):
```
historial inicial vacío OK
respuesta del chat OK
par persistido OK (2 mensajes)
validación vacío 400 OK
sin token 401 OK
SMOKE CHAT OK (6 checks)
```

> Si el entorno NO tuviera clave Groq: el paso 3 devolvería 503 (degradado). En ese caso, adaptar el smoke para verificar la degradación (POST /ai/chat → 503 y el historial sigue vacío). El criterio de aceptación cubre ambos caminos.

- [ ] **Step 4: Verificación backend completa**

Run: `cd /Users/gustavo/Desktop/focus-365/api && make check`
Expected: `go vet ./...` limpio y `go test -p 1 ./...` todo verde.

- [ ] **Step 5: Verificación frontend completa**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npm test && npm run build`
Expected: toda la suite Vitest verde y build sin errores de tipo.

- [ ] **Step 6: Verificar árbol limpio**

Run: `cd /Users/gustavo/Desktop/focus-365 && git status`
Expected: sin cambios sin commitear (todo el trabajo de Tasks 1-8 ya está commiteado; el smoke vive en `/tmp`, no en el repo).

---

## Self-Review (ejecutado por el autor del plan)

**1. Cobertura del spec** (`2026-06-11-plan-9-chat-ia-design.md`):
- §3 Modelo de datos (migración + queries) → Task 1. ✓
- §4.1 GroqClient.Chat → Task 2. ✓
- §4.2 Constructor de contexto → Task 3. ✓
- §4.3 Prompt de chat → Task 4. ✓
- §4.4 ChatService (History/Send, persistir solo ante éxito, cola ~10) → Task 5. ✓
- §4.5 Tipo Message → Task 5. ✓
- §5 API (GET /messages, POST /chat, validación required/max 2000/no vacío tras trim) → Task 6. ✓
- §6 Frontend (lib + ruta + nav + link de banda) → Tasks 7-8. ✓
- §7 Manejo de errores (503/400/401/500) → Tasks 6 (handlers) y 8 (UX inline + reintento). ✓
- §8 Testing (store, contexto, prompt, servicio, handler, frontend) → cubierto en cada task. ✓
- §9 Criterios de aceptación → Task 9 (smoke happy/degradado, make check, Vitest+build). ✓

**2. Placeholders:** sin TBD/TODO; cada step de código trae el código completo y cada step de comando trae el comando exacto + salida esperada.

**3. Consistencia de tipos:** `ChatMsg{Role,Content}` (Task 2) usado en `chatCompleter.Chat` (Task 5) y el fake (Task 6). `Message{Role,Content,CreatedAt}` (Task 5) usado en handlers (Task 6) y lib TS (Task 7). `store.CreateMessageParams{UserID,Role,Content}` y `ListMessages(ctx, userID)` (Task 1) usados en `messageStore` (Task 5) y `memStore` (Task 5). `NewChatContextBuilder(snapshotter, cycler, checkinLister)` (Task 3) usado en `newEnv` y server.go (Task 6). `Routes(*Service, *ChatService)` (Task 6) coherente con server.go y handler_test.go. `ErrUnavailable` (Task 5) → 503 (Task 6). Consistente.
