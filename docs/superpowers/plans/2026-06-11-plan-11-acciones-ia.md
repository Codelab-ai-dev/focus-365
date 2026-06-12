# Plan 11 — Acciones de la IA (tool-use con confirmación) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** El chat propone acciones (check-in, movimiento, hábito, meta) como tarjetas con Confirmar/Cancelar; solo al confirmar se escribe vía los servicios existentes.

**Architecture:** Se extiende el camino de streaming: `GroqClient.ChatStream` gana tools y devuelve un `*ToolCall` opcional; `ChatService.SendStream` valida el tool call y persiste el par con la acción `proposed` en `ai_messages` (columnas nuevas); `ConfirmAction`/`CancelAction` + 2 endpoints cierran el ciclo vía un `actionExecutor` que delega en checkin/finance/habits/goals. El evento SSE `done` carga el mensaje completo (ahora con `id` y `action` opcional); no hay evento nuevo.

**Tech Stack:** Go + chi + sqlc/pgx (api), Groq tool-use OpenAI-style, React + TanStack Query + Vitest (web).

**Spec:** `docs/superpowers/specs/2026-06-11-plan-11-acciones-ia-design.md`

**Entorno:**
- Go desde `api/`: prefijo `GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`.
- `sqlc generate` se corre desde `api/` (regenera `internal/store`).
- Tests backend: `go test -p 1 ./...` (DB compartida).
- Docker: `dangerouslyDisableSandbox: true` + `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"`. El api aplica migraciones al arrancar.
- Valores de dominio reales: movimientos usan `income`/`expense` (no «ingreso/gasto»); las validaciones de rango viven en los handlers HTTP (validator tags), así que el ejecutor re-valida por su cuenta.

---

### Task 1: Migración 0009, queries y `CreatePairWithAction`

**Files:**
- Create: `api/db/migrations/0009_ai_actions.sql`
- Modify: `api/db/queries/ai_messages.sql`
- Modify: `api/internal/ai/chatstore.go`
- Test: `api/internal/store/ai_messages_test.go` (ampliar)
- Generated: `api/internal/store/` (vía `sqlc generate`)

- [ ] **Step 1: Escribir la migración**

`api/db/migrations/0009_ai_actions.sql`:

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

- [ ] **Step 2: Agregar las queries**

Al final de `api/db/queries/ai_messages.sql`:

```sql
-- name: CreateMessageWithAction :one
INSERT INTO ai_messages (user_id, role, content, action_kind, action_payload, action_status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetMessageForAction :one
SELECT * FROM ai_messages
WHERE id = $1 AND user_id = $2;

-- name: SetActionStatus :one
UPDATE ai_messages
SET action_status = $3
WHERE id = $1 AND user_id = $2 AND action_status = 'proposed'
RETURNING *;
```

- [ ] **Step 3: Regenerar sqlc**

```bash
cd api && sqlc generate
```
Con `emit_pointers_for_null_types`, `store.AiMessage` gana `ActionKind *string`, `ActionPayload []byte`, `ActionStatus *string` (jsonb → `[]byte`, nil cuando NULL). Verificar con `grep -A10 "type AiMessage struct" internal/store/models.go`.

- [ ] **Step 4: Escribir el test de round-trip que falla**

Agregar a `api/internal/store/ai_messages_test.go` (seguir el patrón existente del archivo: `q.CreateUser(ctx, store.CreateUserParams{Email: ..., PasswordHash: "h", Name: ...})`):

```go
func TestAiMessageActionRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "action-rt@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	kind := "checkin"
	status := "proposed"
	m, err := q.CreateMessageWithAction(ctx, store.CreateMessageWithActionParams{
		UserID: u.ID, Role: "assistant", Content: "Propongo registrar tu check-in.",
		ActionKind: &kind, ActionPayload: []byte(`{"mood":8,"energy":6,"discipline":9}`),
		ActionStatus: &status,
	})
	if err != nil {
		t.Fatalf("CreateMessageWithAction: %v", err)
	}
	if m.ActionKind == nil || *m.ActionKind != "checkin" || m.ActionStatus == nil || *m.ActionStatus != "proposed" {
		t.Errorf("acción mal persistida: %+v", m)
	}

	got, err := q.GetMessageForAction(ctx, store.GetMessageForActionParams{ID: m.ID, UserID: u.ID})
	if err != nil {
		t.Fatalf("GetMessageForAction: %v", err)
	}
	if string(got.ActionPayload) != `{"mood":8,"energy":6,"discipline":9}` &&
		string(got.ActionPayload) != `{"mood": 8, "energy": 6, "discipline": 9}` {
		t.Errorf("payload = %s", got.ActionPayload)
	}

	// Transición válida: proposed → done.
	upd, err := q.SetActionStatus(ctx, store.SetActionStatusParams{ID: m.ID, UserID: u.ID, ActionStatus: ptr("done")})
	if err != nil {
		t.Fatalf("SetActionStatus: %v", err)
	}
	if upd.ActionStatus == nil || *upd.ActionStatus != "done" {
		t.Errorf("status = %v", upd.ActionStatus)
	}

	// Segunda transición: ya no está proposed → ErrNoRows (conflicto).
	if _, err := q.SetActionStatus(ctx, store.SetActionStatusParams{ID: m.ID, UserID: u.ID, ActionStatus: ptr("cancelled")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("doble transición err = %v, want pgx.ErrNoRows", err)
	}

	// Otro usuario no puede tocar la acción.
	otro, _ := q.CreateUser(ctx, store.CreateUserParams{Email: "action-otro@b.com", PasswordHash: "h", Name: "Eve"})
	if _, err := q.GetMessageForAction(ctx, store.GetMessageForActionParams{ID: m.ID, UserID: otro.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("scoping err = %v, want pgx.ErrNoRows", err)
	}
}

func ptr(s string) *string { return &s }
```

(Imports a sumar si faltan: `errors`, `github.com/jackc/pgx/v5`. Si el archivo ya define un helper equivalente a `ptr`, usarlo en su lugar. Nota: el tipo del parámetro `ActionStatus` en `SetActionStatusParams` puede salir de sqlc como `*string` — ajustar la llamada a lo generado.)

- [ ] **Step 5: Verificar que falla, luego correr migración**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestAiMessageActionRoundTrip -v
```
El test usa `testutil.NewDB`, que aplica las migraciones pendientes — la 0009 se aplica sola. Primera corrida tras escribir el test pero antes de `sqlc generate`: FAIL de compilación. Tras sqlc: debe PASAR (la implementación es la query). Si el orden lo permite, correr el test entre Step 2 y Step 3 para ver el rojo.

- [ ] **Step 6: `CreatePairWithAction` en el store del chat**

En `api/internal/ai/chatstore.go`, agregar:

```go
// CreatePairWithAction es CreatePair pero el mensaje del asistente lleva una
// acción propuesta (kind + payload + status 'proposed'). Misma transacción.
func (s *pgChatStore) CreatePairWithAction(ctx context.Context, userID uuid.UUID, userText, assistantText, kind string, payload []byte) (store.AiMessage, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.AiMessage{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if _, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "user", Content: userText,
	}); err != nil {
		return store.AiMessage{}, err
	}
	status := "proposed"
	assistant, err := qtx.CreateMessageWithAction(ctx, store.CreateMessageWithActionParams{
		UserID: userID, Role: "assistant", Content: assistantText,
		ActionKind: &kind, ActionPayload: payload, ActionStatus: &status,
	})
	if err != nil {
		return store.AiMessage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AiMessage{}, err
	}
	return assistant, nil
}

// GetMessageForAction y SetActionStatus exponen las queries con el pool simple
// (sin transacción): la transición atómica la garantiza el WHERE de la query.
func (s *pgChatStore) GetMessageForAction(ctx context.Context, id, userID uuid.UUID) (store.AiMessage, error) {
	return s.q.GetMessageForAction(ctx, store.GetMessageForActionParams{ID: id, UserID: userID})
}

func (s *pgChatStore) SetActionStatus(ctx context.Context, id, userID uuid.UUID, status string) (store.AiMessage, error) {
	return s.q.SetActionStatus(ctx, store.SetActionStatusParams{ID: id, UserID: userID, ActionStatus: &status})
}
```

(Si sqlc generó `ActionStatus` como otro tipo en los params, ajustar. NO tocar la interfaz `messageStore` todavía — eso es de la Task 4.)

- [ ] **Step 7: Verificar paquete + commit**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ ./internal/ai/ -count=1
git add api/db api/internal/store api/internal/ai/chatstore.go
git commit -m "feat(ai): migración 0009 y store de acciones propuestas en ai_messages"
```

---

### Task 2: Contexto del chat con hábitos y metas

**Files:**
- Modify: `api/internal/ai/chatcontext.go`
- Modify: `api/internal/server/server.go` (línea `NewChatContextBuilder`)
- Modify: `api/internal/ai/handler_test.go` (línea `NewChatContextBuilder` en `newEnv`)
- Test: `api/internal/ai/chatcontext_test.go`

- [ ] **Step 1: Test que falla**

En `api/internal/ai/chatcontext_test.go`, agregar fakes y ampliar el test principal:

```go
type fakeHabits struct {
	list []habits.Habit
	err  error
}

func (f fakeHabits) List(ctx context.Context, userID uuid.UUID, archived bool, today time.Time) ([]habits.Habit, error) {
	return f.list, f.err
}

type fakeGoals struct {
	list []goals.Goal
	err  error
}

func (f fakeGoals) List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error) {
	return f.list, f.err
}
```

(Imports nuevos: `github.com/focus365/api/internal/habits`, `github.com/focus365/api/internal/goals`.)

En `TestChatContextComposesJSON`: construir con los fakes nuevos —

```go
hab := fakeHabits{list: []habits.Habit{{ID: "h1", Name: "Meditar", DoneToday: false}}}
gls := fakeGoals{list: []goals.Goal{{ID: "g1", Title: "Ahorrar", Progress: 40, Status: "activa"}}}
b := newChatContextBuilder(fakeSnap{snap: snap}, fakeCycler{cycles: cyc}, lister, hab, gls)
```

y al struct `payload` sumarle:

```go
Habits []habits.Habit `json:"habits"`
Goals  []goals.Goal   `json:"goals"`
```

con asserts:

```go
if len(payload.Habits) != 1 || payload.Habits[0].ID != "h1" {
	t.Errorf("habits mal compuesto: %s", out)
}
if len(payload.Goals) != 1 || payload.Goals[0].ID != "g1" {
	t.Errorf("goals mal compuesto: %s", out)
}
```

Actualizar también las demás llamadas a `newChatContextBuilder` del archivo (test de propagación de error) agregando `fakeHabits{}, fakeGoals{}`.

- [ ] **Step 2: Verificar que falla**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -run TestChatContext -v
```
Esperado: FAIL de compilación (aridad del constructor).

- [ ] **Step 3: Implementar**

En `api/internal/ai/chatcontext.go`:

```go
// habitLister es la porción de habits.Service que usamos (hábitos activos).
type habitLister interface {
	List(ctx context.Context, userID uuid.UUID, archived bool, today time.Time) ([]habits.Habit, error)
}

// goalLister es la porción de goals.Service que usamos (metas activas).
type goalLister interface {
	List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error)
}
```

Struct y constructores pasan a 5 dependencias (`habits habitLister`, `goals goalLister`); imports nuevos `habits` y `goals`. En `build`, tras los checkins:

```go
habs, err := b.habits.List(ctx, userID, false, today)
if err != nil {
	return "", err
}
gls, err := b.goals.List(ctx, userID, "activa", today)
if err != nil {
	return "", err
}
```

y el JSON gana `"habits": habs, "goals": gls`.

Wiring:
- `api/internal/server/server.go`: `chatCtx := ai.NewChatContextBuilder(dashboardSvc, financeSvc, checkinSvc, habitsSvc, goalsSvc)`.
- `api/internal/ai/handler_test.go` (`newEnv`): `chatCtx := ai.NewChatContextBuilder(dash, fi, ci, ha, go_)`.

- [ ] **Step 4: Verificar + commit**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai/chatcontext.go api/internal/ai/chatcontext_test.go api/internal/server/server.go api/internal/ai/handler_test.go
git commit -m "feat(ai): contexto del chat con hábitos y metas (IDs para tool-use)"
```

---

### Task 3: `GroqClient.ChatStream` con tools

**Files:**
- Modify: `api/internal/ai/groq.go`
- Modify: `api/internal/ai/chat.go` (interfaz `chatStreamer` + call site)
- Modify: `api/internal/ai/chat_test.go` (fake)
- Modify: `api/internal/ai/handler_test.go` (fake)
- Test: `api/internal/ai/groq_test.go`

- [ ] **Step 1: Tests que fallan**

Agregar a `api/internal/ai/groq_test.go`:

```go
func TestGroqChatStreamToolCall(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		// El name llega en el primer fragmento; arguments llega partido.
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"function":{"name":"registrar_checkin","arguments":""}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{\"mood\":8,"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"\"energy\":6}"}}]}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	tools := []Tool{{Name: "registrar_checkin", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}
	text, tc, err := c.ChatStream(context.Background(), "sys", []ChatMsg{{Role: "user", Content: "registra"}}, tools, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if text != "" {
		t.Errorf("text = %q, want vacío en turno de tool call", text)
	}
	if tc == nil || tc.Name != "registrar_checkin" || tc.Arguments != `{"mood":8,"energy":6}` {
		t.Errorf("toolCall = %+v", tc)
	}
	body := string(gotBody)
	for _, want := range []string{`"tools":[{"type":"function"`, `"name":"registrar_checkin"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqChatStreamTextWithToolsReturnsNilToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"Hola."}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	text, tc, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "hola"}},
		[]Tool{{Name: "x", Description: "d", Parameters: json.RawMessage(`{}`)}}, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if text != "Hola." || tc != nil {
		t.Errorf("text = %q, tc = %+v", text, tc)
	}
}

func TestGroqChatStreamNoToolsOmitsField(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"ok"}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}, nil, func(string) {}); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if strings.Contains(string(gotBody), `"tools"`) {
		t.Errorf("sin tools el body no debe llevar el campo: %s", gotBody)
	}
}
```

(Import nuevo en groq_test.go: `encoding/json`.) Además, **actualizar los 3 tests existentes** de `ChatStream` (OK / CutMidway / HTTPError) a la firma nueva: `got, tc, err := c.ChatStream(ctx, sys, history, nil, onDelta)`; en el caso OK assert extra `tc == nil`.

- [ ] **Step 2: Verificar que fallan** (FAIL de compilación: aridad/retornos).

- [ ] **Step 3: Implementar en `groq.go`**

```go
// Tool es la definición de una function (estilo OpenAI) que se ofrece al modelo.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema de los argumentos
}

// ToolCall es la llamada a función que el modelo decidió hacer.
type ToolCall struct {
	Name      string
	Arguments string // JSON crudo, lo valida el servicio
}

type openaiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}
```

`chatRequest` gana `Tools []openaiTool \`json:"tools,omitempty"\``. El struct de chunk pasa a:

```go
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}
```

Firma y cuerpo nuevos de `ChatStream` (reemplaza al actual; mismas partes de
request/response, cambia la acumulación):

```go
// ChatStream envía el chat con "stream": true (y tools si hay) y re-emite cada
// delta de texto vía onDelta. Devuelve el texto acumulado y, si el modelo
// decidió llamar una función, el ToolCall reensamblado (name llega una vez,
// arguments llega fragmentado). Corte antes de [DONE] → error.
func (c *GroqClient) ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, *ToolCall, error) {
	msgs := make([]chatMessage, 0, len(history)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: system})
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}

	req := chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.7,
		MaxTokens:   400,
		Stream:      true,
	}
	for _, t := range tools {
		req.Tools = append(req.Tools, openaiTool{Type: "function", Function: openaiToolFunction{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.streamHTTP.Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return "", nil, fmt.Errorf("groq status %d: %s", res.StatusCode, string(body))
	}

	var full strings.Builder
	var tcName string
	var tcArgs strings.Builder
	sawDone := false
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			sawDone = true
			break
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return "", nil, fmt.Errorf("groq chunk inválido: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			full.WriteString(delta.Content)
			onDelta(delta.Content)
		}
		for _, tc := range delta.ToolCalls {
			if tc.Function.Name != "" {
				tcName = tc.Function.Name
			}
			tcArgs.WriteString(tc.Function.Arguments)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}
	if !sawDone {
		return "", nil, fmt.Errorf("groq stream cortado antes de [DONE]")
	}
	if tcName != "" {
		return full.String(), &ToolCall{Name: tcName, Arguments: tcArgs.String()}, nil
	}
	// Una respuesta vacía con [DONE] se trata como fallo a propósito: persistir
	// un mensaje de asistente vacío no le sirve de nada al usuario.
	if full.Len() == 0 {
		return "", nil, fmt.Errorf("groq stream sin contenido")
	}
	return full.String(), nil, nil
}
```

- [ ] **Step 4: Propagar la firma**

1. `api/internal/ai/chat.go` — interfaz:

```go
type chatStreamer interface {
	ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, *ToolCall, error)
}
```

y en `SendStream` (por ahora sin tools; la Task 4 los conecta):

```go
reply, _, err := s.streamer.ChatStream(ctx, buildChatSystemPrompt(contextJSON), history, nil, onDelta)
```

2. `api/internal/ai/chat_test.go` — `fakeChatGroq` gana campo `toolCall *ToolCall` y la firma nueva:

```go
func (f *fakeChatGroq) ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, *ToolCall, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	f.lastTools = tools
	var full string
	for _, d := range f.chatDeltas {
		full += d
		onDelta(d)
	}
	if f.err != nil {
		return "", nil, f.err
	}
	return full, f.toolCall, nil
}
```

(campos nuevos en el struct: `toolCall *ToolCall`, `lastTools []Tool`).

3. `api/internal/ai/handler_test.go` — `fakeCompleter` igual: campos `chatToolCall *ai.ToolCall`, firma `(string, *ai.ToolCall, error)` con parámetro `tools []ai.Tool`, devuelve `full, f.chatToolCall, nil` (o `"", nil, f.chatStreamErr`).

- [ ] **Step 5: Verificar todo el paquete + commit**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai/groq.go api/internal/ai/groq_test.go api/internal/ai/chat.go api/internal/ai/chat_test.go api/internal/ai/handler_test.go
git commit -m "feat(ai): ChatStream con tools y reensamblado de tool calls"
```

---

### Task 4: Definiciones de tools, payloads y `SendStream` que propone

**Files:**
- Create: `api/internal/ai/actions.go`
- Create: `api/internal/ai/actions_test.go`
- Modify: `api/internal/ai/chat.go`, `api/internal/ai/chatprompt.go`, `api/internal/ai/types.go`
- Modify: `api/internal/ai/chat_test.go` (memStore + tests)

- [ ] **Step 1: Tests que fallan**

`api/internal/ai/actions_test.go` (nuevo):

```go
package ai

import (
	"strings"
	"testing"
)

func TestParseActionPayloadValid(t *testing.T) {
	cases := []struct {
		kind, args string
	}{
		{"checkin", `{"mood":8,"energy":6,"discipline":9,"note":"bien"}`},
		{"checkin", `{"mood":1,"energy":10,"discipline":5}`},
		{"movimiento", `{"type":"expense","amount_centavos":2500000,"category":"comida"}`},
		{"movimiento", `{"type":"income","amount_centavos":1,"category":"sueldo","remark":"junio"}`},
		{"habito", `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`},
		{"meta", `{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`},
	}
	for _, c := range cases {
		if _, err := parseActionPayload(c.kind, c.args); err != nil {
			t.Errorf("%s %s: %v", c.kind, c.args, err)
		}
	}
}

func TestParseActionPayloadInvalid(t *testing.T) {
	cases := []struct {
		name, kind, args string
	}{
		{"kind desconocido", "borrar_todo", `{}`},
		{"json roto", "checkin", `{mood:`},
		{"mood fuera de rango", "checkin", `{"mood":11,"energy":6,"discipline":9}`},
		{"mood faltante", "checkin", `{"energy":6,"discipline":9}`},
		{"type inválido", "movimiento", `{"type":"transfer","amount_centavos":100,"category":"x"}`},
		{"monto cero", "movimiento", `{"type":"expense","amount_centavos":0,"category":"x"}`},
		{"categoría vacía", "movimiento", `{"type":"expense","amount_centavos":100,"category":"  "}`},
		{"uuid inválido", "habito", `{"habit_id":"no-es-uuid"}`},
		{"progress fuera de rango", "meta", `{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":101}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseActionPayload(c.kind, c.args); err == nil {
				t.Errorf("esperaba error para %s %s", c.kind, c.args)
			}
		})
	}
}

func TestActionSummaryPorKind(t *testing.T) {
	got := actionSummary("checkin", []byte(`{"mood":8,"energy":6,"discipline":9}`))
	for _, want := range []string{"check-in", "8", "6", "9"} {
		if !strings.Contains(strings.ToLower(got), want) {
			t.Errorf("summary %q no contiene %q", got, want)
		}
	}
	if s := actionSummary("habito", []byte(`{"habit_id":"x"}`)); s == "" {
		t.Error("summary de hábito vacío")
	}
}

func TestBuildChatToolsCuatro(t *testing.T) {
	tools := buildChatTools()
	if len(tools) != 4 {
		t.Fatalf("tools = %d, want 4", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
		if len(tl.Parameters) == 0 || tl.Description == "" {
			t.Errorf("tool %s incompleta", tl.Name)
		}
	}
	for _, want := range []string{"registrar_checkin", "registrar_movimiento", "marcar_habito", "actualizar_meta"} {
		if !names[want] {
			t.Errorf("falta tool %s", want)
		}
	}
}
```

En `api/internal/ai/chat_test.go`, ampliar `memStore` (cumplirá la interfaz que crece en Step 3):

```go
func (m *memStore) CreatePairWithAction(ctx context.Context, userID uuid.UUID, userText, assistantText, kind string, payload []byte) (store.AiMessage, error) {
	user := store.AiMessage{
		ID: uuid.New(), UserID: userID, Role: "user", Content: userText,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, user)
	status := "proposed"
	assistant := store.AiMessage{
		ID: uuid.New(), UserID: userID, Role: "assistant", Content: assistantText,
		ActionKind: &kind, ActionPayload: payload, ActionStatus: &status,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, assistant)
	return assistant, nil
}

func (m *memStore) GetMessageForAction(ctx context.Context, id, userID uuid.UUID) (store.AiMessage, error) {
	for _, r := range m.rows {
		if r.ID == id && r.UserID == userID {
			return r, nil
		}
	}
	return store.AiMessage{}, pgx.ErrNoRows
}

func (m *memStore) SetActionStatus(ctx context.Context, id, userID uuid.UUID, status string) (store.AiMessage, error) {
	for i, r := range m.rows {
		if r.ID == id && r.UserID == userID && r.ActionStatus != nil && *r.ActionStatus == "proposed" {
			s := status
			m.rows[i].ActionStatus = &s
			return m.rows[i], nil
		}
	}
	return store.AiMessage{}, pgx.ErrNoRows
}
```

(Import nuevo en chat_test.go: `github.com/jackc/pgx/v5`.)

Tests nuevos de servicio en `chat_test.go`:

```go
func TestChatSendStreamToolCallPersistsProposal(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, true)
	uid := uuid.New()

	msg, err := svc.SendStream(context.Background(), uid, "registra mi check-in", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if msg.Action == nil || msg.Action.Kind != "checkin" || msg.Action.Status != "proposed" {
		t.Fatalf("action = %+v", msg.Action)
	}
	if msg.ID == "" {
		t.Error("falta ID en el mensaje")
	}
	if msg.Content == "" {
		t.Error("el contenido no debe quedar vacío (resumen generado)")
	}
	if len(groq.lastTools) != 4 {
		t.Errorf("tools enviadas = %d, want 4", len(groq.lastTools))
	}
	if len(st.rows) != 2 || st.rows[1].ActionKind == nil {
		t.Errorf("persistencia = %+v", st.rows)
	}
}

func TestChatSendStreamUnknownToolDegrades(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "borrar_todo", Arguments: `{}`}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, true)

	_, err := svc.SendStream(context.Background(), uuid.New(), "x", time.Now(), func(string) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("tool desconocido no debe persistir nada")
	}
}

func TestChatHistoryIncludesAction(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, true)
	uid := uuid.New()
	if _, err := svc.SendStream(context.Background(), uid, "marca meditar", time.Now(), func(string) {}); err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	msgs, err := svc.History(context.Background(), uid)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	last := msgs[len(msgs)-1]
	if last.Action == nil || last.Action.Kind != "habito" || last.ID == "" {
		t.Errorf("history sin acción: %+v", last)
	}
}
```

- [ ] **Step 2: Verificar que fallan** (compilación: `parseActionPayload`, `Action` en `Message`, etc.).

- [ ] **Step 3: Implementar**

1. `api/internal/ai/types.go`:

```go
// ActionView es la acción propuesta/resuelta embebida en un mensaje.
type ActionView struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Status  string          `json:"status"`
}

// Message es un mensaje del chat (vista que se serializa a JSON).
type Message struct {
	ID        string      `json:"id"`
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	Action    *ActionView `json:"action,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}
```

(import `encoding/json`.)

2. `api/internal/ai/actions.go` (nuevo) — kinds, payloads, validación, resumen y tools:

```go
package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Kinds de acción persistidos en ai_messages.action_kind.
const (
	actionCheckin    = "checkin"
	actionMovimiento = "movimiento"
	actionHabito     = "habito"
	actionMeta       = "meta"
)

// toolNameToKind mapea el nombre de la function de Groq al kind persistido.
var toolNameToKind = map[string]string{
	"registrar_checkin":    actionCheckin,
	"registrar_movimiento": actionMovimiento,
	"marcar_habito":        actionHabito,
	"actualizar_meta":      actionMeta,
}

type checkinPayload struct {
	Mood       int    `json:"mood"`
	Energy     int    `json:"energy"`
	Discipline int    `json:"discipline"`
	Note       string `json:"note,omitempty"`
}

type movimientoPayload struct {
	Type           string `json:"type"`
	AmountCentavos int64  `json:"amount_centavos"`
	Category       string `json:"category"`
	Remark         string `json:"remark,omitempty"`
}

type habitoPayload struct {
	HabitID string `json:"habit_id"`
}

type metaPayload struct {
	GoalID   string `json:"goal_id"`
	Progress int    `json:"progress"`
}

func rango(v, min, max int, campo string) error {
	if v < min || v > max {
		return fmt.Errorf("%s fuera de rango (%d-%d)", campo, min, max)
	}
	return nil
}

// parseActionPayload valida los argumentos del modelo para el kind y devuelve
// el payload normalizado (re-serializado) listo para persistir. Las mismas
// reglas que validan los handlers HTTP de cada módulo.
func parseActionPayload(kind, args string) (json.RawMessage, error) {
	dec := func(v any) error {
		d := json.NewDecoder(strings.NewReader(args))
		d.DisallowUnknownFields()
		return d.Decode(v)
	}
	switch kind {
	case actionCheckin:
		var p checkinPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		for _, c := range []struct {
			v int
			n string
		}{{p.Mood, "mood"}, {p.Energy, "energy"}, {p.Discipline, "discipline"}} {
			if err := rango(c.v, 1, 10, c.n); err != nil {
				return nil, err
			}
		}
		return json.Marshal(p)
	case actionMovimiento:
		var p movimientoPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		if p.Type != "income" && p.Type != "expense" {
			return nil, fmt.Errorf("type debe ser income o expense")
		}
		if p.AmountCentavos < 1 {
			return nil, fmt.Errorf("amount_centavos debe ser positivo")
		}
		if strings.TrimSpace(p.Category) == "" {
			return nil, fmt.Errorf("falta category")
		}
		return json.Marshal(p)
	case actionHabito:
		var p habitoPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		if _, err := uuid.Parse(p.HabitID); err != nil {
			return nil, fmt.Errorf("habit_id inválido")
		}
		return json.Marshal(p)
	case actionMeta:
		var p metaPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		if _, err := uuid.Parse(p.GoalID); err != nil {
			return nil, fmt.Errorf("goal_id inválido")
		}
		if err := rango(p.Progress, 0, 100, "progress"); err != nil {
			return nil, err
		}
		return json.Marshal(p)
	}
	return nil, fmt.Errorf("acción desconocida: %s", kind)
}

// actionSummary genera el texto de la burbuja cuando el modelo no dio texto.
func actionSummary(kind string, payload json.RawMessage) string {
	switch kind {
	case actionCheckin:
		var p checkinPayload
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Propongo registrar tu check-in de hoy: ánimo %d, energía %d, disciplina %d.", p.Mood, p.Energy, p.Discipline)
	case actionMovimiento:
		var p movimientoPayload
		_ = json.Unmarshal(payload, &p)
		verbo := "un gasto"
		if p.Type == "income" {
			verbo = "un ingreso"
		}
		return fmt.Sprintf("Propongo registrar %s de $%.2f en %s.", verbo, float64(p.AmountCentavos)/100, p.Category)
	case actionHabito:
		return "Propongo marcar el hábito como hecho hoy."
	case actionMeta:
		var p metaPayload
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Propongo actualizar el progreso de la meta a %d%%.", p.Progress)
	}
	return "Propongo una acción."
}

// buildChatTools define las 4 functions que se ofrecen al modelo.
func buildChatTools() []Tool {
	return []Tool{
		{
			Name:        "registrar_checkin",
			Description: "Registra o actualiza el check-in de HOY del usuario. Úsala solo si el usuario pide explícitamente registrar su check-in y dio los tres valores (1-10).",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"mood":{"type":"integer","minimum":1,"maximum":10,"description":"ánimo 1-10"},
				"energy":{"type":"integer","minimum":1,"maximum":10,"description":"energía 1-10"},
				"discipline":{"type":"integer","minimum":1,"maximum":10,"description":"disciplina 1-10"},
				"note":{"type":"string","description":"nota opcional"}},
				"required":["mood","energy","discipline"]}`),
		},
		{
			Name:        "registrar_movimiento",
			Description: "Registra un movimiento financiero de HOY. type es income (ingreso) o expense (gasto). El monto va en CENTAVOS (ej: $250.00 → 25000). Úsala solo si el usuario dio monto y categoría.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"type":{"type":"string","enum":["income","expense"]},
				"amount_centavos":{"type":"integer","minimum":1},
				"category":{"type":"string"},
				"remark":{"type":"string"}},
				"required":["type","amount_centavos","category"]}`),
		},
		{
			Name:        "marcar_habito",
			Description: "Marca un hábito como hecho HOY. habit_id debe ser el id exacto de la lista habits del contexto; si el usuario nombra un hábito que no está, no uses la función y dilo.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"habit_id":{"type":"string","description":"UUID del hábito, de la lista habits del contexto"}},
				"required":["habit_id"]}`),
		},
		{
			Name:        "actualizar_meta",
			Description: "Actualiza el progreso (0-100) de una meta activa. goal_id debe ser el id exacto de la lista goals del contexto.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"goal_id":{"type":"string","description":"UUID de la meta, de la lista goals del contexto"},
				"progress":{"type":"integer","minimum":0,"maximum":100}},
				"required":["goal_id","progress"]}`),
		},
	}
}
```

3. `api/internal/ai/chatprompt.go` — al final del prompt (antes del bloque de contexto) sumar:

```
Puedes PROPONER acciones con las herramientas disponibles (registrar check-in, movimiento, marcar hábito, actualizar meta) SOLO cuando el usuario pida explícitamente registrar/marcar/actualizar algo y haya dado los datos. Una sola acción por turno. Usa los IDs exactos de las listas habits/goals del contexto; nunca inventes IDs ni valores que el usuario no dijo. El usuario confirmará la acción antes de ejecutarse.
```

(Integrarlo al `fmt.Sprintf` existente manteniendo el resto igual.)

4. `api/internal/ai/chat.go`:

- Interfaz `messageStore` gana:

```go
CreatePairWithAction(ctx context.Context, userID uuid.UUID, userText, assistantText, kind string, payload []byte) (store.AiMessage, error)
GetMessageForAction(ctx context.Context, id, userID uuid.UUID) (store.AiMessage, error)
SetActionStatus(ctx context.Context, id, userID uuid.UUID, status string) (store.AiMessage, error)
```

- `SendStream`: la llamada a Groq pasa a

```go
reply, toolCall, err := s.streamer.ChatStream(ctx, buildChatSystemPrompt(contextJSON), history, buildChatTools(), onDelta)
if err != nil {
	return nil, ErrUnavailable
}

if toolCall != nil {
	kind, ok := toolNameToKind[toolCall.Name]
	if !ok {
		return nil, ErrUnavailable
	}
	payload, perr := parseActionPayload(kind, toolCall.Arguments)
	if perr != nil {
		return nil, ErrUnavailable
	}
	content := strings.TrimSpace(reply)
	if content == "" {
		content = actionSummary(kind, payload)
	}
	assistant, cerr := s.store.CreatePairWithAction(ctx, userID, text, content, kind, payload)
	if cerr != nil {
		return nil, cerr
	}
	v := toMessageView(assistant)
	return &v, nil
}
```

(el camino de solo-texto sigue igual con `CreatePair`). Import nuevo: `strings`.

- Vista única para mapear filas (reemplaza el mapeo inline de `Send`, `SendStream` y `mapMessages` — DRY):

```go
// toMessageView mapea la fila a la vista, incluyendo la acción si la hay.
func toMessageView(r store.AiMessage) Message {
	m := Message{ID: r.ID.String(), Role: r.Role, Content: r.Content, CreatedAt: r.CreatedAt}
	if r.ActionKind != nil && r.ActionStatus != nil {
		m.Action = &ActionView{Kind: *r.ActionKind, Payload: json.RawMessage(r.ActionPayload), Status: *r.ActionStatus}
	}
	return m
}
```

`mapMessages` pasa a usar `toMessageView`; `Send` y `SendStream` (camino texto) también. Import `encoding/json`.

- [ ] **Step 4: Verificar paquete completo + commit**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai
git commit -m "feat(ai): SendStream propone acciones (tools + validación + persistencia proposed)"
```

---

### Task 5: Ejecutor y `ConfirmAction`/`CancelAction`

**Files:**
- Modify: `api/internal/ai/actions.go` (ejecutor), `api/internal/ai/chat.go` (servicio + errores)
- Modify: `api/internal/server/server.go`, `api/internal/ai/handler_test.go` (wiring `NewChatService` → 7 args)
- Modify: `api/internal/ai/chat_test.go` (call sites + tests)
- Test: `api/internal/ai/actions_test.go`, `api/internal/ai/chat_test.go`

- [ ] **Step 1: Tests que fallan**

En `api/internal/ai/actions_test.go`, fakes y tests del ejecutor:

```go
type fakeCheckinSvc struct{ in *checkin.Input }

func (f *fakeCheckinSvc) Upsert(ctx context.Context, userID uuid.UUID, in checkin.Input) (*checkin.CheckIn, error) {
	f.in = &in
	return &checkin.CheckIn{}, nil
}

type fakeFinanceSvc struct{ in *finance.Input }

func (f *fakeFinanceSvc) Create(ctx context.Context, userID uuid.UUID, in finance.Input) (*finance.Transaction, error) {
	f.in = &in
	return &finance.Transaction{}, nil
}

type fakeHabitsSvc struct {
	habitID  uuid.UUID
	done     bool
	notFound bool
}

func (f *fakeHabitsSvc) SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*habits.Habit, error) {
	if f.notFound {
		return nil, nil
	}
	f.habitID, f.done = habitID, done
	return &habits.Habit{}, nil
}

type fakeGoalsSvc struct {
	goalID   uuid.UUID
	progress *int32
	notFound bool
}

func (f *fakeGoalsSvc) Patch(ctx context.Context, userID, id uuid.UUID, p goals.GoalPatch, today time.Time) (*goals.Goal, error) {
	if f.notFound {
		return nil, nil
	}
	f.goalID, f.progress = id, p.Progress
	return &goals.Goal{}, nil
}

func newTestExecutor(c *fakeCheckinSvc, fin *fakeFinanceSvc, h *fakeHabitsSvc, g *fakeGoalsSvc) *actionExecutor {
	return &actionExecutor{checkin: c, finance: fin, habits: h, goals: g}
}

func TestExecutorCheckin(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{})
	today := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	err := ex.execute(context.Background(), uuid.New(), "checkin",
		[]byte(`{"mood":8,"energy":6,"discipline":9,"note":"ok"}`), today)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if c.in == nil || c.in.Mood != 8 || c.in.Note != "ok" || !c.in.Date.Equal(today) {
		t.Errorf("input = %+v", c.in)
	}
}

func TestExecutorMovimiento(t *testing.T) {
	fin := &fakeFinanceSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{})
	today := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	if err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":2500000,"category":"comida"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fin.in == nil || fin.in.Type != "expense" || fin.in.Amount != 2500000 || !fin.in.OccurredOn.Equal(today) {
		t.Errorf("input = %+v", fin.in)
	}
}

func TestExecutorHabitoNotFound(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{notFound: true}, &fakeGoalsSvc{})
	err := ex.execute(context.Background(), uuid.New(), "habito",
		[]byte(`{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}

func TestExecutorMeta(t *testing.T) {
	g := &fakeGoalsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g)
	if err := ex.execute(context.Background(), uuid.New(), "meta",
		[]byte(`{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if g.progress == nil || *g.progress != 60 {
		t.Errorf("progress = %v", g.progress)
	}
}

func TestExecutorPayloadInvalido(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{})
	err := ex.execute(context.Background(), uuid.New(), "checkin", []byte(`{"mood":99}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}
```

(Imports en actions_test.go: `context`, `errors`, `time`, `github.com/focus365/api/internal/checkin`, `finance`, `goals`, `habits`, `github.com/google/uuid`.)

En `api/internal/ai/chat_test.go`, helper + tests de confirm/cancel (los call sites existentes de `NewChatService` ganan el ejecutor como 6.º parámetro — usar `nil` en los tests que no confirman, y `newTestExecutor(...)` donde sí):

```go
func proposeCheckin(t *testing.T, svc *ChatService, uid uuid.UUID) *Message {
	t.Helper()
	msg, err := svc.SendStream(context.Background(), uid, "registra mi check-in", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	return msg
}

func TestConfirmActionExecutesAndTransitions(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}
	st := &memStore{}
	c := &fakeCheckinSvc{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)

	got, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now())
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if got.Action == nil || got.Action.Status != "done" {
		t.Errorf("status = %+v", got.Action)
	}
	if c.in == nil || c.in.Mood != 8 {
		t.Errorf("no ejecutó el check-in: %+v", c.in)
	}

	// Doble confirm → conflicto.
	if _, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now()); !errors.Is(err, ErrActionConflict) {
		t.Errorf("doble confirm err = %v, want ErrActionConflict", err)
	}
}

func TestCancelActionTransitionsWithoutExecuting(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}
	st := &memStore{}
	c := &fakeCheckinSvc{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)

	got, err := svc.CancelAction(context.Background(), uid, uuid.MustParse(msg.ID))
	if err != nil {
		t.Fatalf("CancelAction: %v", err)
	}
	if got.Action == nil || got.Action.Status != "cancelled" {
		t.Errorf("status = %+v", got.Action)
	}
	if c.in != nil {
		t.Error("cancelar no debe ejecutar nada")
	}
}

func TestConfirmActionNotFound(t *testing.T) {
	svc := NewChatService(fakeCtx{out: "{}"}, &memStore{}, &fakeChatGroq{}, &fakeChatGroq{}, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}), true)
	if _, err := svc.ConfirmAction(context.Background(), uuid.New(), uuid.New(), time.Now()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}

func TestConfirmActionOnPlainMessageIsNotFound(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"hola"}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}), true)
	uid := uuid.New()
	msg, err := svc.SendStream(context.Background(), uid, "hola", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if _, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}
```

- [ ] **Step 2: Verificar que fallan** (compilación).

- [ ] **Step 3: Implementar**

1. En `api/internal/ai/actions.go`, el ejecutor (imports nuevos: `context`, `time`, `errors`, y los 4 paquetes de dominio):

```go
// Errores del ciclo de vida de una acción. El handler los traduce a HTTP.
var (
	ErrActionNotFound = errors.New("acción no encontrada")
	ErrActionConflict = errors.New("la acción ya fue resuelta")
	ErrActionInvalid  = errors.New("acción inválida")
)

// Interfaces estrechas sobre los servicios de dominio (testeables con fakes).
type checkinUpserter interface {
	Upsert(ctx context.Context, userID uuid.UUID, in checkin.Input) (*checkin.CheckIn, error)
}

type txCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in finance.Input) (*finance.Transaction, error)
}

type habitChecker interface {
	SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*habits.Habit, error)
}

type goalPatcher interface {
	Patch(ctx context.Context, userID, id uuid.UUID, p goals.GoalPatch, today time.Time) (*goals.Goal, error)
}

// actionExecutor traduce una acción confirmada a la llamada del servicio de
// dominio correspondiente. Re-valida el payload (defensa en profundidad: ya se
// validó al proponer, pero el dato vivió en la DB entre medio).
type actionExecutor struct {
	checkin checkinUpserter
	finance txCreator
	habits  habitChecker
	goals   goalPatcher
}

// NewActionExecutor arma el ejecutor con los servicios reales (wiring en server.go).
func NewActionExecutor(c checkinUpserter, f txCreator, h habitChecker, g goalPatcher) *actionExecutor {
	return &actionExecutor{checkin: c, finance: f, habits: h, goals: g}
}

func (e *actionExecutor) execute(ctx context.Context, userID uuid.UUID, kind string, payload []byte, today time.Time) error {
	normalized, err := parseActionPayload(kind, string(payload))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrActionInvalid, err)
	}
	switch kind {
	case actionCheckin:
		var p checkinPayload
		_ = json.Unmarshal(normalized, &p)
		_, err := e.checkin.Upsert(ctx, userID, checkin.Input{
			Date: today, Mood: p.Mood, Energy: p.Energy, Discipline: p.Discipline, Note: p.Note,
		})
		return err
	case actionMovimiento:
		var p movimientoPayload
		_ = json.Unmarshal(normalized, &p)
		_, err := e.finance.Create(ctx, userID, finance.Input{
			Type: p.Type, Amount: p.AmountCentavos, OccurredOn: today, Category: p.Category, Remark: p.Remark,
		})
		return err
	case actionHabito:
		var p habitoPayload
		_ = json.Unmarshal(normalized, &p)
		h, err := e.habits.SetCheck(ctx, userID, uuid.MustParse(p.HabitID), today, true, today)
		if err != nil {
			return err
		}
		if h == nil {
			return fmt.Errorf("%w: hábito no encontrado", ErrActionInvalid)
		}
		return nil
	case actionMeta:
		var p metaPayload
		_ = json.Unmarshal(normalized, &p)
		prog := int32(p.Progress)
		g, err := e.goals.Patch(ctx, userID, uuid.MustParse(p.GoalID), goals.GoalPatch{Progress: &prog}, today)
		if err != nil {
			return err
		}
		if g == nil {
			return fmt.Errorf("%w: meta no encontrada", ErrActionInvalid)
		}
		return nil
	}
	return fmt.Errorf("%w: kind %s", ErrActionInvalid, kind)
}
```

2. En `api/internal/ai/chat.go`: `ChatService` gana campo `exec *actionExecutor`; `NewChatService(ctxb, q, c, s, exec *actionExecutor, hasKey)` (6 parámetros + hasKey = 7 args en total contando hasKey). Métodos:

```go
// ConfirmAction ejecuta la acción propuesta del mensaje y la marca done.
// Solo transiciona si la ejecución fue exitosa.
func (s *ChatService) ConfirmAction(ctx context.Context, userID, messageID uuid.UUID, today time.Time) (*Message, error) {
	row, err := s.store.GetMessageForAction(ctx, messageID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.ActionKind == nil || row.ActionStatus == nil {
		return nil, ErrActionNotFound
	}
	if *row.ActionStatus != "proposed" {
		return nil, ErrActionConflict
	}
	if err := s.exec.execute(ctx, userID, *row.ActionKind, row.ActionPayload, today); err != nil {
		return nil, err
	}
	upd, err := s.store.SetActionStatus(ctx, messageID, userID, "done")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toMessageView(upd)
	return &v, nil
}

// CancelAction marca la propuesta como cancelada sin ejecutar nada.
func (s *ChatService) CancelAction(ctx context.Context, userID, messageID uuid.UUID) (*Message, error) {
	row, err := s.store.GetMessageForAction(ctx, messageID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.ActionKind == nil || row.ActionStatus == nil {
		return nil, ErrActionNotFound
	}
	if *row.ActionStatus != "proposed" {
		return nil, ErrActionConflict
	}
	upd, err := s.store.SetActionStatus(ctx, messageID, userID, "cancelled")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toMessageView(upd)
	return &v, nil
}
```

(Import nuevo en chat.go: `github.com/jackc/pgx/v5`.)

3. Call sites de `NewChatService`:
- `api/internal/server/server.go`:

```go
actionExec := ai.NewActionExecutor(checkinSvc, financeSvc, habitsSvc, goalsSvc)
chatSvc := ai.NewChatService(chatCtx, chatStore, groq, groq, actionExec, d.GroqAPIKey != "")
```

(usar los nombres reales de las variables de servicio del archivo).
- `api/internal/ai/handler_test.go` (`newEnv`):

```go
actionExec := ai.NewActionExecutor(ci, fi, ha, go_)
chatSvc := ai.NewChatService(chatCtx, chatStore, comp, comp, actionExec, hasKey)
```

- `api/internal/ai/chat_test.go`: los tests previos que no confirman pasan `nil` como ejecutor.

- [ ] **Step 4: Verificar + commit**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai api/internal/server/server.go
git commit -m "feat(ai): ConfirmAction/CancelAction con ejecutor sobre los servicios de dominio"
```

---

### Task 6: Endpoints `POST /ai/actions/{id}/confirm|cancel`

**Files:**
- Modify: `api/internal/ai/handler.go`
- Test: `api/internal/ai/chat_handler_test.go`

- [ ] **Step 1: Tests que fallan**

Agregar a `api/internal/ai/chat_handler_test.go`:

```go
func postAction(t *testing.T, h http.Handler, tok, id, verb string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ai/actions/"+id+"/"+verb+"?today="+today, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// proposeViaChat dispara el chat/stream con un fake que devuelve un tool call
// y extrae el id del mensaje propuesto desde el historial.
func proposeViaChat(t *testing.T, e *env, tok string) string {
	t.Helper()
	rec := postChatStream(t, e.h, tok, `{"message":"registra mi check-in: 8 6 9"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat/stream code = %d, body = %s", rec.Code, rec.Body.String())
	}
	_, body := getMessages(t, e.h, tok)
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("sin mensajes tras proponer")
	}
	last, _ := msgs[len(msgs)-1].(map[string]any)
	action, _ := last["action"].(map[string]any)
	if action == nil || action["status"] != "proposed" {
		t.Fatalf("último mensaje sin acción proposed: %v", last)
	}
	id, _ := last["id"].(string)
	if id == "" {
		t.Fatal("mensaje sin id")
	}
	return id
}

func checkinToolCall() *ai.ToolCall {
	return &ai.ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}
}

func TestActionConfirmHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "action-ok@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm code = %d, body = %s", rec.Code, rec.Body.String())
	}
	msg, _ := body["message"].(map[string]any)
	action, _ := msg["action"].(map[string]any)
	if action["status"] != "done" {
		t.Errorf("status = %v", action)
	}

	// El check-in quedó escrito de verdad.
	ci, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{
		UserID: uid, Date: dayTime(t),
	})
	if err != nil {
		t.Fatalf("check-in no escrito: %v", err)
	}
	if ci.Mood != 8 || ci.Energy != 6 || ci.Discipline != 9 {
		t.Errorf("check-in = %+v", ci)
	}

	// Doble confirm → 409.
	if rec2, _ := postAction(t, e.h, tok, id, "confirm"); rec2.Code != http.StatusConflict {
		t.Errorf("doble confirm code = %d, want 409", rec2.Code)
	}
}

func TestActionCancel(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "action-cancel@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel code = %d", rec.Code)
	}
	msg, _ := body["message"].(map[string]any)
	action, _ := msg["action"].(map[string]any)
	if action["status"] != "cancelled" {
		t.Errorf("status = %v", action)
	}
	// Nada se escribió.
	if _, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{
		UserID: uid, Date: dayTime(t),
	}); err == nil {
		t.Error("cancelar no debe escribir el check-in")
	}
}

func TestActionErrors(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: checkinToolCall()}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "action-err@b.com")

	// id inexistente → 404; id malformado → 404; sin token → 401.
	if rec, _ := postAction(t, e.h, tok, uuid.New().String(), "confirm"); rec.Code != http.StatusNotFound {
		t.Errorf("inexistente code = %d, want 404", rec.Code)
	}
	if rec, _ := postAction(t, e.h, tok, "no-uuid", "confirm"); rec.Code != http.StatusNotFound {
		t.Errorf("uuid malformado code = %d, want 404", rec.Code)
	}
	if rec, _ := postAction(t, e.h, "", uuid.New().String(), "confirm"); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}

	// El mensaje de otro usuario no se puede confirmar.
	idA := proposeViaChat(t, e, tok)
	_, tokB := e.user(t, "action-eve@b.com")
	if rec, _ := postAction(t, e.h, tokB, idA, "confirm"); rec.Code != http.StatusNotFound {
		t.Errorf("cross-user code = %d, want 404", rec.Code)
	}
}
```

(Imports a verificar en el archivo: `context`, `github.com/focus365/api/internal/ai`, `github.com/focus365/api/internal/store`, `github.com/google/uuid`. Si el nombre real de la query de lectura difiere de `GetCheckInByDate`, usar el de `api/db/queries/check_ins.sql`.)

- [ ] **Step 2: Verificar que fallan** (404: rutas inexistentes).

- [ ] **Step 3: Implementar en `handler.go`**

En `Routes`:

```go
r.Post("/actions/{id}/confirm", handleActionConfirm(chat))
r.Post("/actions/{id}/cancel", handleActionCancel(chat))
```

Handlers + helper:

```go
// actionMessageResponse envuelve el mensaje actualizado tras confirm/cancel.
type actionMessageResponse struct {
	Message Message `json:"message"`
}

// resolveAction maneja lo común de confirm/cancel: auth, parseo del id y la
// traducción de errores del servicio a HTTP.
func resolveAction(w http.ResponseWriter, r *http.Request,
	do func(ctx context.Context, userID, id uuid.UUID) (*Message, error)) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteErr(w, http.StatusNotFound, "acción no encontrada")
		return
	}
	msg, err := do(r.Context(), userID, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrActionNotFound):
			httpx.WriteErr(w, http.StatusNotFound, "acción no encontrada")
		case errors.Is(err, ErrActionConflict):
			httpx.WriteErr(w, http.StatusConflict, "la acción ya fue resuelta")
		case errors.Is(err, ErrActionInvalid):
			httpx.WriteErr(w, http.StatusBadRequest, err.Error())
		default:
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, actionMessageResponse{Message: *msg})
}

func handleActionConfirm(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolveAction(w, r, func(ctx context.Context, userID, id uuid.UUID) (*Message, error) {
			return chat.ConfirmAction(ctx, userID, id, parseTodayParam(r))
		})
	}
}

func handleActionCancel(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolveAction(w, r, func(ctx context.Context, userID, id uuid.UUID) (*Message, error) {
			return chat.CancelAction(ctx, userID, id)
		})
	}
}
```

(Import nuevo en handler.go: `context`.)

- [ ] **Step 4: Verificar todo el backend + commit**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/internal/ai/handler.go api/internal/ai/chat_handler_test.go
git commit -m "feat(ai): endpoints POST /ai/actions/{id}/confirm y /cancel"
```

---

### Task 7: Lib frontend (`Message.action`, `confirmAction`, `cancelAction`)

**Files:**
- Modify: `web/src/lib/ai.ts`
- Test: `web/src/lib/ai.test.ts`

- [ ] **Step 1: Tests que fallan**

En `web/src/lib/ai.test.ts` (sumar `confirmAction, cancelAction, type Action` al import de `./ai`):

```ts
describe("confirmAction / cancelAction", () => {
  afterEach(() => vi.restoreAllMocks());

  const done: Message = {
    id: "m1",
    role: "assistant",
    content: "Propongo registrar tu check-in.",
    action: { kind: "checkin", payload: { mood: 8, energy: 6, discipline: 9 }, status: "done" },
    created_at: "2026-06-11T10:00:02Z",
  };

  it("confirmAction hace POST al endpoint y devuelve el mensaje", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ message: done }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await confirmAction("m1");
    expect(got).toEqual(done);
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/ai/actions/m1/confirm");
    expect(opts?.method).toBe("POST");
  });

  it("cancelAction hace POST al endpoint de cancelar", async () => {
    const cancelled = { ...done, action: { ...done.action!, status: "cancelled" } };
    const fetchMock = vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify({ message: cancelled }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);

    const got = await cancelAction("m1");
    expect(got.action?.status).toBe("cancelled");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/ai/actions/m1/cancel");
  });

  it("confirmAction propaga el error del backend (409)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "la acción ya fue resuelta" }), { status: 409 })
        )
      )
    );
    await expect(confirmAction("m1")).rejects.toThrowError("la acción ya fue resuelta");
  });
});
```

- [ ] **Step 2: Verificar que fallan** (`cd web && npx vitest run src/lib/ai.test.ts` — import error).

- [ ] **Step 3: Implementar en `web/src/lib/ai.ts`**

```ts
export type Action = {
  kind: string;
  payload: Record<string, unknown>;
  status: "proposed" | "done" | "cancelled";
};

export type Message = {
  id: string;
  role: string;
  content: string;
  action?: Action;
  created_at: string;
};

export function confirmAction(id: string): Promise<Message> {
  return apiFetch<{ message: Message }>(`/api/v1/ai/actions/${id}/confirm`, {
    method: "POST",
  }).then((r) => r.message);
}

export function cancelAction(id: string): Promise<Message> {
  return apiFetch<{ message: Message }>(`/api/v1/ai/actions/${id}/cancel`, {
    method: "POST",
  }).then((r) => r.message);
}
```

(El type `Message` existente se reemplaza por este — `id` y `action` nuevos. Los tests previos del archivo que construyen `Message` literales ganan `id: "..."`.)

- [ ] **Step 4: Verificar + commit**

```bash
cd web && npx vitest run src/lib/ai.test.ts && npm run build
git add web/src/lib/ai.ts web/src/lib/ai.test.ts
git commit -m "feat(web): tipos de acción y confirmAction/cancelAction en la lib del chat"
```

Nota: si otros tests del repo construyen `Message` sin `id` (p. ej. `asistente.test.tsx`), el build estricto fallará — agregar `id` a esos literales es parte de esta task (sin tocar la lógica de la página, que es la Task 8).

---

### Task 8: Tarjeta de acción en `/asistente`

**Files:**
- Modify: `web/src/routes/asistente.tsx`
- Test: `web/src/routes/asistente.test.tsx`

- [ ] **Step 1: Tests que fallan**

Agregar a `web/src/routes/asistente.test.tsx` (los mensajes de los mocks existentes ganan `id`):

```ts
function proposedMessages() {
  return {
    messages: [
      { id: "u1", role: "user", content: "registra mi check-in", created_at: "2026-06-11T10:00:00Z" },
      {
        id: "m1",
        role: "assistant",
        content: "Propongo registrar tu check-in de hoy: ánimo 8, energía 6, disciplina 9.",
        action: { kind: "checkin", payload: { mood: 8, energy: 6, discipline: 9 }, status: "proposed" },
        created_at: "2026-06-11T10:00:01Z",
      },
    ],
  };
}

it("una acción proposed muestra botones y confirmar la pasa a hecha", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/actions/m1/confirm" && opts?.method === "POST") {
      const done = proposedMessages().messages[1];
      done.action!.status = "done";
      return Promise.resolve(new Response(JSON.stringify({ message: done }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify(proposedMessages()), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  expect(await screen.findByRole("button", { name: "Confirmar" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "Confirmar" }));

  expect(await screen.findByText(/Hecha/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Confirmar" })).toBeNull();
});

it("cancelar deja la tarjeta como cancelada sin ejecutar", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/actions/m1/cancel" && opts?.method === "POST") {
      const cancelled = proposedMessages().messages[1];
      cancelled.action!.status = "cancelled";
      return Promise.resolve(new Response(JSON.stringify({ message: cancelled }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify(proposedMessages()), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: "Cancelar" }));

  expect(await screen.findByText(/Cancelada/)).toBeInTheDocument();
  const confirmCalls = fetchMock.mock.calls.filter(([u]) => String(u).includes("/confirm"));
  expect(confirmCalls).toHaveLength(0);
});

it("mensajes sin acción no muestran tarjeta ni botones", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            messages: [
              { id: "m2", role: "assistant", content: "Vas bien.", created_at: "2026-06-11T10:00:01Z" },
            ],
          }),
          { status: 200 }
        )
      )
    )
  );
  renderPage();
  expect(await screen.findByText("Vas bien.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Confirmar" })).toBeNull();
});
```

- [ ] **Step 2: Verificar que fallan** (`npx vitest run src/routes/asistente.test.tsx`).

- [ ] **Step 3: Implementar en `asistente.tsx`**

1. Imports: sumar `confirmAction, cancelAction` al import de `@/lib/ai`.
2. Mutaciones de acción (junto a la mutación del chat):

```ts
const actionMutation = useMutation({
  mutationFn: ({ id, verb }: { id: string; verb: "confirm" | "cancel" }) =>
    verb === "confirm" ? confirmAction(id) : cancelAction(id),
  onSuccess: (updated) => {
    setError(null);
    qc.setQueryData<Message[]>(["ai-messages"], (prev) =>
      (prev ?? []).map((m) => (m.id === updated.id ? updated : m))
    );
  },
  onError: (err) =>
    setError(err instanceof Error ? err.message : "No se pudo resolver la acción"),
});
```

3. Componente de tarjeta dentro del archivo (antes de `AsistentePage`):

```tsx
const ACTION_TITLES: Record<string, string> = {
  checkin: "Check-in de hoy",
  movimiento: "Movimiento",
  habito: "Hábito",
  meta: "Meta",
};

function actionDetails(action: NonNullable<Message["action"]>): string {
  const p = action.payload as Record<string, unknown>;
  switch (action.kind) {
    case "checkin":
      return `Ánimo ${p.mood} · Energía ${p.energy} · Disciplina ${p.discipline}`;
    case "movimiento":
      return `${p.type === "income" ? "Ingreso" : "Gasto"} de $${(Number(p.amount_centavos) / 100).toFixed(2)} en ${p.category}`;
    case "habito":
      return "Marcar como hecho hoy";
    case "meta":
      return `Progreso al ${p.progress}%`;
    default:
      return "";
  }
}

function ActionCard({
  message,
  pending,
  onResolve,
}: {
  message: Message;
  pending: boolean;
  onResolve: (id: string, verb: "confirm" | "cancel") => void;
}) {
  const action = message.action!;
  return (
    <div className="mt-2 rounded-lg border border-amber-brand/40 bg-ink-800 p-3 text-sm">
      <p className="font-bold">{ACTION_TITLES[action.kind] ?? "Acción"}</p>
      <p className="text-sand-400">{actionDetails(action)}</p>
      {action.status === "proposed" && (
        <div className="mt-2 flex gap-2">
          <button
            onClick={() => onResolve(message.id, "confirm")}
            disabled={pending}
            className="rounded-lg bg-amber-brand px-3 py-1 text-xs font-bold text-ink-950 disabled:opacity-60"
          >
            Confirmar
          </button>
          <button
            onClick={() => onResolve(message.id, "cancel")}
            disabled={pending}
            className="rounded-lg border border-ink-700 px-3 py-1 text-xs disabled:opacity-60"
          >
            Cancelar
          </button>
        </div>
      )}
      {action.status === "done" && <p className="mt-2 text-xs font-bold text-amber-brand">✓ Hecha</p>}
      {action.status === "cancelled" && <p className="mt-2 text-xs text-sand-400">Cancelada</p>}
    </div>
  );
}
```

4. En el render de mensajes, dentro de la burbuja del asistente (el `div` existente del map), debajo del `{m.content}`:

```tsx
{m.role === "assistant" && m.action && (
  <ActionCard
    message={m}
    pending={actionMutation.isPending}
    onResolve={(id, verb) => actionMutation.mutate({ id, verb })}
  />
)}
```

(La key del map puede seguir siendo el índice; los mensajes ahora traen `id` pero no es necesario cambiarla para esta task.)

- [ ] **Step 4: Verificar suite completa + build + commit**

```bash
cd web && npx vitest run && npm run build
git add web/src/routes/asistente.tsx web/src/routes/asistente.test.tsx
git commit -m "feat(web): tarjeta de acción con Confirmar/Cancelar en /asistente"
```

---

### Task 9: E2E docker, verificación final y cierre

**Files:**
- Create: `/tmp/smoke_actions.sh` (efímero)

- [ ] **Step 1: Rebuild docker**

```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"; cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
```
(El api aplica la migración 0009 al arrancar.)

- [ ] **Step 2: Smoke E2E con la clave Groq real**

`/tmp/smoke_actions.sh`:

```bash
#!/bin/zsh
set -u
BASE=http://localhost:5174/api/v1
PASS=0; FAIL=0
check() { if [ "$1" = "$2" ]; then echo "OK   $3"; PASS=$((PASS+1)); else echo "FAIL $3 (got $1, want $2)"; FAIL=$((FAIL+1)); fi }

EMAIL="smoke-actions-$RANDOM@focus.com"
TOKEN=$(curl -s -X POST $BASE/auth/register -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"p4ssword\",\"name\":\"Smoke\"}" \
  | python3 -c "import json,sys;print(json.load(sys.stdin)['access_token'])")
[ -n "$TOKEN" ] && check yes yes "registro y token" || check no yes "registro y token"

# 1. Pedir una acción por chat (stream); el done debe traer action proposed.
SSE=$(curl -s -N -X POST $BASE/ai/chat/stream \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"message":"Registra mi check-in de hoy: ánimo 8, energía 6, disciplina 9"}')
echo "$SSE" | grep -q '"status":"proposed"' && check yes yes "done con acción proposed" || check no yes "done con acción proposed"

# 2. Extraer el id del mensaje propuesto del historial.
MSGID=$(curl -s $BASE/ai/messages -H "Authorization: Bearer $TOKEN" \
  | python3 -c "
import json,sys
msgs=json.load(sys.stdin)['messages']
acts=[m for m in msgs if m.get('action')]
print(acts[-1]['id'] if acts else '')")
[ -n "$MSGID" ] && check yes yes "propuesta en el historial" || check no yes "propuesta en el historial"

# 3. Confirmar → done.
STATUS=$(curl -s -X POST $BASE/ai/actions/$MSGID/confirm -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import json,sys;print(json.load(sys.stdin)['message']['action']['status'])" 2>/dev/null)
check "$STATUS" "done" "confirm deja la acción done"

# 4. El check-in quedó escrito.
TODAY=$(date -u +%F)
MOOD=$(curl -s "$BASE/check-ins/today?date=$TODAY" -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('mood',''))" 2>/dev/null)
check "$MOOD" "8" "check-in escrito con mood 8"

# 5. Doble confirm → 409.
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE/ai/actions/$MSGID/confirm -H "Authorization: Bearer $TOKEN")
check "$CODE" "409" "doble confirm 409"

# 6. Sin token → 401.
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE/ai/actions/$MSGID/confirm)
check "$CODE" "401" "sin token 401"

# 7. Una pregunta normal sigue streameando texto.
SSE2=$(curl -s -N -X POST $BASE/ai/chat/stream \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"message":"¿cómo voy hoy? en una frase"}')
echo "$SSE2" | grep -q "event: delta" && check yes yes "chat normal sigue streameando" || check no yes "chat normal sigue streameando"

echo "---"; echo "$PASS OK / $FAIL FAIL"
exit $FAIL
```

Antes de correrlo, verificar la ruta real del check-in del día (`grep -rn "today" api/internal/checkin/handler.go`) y ajustar el paso 4 si difiere. Correr: `chmod +x /tmp/smoke_actions.sh && /tmp/smoke_actions.sh`. Esperado: **8 OK / 0 FAIL**. (El paso 1 depende del modelo: si Groq no propone la acción al primer intento, re-correr el script con otro usuario antes de declarar fallo.)

- [ ] **Step 3: Verificación final completa**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
cd ../web && npx vitest run && npm run build
```
Esperado: todo verde.

- [ ] **Step 4: Cierre**

Review final holística (subagente), aplicar nits si los hay, y usar
superpowers:finishing-a-development-branch: merge `--no-ff` a `master`,
borrar la rama, bitácora de sesión en `docs/superpowers/sesiones/`.

---

## Notas para el ejecutor

- **Rama:** crear `plan-11-acciones-ia` desde `master` antes de la Task 1 (rama de feature, NO worktree: docker está atado a la ruta del repo).
- Los tipos generados por sqlc mandan: si un campo nullable sale con otro tipo que el asumido aquí (`*string`/`[]byte`), ajustar el código a lo generado, no al plan.
- El contrato del chat existente no cambia: si un test previo de `/ai/chat` o del streaming falla, es regresión introducida.
- `httptest.ResponseRecorder` implementa `http.Flusher` (los tests SSE no necesitan servidor real).
- Frontend: tras la Task 7 el tipo `Message` exige `id` — el build estricto delata los literales sin actualizar.
