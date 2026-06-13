# Plan 15 — Multi-acción por turno + deshacer — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un turno del chat puede proponer hasta 5 acciones (tarjetas independientes) y toda acción ejecutada puede deshacerse una vez con reversa por kind.

**Architecture:** Migración estructural 1:1 → 1:N: tabla `ai_actions` (con `result` JSONB para el undo) reemplaza las columnas de `ai_messages`, migrando los datos. `ChatStream` devuelve `[]ToolCall` (reensamblado por index), `SendStream` propone all-or-nothing (tope 5), el ejecutor devuelve `result` al ejecutar y gana `undo` por kind, y el frontend renderiza N tarjetas con botón Deshacer.

**Tech Stack:** Go + sqlc/pgx + goose (api), React + TanStack Query (web), Groq tool-use.

**Spec:** `docs/superpowers/specs/2026-06-12-plan-15-multiaccion-undo-design.md`

**Entorno:** Go desde `api/` con `GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`; `sqlc generate` desde `api/`. Frontend `cd web && npx vitest run && npm run build`. Commits en español con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Rama: `plan-15-multiaccion-undo` desde `main`.

**Regla de verificación por tarea:** las tareas de backend verifican el backend completo (`go build ./... && go test -p 1 ./... -count=1`); el frontend queda intencionalmente roto entre la Task 2 y la Task 7 (el contrato pasa de `action` a `actions[]`) — la suite web solo se exige verde a partir de la Task 7.

---

### Task 1: `checkin.Delete`

**Files:**
- Modify: `api/db/queries/check_ins.sql`, `api/internal/checkin/service.go`
- Test: `api/internal/checkin/service_test.go` (seguir el patrón de tests existente del paquete)
- Generated: `api/internal/store/` (sqlc)

- [ ] **Step 1: Query.** Agregar a `api/db/queries/check_ins.sql`:

```sql
-- name: DeleteCheckIn :execrows
DELETE FROM check_ins
WHERE user_id = $1 AND date = $2;
```

Correr `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`.

- [ ] **Step 2: Test que falla** (leer primero `service_test.go` para reusar su setup — usa `testutil.NewDB` y un helper de usuario; replicar el patrón):

```go
func TestDeleteCheckIn(t *testing.T) {
	// setup igual al de TestUpsert del archivo: pool, queries, servicio, usuario.
	// 1. Upsert de un check-in del día.
	// 2. svc.Delete(ctx, userID, date) → deleted == true.
	// 3. svc.Today(ctx, userID, date) → (nil, nil).
	// 4. Delete de nuevo → deleted == false (idempotente).
	// 5. Delete con OTRO usuario sobre el mismo día → false (scoping).
}
```

(Escribir el test completo siguiendo el setup real del archivo; los 5 pasos de arriba son las aserciones obligatorias.)

- [ ] **Step 3: Implementar** en `api/internal/checkin/service.go`:

```go
// Delete borra el check-in del día. Devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, date time.Time) (bool, error) {
	n, err := s.q.DeleteCheckIn(ctx, store.DeleteCheckInParams{UserID: userID, Date: date})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
```

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/checkin/ ./internal/store/ -count=1
git add api/db/queries/check_ins.sql api/internal/store api/internal/checkin
git commit -m "feat(checkin): Delete por usuario y fecha (pieza para el undo de la IA)"
```

---

### Task 2: La mudanza — tabla `ai_actions`, store y servicio con `actions[]`

La tarea grande: migración 0011 + queries nuevas + store + interfaz + vista
`Message.Actions []ActionView` + Confirm/Cancel sobre action id. Al cierre el
comportamiento es equivalente al actual (un turno sigue proponiendo UNA acción,
como lista de 1) y TODO el backend está verde.

**Files:**
- Create: `api/db/migrations/0011_ai_actions_table.sql`, `api/db/queries/ai_actions.sql`
- Modify: `api/db/queries/ai_messages.sql` (eliminar las 3 queries de acciones), `api/internal/ai/chatstore.go`, `api/internal/ai/chat.go`, `api/internal/ai/types.go`, `api/internal/ai/chat_test.go` (memStore + tests), `api/internal/ai/chat_handler_test.go` (helpers a actions[]), `api/internal/store/ai_messages_test.go` (tests viejos de columnas → adaptados a la tabla)
- Generated: `api/internal/store/`

- [ ] **Step 1: Migración** `api/db/migrations/0011_ai_actions_table.sql`:

```sql
-- +goose Up
CREATE TABLE ai_actions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position   INT  NOT NULL DEFAULT 0,
    kind       TEXT NOT NULL CHECK (kind IN (
        'checkin','movimiento','habito','meta',
        'habito_nuevo','meta_nueva','entrenamiento')),
    payload    JSONB NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('proposed','done','cancelled','undone')),
    result     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_actions_message ON ai_actions (message_id, position);
CREATE INDEX idx_ai_actions_user ON ai_actions (user_id);

INSERT INTO ai_actions (message_id, user_id, position, kind, payload, status, created_at)
SELECT id, user_id, 0, action_kind, action_payload, action_status, created_at
FROM ai_messages
WHERE action_kind IS NOT NULL;

ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_consistente,
    DROP CONSTRAINT ai_messages_action_status_valid,
    DROP CONSTRAINT ai_messages_action_kind_valid,
    DROP COLUMN action_status,
    DROP COLUMN action_payload,
    DROP COLUMN action_kind;

-- +goose Down
ALTER TABLE ai_messages
    ADD COLUMN action_kind    TEXT,
    ADD COLUMN action_payload JSONB,
    ADD COLUMN action_status  TEXT,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN (
            'checkin','movimiento','habito','meta',
            'habito_nuevo','meta_nueva','entrenamiento')
    ),
    ADD CONSTRAINT ai_messages_action_status_valid CHECK (
        action_status IS NULL OR action_status IN ('proposed','done','cancelled')
    ),
    ADD CONSTRAINT ai_messages_action_consistente CHECK (
        (action_kind IS NULL AND action_payload IS NULL AND action_status IS NULL)
        OR (action_kind IS NOT NULL AND action_payload IS NOT NULL AND action_status IS NOT NULL)
    );

UPDATE ai_messages m SET
    action_kind = a.kind,
    action_payload = a.payload,
    -- undone no existe en el modelo viejo: degrada a done
    action_status = CASE WHEN a.status = 'undone' THEN 'done' ELSE a.status END
FROM ai_actions a
WHERE a.message_id = m.id AND a.position = 0;

DROP TABLE ai_actions;
```

- [ ] **Step 2: Queries.** Crear `api/db/queries/ai_actions.sql`:

```sql
-- name: CreateAction :one
INSERT INTO ai_actions (message_id, user_id, position, kind, payload, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListActionsByMessages :many
SELECT * FROM ai_actions
WHERE message_id = ANY($1::uuid[])
ORDER BY message_id, position;

-- name: GetAction :one
SELECT * FROM ai_actions
WHERE id = $1 AND user_id = $2;

-- name: SetActionStatusFrom :one
UPDATE ai_actions
SET status = $3, result = COALESCE($4, result)
WHERE id = $1 AND user_id = $2 AND status = $5
RETURNING *;
```

Eliminar de `api/db/queries/ai_messages.sql` las queries `CreateMessageWithAction`, `GetMessageForAction` y `SetActionStatus`. Correr `sqlc generate` (los tests de store que las usaban se adaptan en el Step 5).

- [ ] **Step 3: Store del chat** (`chatstore.go`). Eliminar `CreatePairWithAction`, `GetMessageForAction` y `SetActionStatus`; agregar:

```go
// ProposedAction es una acción validada lista para persistir como proposed.
type ProposedAction struct {
	Kind    string
	Payload json.RawMessage
}

// CreatePairWithActions persiste el par usuario+asistente y las N acciones
// propuestas del turno en una sola transacción.
func (s *pgChatStore) CreatePairWithActions(ctx context.Context, userID uuid.UUID, userText, assistantText string, actions []ProposedAction) (store.AiMessage, []store.AiAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.AiMessage{}, nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if _, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "user", Content: userText,
	}); err != nil {
		return store.AiMessage{}, nil, err
	}
	assistant, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "assistant", Content: assistantText,
	})
	if err != nil {
		return store.AiMessage{}, nil, err
	}
	rows := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row, err := qtx.CreateAction(ctx, store.CreateActionParams{
			MessageID: assistant.ID, UserID: userID, Position: int32(i),
			Kind: a.Kind, Payload: a.Payload, Status: "proposed",
		})
		if err != nil {
			return store.AiMessage{}, nil, err
		}
		rows = append(rows, row)
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AiMessage{}, nil, err
	}
	return assistant, rows, nil
}

func (s *pgChatStore) ListActionsByMessages(ctx context.Context, messageIDs []uuid.UUID) ([]store.AiAction, error) {
	return s.q.ListActionsByMessages(ctx, messageIDs)
}

func (s *pgChatStore) GetAction(ctx context.Context, id, userID uuid.UUID) (store.AiAction, error) {
	return s.q.GetAction(ctx, store.GetActionParams{ID: id, UserID: userID})
}

// SetActionStatusFrom transiciona el estado solo si está en from (atómico) y
// guarda result si no es nil.
func (s *pgChatStore) SetActionStatusFrom(ctx context.Context, id, userID uuid.UUID, to string, result []byte, from string) (store.AiAction, error) {
	return s.q.SetActionStatusFrom(ctx, store.SetActionStatusFromParams{
		ID: id, UserID: userID, Status: to, Result: result, Status_2: from,
	})
}
```

(Los nombres de los params generados por sqlc mandan — `Status_2` puede salir
con otro nombre; ajustar a lo generado. Import nuevo: `encoding/json`.)

- [ ] **Step 4: Servicio y vista.**

`types.go` — la vista pasa a plural y la acción gana ID:

```go
// ActionView es una acción del mensaje (propuesta o resuelta).
type ActionView struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Status  string          `json:"status"`
}

type Message struct {
	ID        string       `json:"id"`
	Role      string       `json:"role"`
	Content   string       `json:"content"`
	Actions   []ActionView `json:"actions,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}
```

`chat.go`:
- La interfaz `messageStore` reemplaza los 3 métodos viejos por los 4 nuevos
  (firmas exactas del Step 3).
- `toMessageView(r store.AiMessage)` pierde la lógica de Action (queda
  id/role/content/created_at). Helper nuevo:

```go
func toActionView(a store.AiAction) ActionView {
	return ActionView{ID: a.ID.String(), Kind: a.Kind, Payload: json.RawMessage(a.Payload), Status: a.Status}
}
```

- `History` arma los mensajes y cuelga las acciones (un solo query):

```go
func (s *ChatService) History(ctx context.Context, userID uuid.UUID) ([]Message, error) {
	rows, err := s.store.ListMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	msgs := mapMessages(rows)
	if len(rows) == 0 {
		return msgs, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	acts, err := s.store.ListActionsByMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	byMsg := make(map[string][]ActionView)
	for _, a := range acts {
		k := a.MessageID.String()
		byMsg[k] = append(byMsg[k], toActionView(a))
	}
	for i := range msgs {
		msgs[i].Actions = byMsg[msgs[i].ID]
	}
	return msgs, nil
}
```

- `SendStream`, rama de tool call: sigue tomando UNA tool call (la
  multi-acción llega en la Task 4), pero persiste vía la API nueva:

```go
assistant, actions, cerr := s.store.CreatePairWithActions(ctx, userID, text, content,
	[]ProposedAction{{Kind: kind, Payload: payload}})
if cerr != nil {
	return nil, cerr
}
v := toMessageView(assistant)
for _, a := range actions {
	v.Actions = append(v.Actions, toActionView(a))
}
return &v, nil
```

- `ConfirmAction`/`CancelAction` operan sobre el **action id**:

```go
func (s *ChatService) ConfirmAction(ctx context.Context, userID, actionID uuid.UUID, today time.Time) (*ActionView, error) {
	row, err := s.store.GetAction(ctx, actionID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.Status != "proposed" {
		return nil, ErrActionConflict
	}
	if err := s.exec.execute(ctx, userID, row.Kind, row.Payload, today); err != nil {
		return nil, err
	}
	upd, err := s.store.SetActionStatusFrom(ctx, actionID, userID, "done", nil, "proposed")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toActionView(upd)
	return &v, nil
}
```

(`CancelAction` igual con `"cancelled"` y sin execute. Nota: devuelven
`*ActionView`, ya no `*Message` — el `result` del ejecutor se conecta en la
Task 5; aquí se pasa `nil`.)

- Handler: `resolveAction` y la respuesta cambian a
  `{"action": ActionView}` (struct `actionResponse{Action ActionView}`);
  los handlers llaman igual.

- [ ] **Step 5: Adaptar tests.** Con TDD invertido aquí no aplica (es una
  mudanza); la guía es: **misma cobertura, nueva forma**.
  - `api/internal/store/ai_messages_test.go`: `TestAiMessageActionRoundTrip` y
    `TestAiMessageNewActionKinds` se reescriben contra `ai_actions`
    (CreateMessage normal + CreateAction; transición con `SetActionStatusFrom`
    incluyendo `done→undone` y doble transición → ErrNoRows; scoping). Agregar
    un test de la **migración de datos**: no se puede sembrar columnas viejas
    post-migración, así que en su lugar verificar vía SQL directo
    (`pool.Exec`) que la tabla existe y acepta las 7 kinds.
  - `api/internal/ai/chat_test.go`: `memStore` implementa la interfaz nueva
    (guarda `[]store.AiAction` aparte, genera IDs con `uuid.New()`); los tests
    de Confirm/Cancel usan el `actions[0].ID` del mensaje devuelto; los
    asserts de `msg.Action.X` pasan a `msg.Actions[0].X`.
  - `api/internal/ai/chat_handler_test.go`: `proposeViaChat` extrae
    `last["actions"].([]any)[0]["id"]`; los asserts de `action["status"]`
    leen `body["action"]` (respuesta nueva de confirm/cancel).
- [ ] **Step 6: Verificar backend completo + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/db api/internal/store api/internal/ai
git commit -m "feat(ai): tabla ai_actions (1:N) con migración de datos y servicio sobre actions[]"
```

(El frontend queda roto a propósito hasta la Task 7.)

---

### Task 3: `ChatStream` devuelve `[]ToolCall`

**Files:**
- Modify: `api/internal/ai/groq.go`, `api/internal/ai/chat.go` (interfaz + call site), `api/internal/ai/chat_test.go` y `api/internal/ai/handler_test.go` (fakes)
- Test: `api/internal/ai/groq_test.go`

- [ ] **Step 1: Test que falla** (reemplaza a `TestGroqChatStreamMultipleToolCallsFirstWins`, que se elimina):

```go
func TestGroqChatStreamMultipleToolCallsAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"registrar_checkin","arguments":"{\"mood\":8,"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"name":"marcar_habito","arguments":""}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"energy\":6,\"discipline\":9}"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"habit_id\":\"h1\"}"}}]}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	_, tcs, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}, nil, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(tcs) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(tcs))
	}
	if tcs[0].Name != "registrar_checkin" || tcs[0].Arguments != `{"mood":8,"energy":6,"discipline":9}` {
		t.Errorf("tc0 = %+v", tcs[0])
	}
	if tcs[1].Name != "marcar_habito" || tcs[1].Arguments != `{"habit_id":"h1"}` {
		t.Errorf("tc1 = %+v", tcs[1])
	}
}
```

Actualizar la firma en TODOS los tests existentes de ChatStream: el segundo
retorno es `[]ToolCall` (en turnos de una tool call, asserts sobre `tcs[0]`;
en turnos de texto, `len(tcs) == 0`).

- [ ] **Step 2: Implementar.** En `groq.go`, la acumulación pasa a mapa por
  index (reemplaza `tcName`/`tcArgs` y el `if tc.Index != 0 { continue }`):

```go
type tcAccum struct {
	name string
	args strings.Builder
}
// ...
accums := map[int]*tcAccum{}
maxIdx := -1
// dentro del loop de chunks:
for _, tc := range delta.ToolCalls {
	a, ok := accums[tc.Index]
	if !ok {
		a = &tcAccum{}
		accums[tc.Index] = a
		if tc.Index > maxIdx {
			maxIdx = tc.Index
		}
	}
	if tc.Function.Name != "" {
		a.name = tc.Function.Name
	}
	a.args.WriteString(tc.Function.Arguments)
}
// al cierre (tras sawDone):
var calls []ToolCall
for i := 0; i <= maxIdx; i++ {
	if a, ok := accums[i]; ok && a.name != "" {
		calls = append(calls, ToolCall{Name: a.name, Arguments: a.args.String()})
	}
}
if len(calls) > 0 {
	return full.String(), calls, nil
}
```

Firma: `(string, []ToolCall, error)`. Propagar a `chatStreamer`, a los fakes
(`fakeChatGroq.toolCall *ToolCall` → `toolCalls []ToolCall`;
`fakeCompleter.chatToolCall` → `chatToolCalls []ai.ToolCall`; los tests que
seteaban una sola pasan a slice de 1) y al call site de `SendStream`
(temporal: si `len(toolCalls) > 1` → `ErrUnavailable`; si es 1, el flujo
actual — la Task 4 lo reemplaza).

- [ ] **Step 3: Verificar backend + commit**

```bash
git add api/internal/ai
git commit -m "feat(ai): ChatStream reensambla todas las tool calls por index"
```

---

### Task 4: `SendStream` multi-acción (all-or-nothing) + prompt

**Files:**
- Modify: `api/internal/ai/chat.go`, `api/internal/ai/actions.go` (const), `api/internal/ai/chatprompt.go`
- Test: `api/internal/ai/chat_test.go`

- [ ] **Step 1: Tests que fallan:**

```go
func TestChatSendStreamMultipleActions(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{
		{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":7,"discipline":9}`},
		{Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`},
	}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	msg, err := svc.SendStream(context.Background(), uuid.New(), "check-in y meditación", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if len(msg.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(msg.Actions))
	}
	if msg.Actions[0].Kind != "checkin" || msg.Actions[1].Kind != "habito" {
		t.Errorf("kinds = %s, %s", msg.Actions[0].Kind, msg.Actions[1].Kind)
	}
	if msg.Content == "" {
		t.Error("contenido de fallback vacío")
	}
}

func TestChatSendStreamTooManyActionsDegrades(t *testing.T) {
	var calls []ToolCall
	for i := 0; i < 6; i++ {
		calls = append(calls, ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":7,"discipline":9}`})
	}
	st := &memStore{}
	groq := &fakeChatGroq{toolCalls: calls}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	if _, err := svc.SendStream(context.Background(), uuid.New(), "x", time.Now(), func(string) {}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("no debe persistir nada")
	}
}

func TestChatSendStreamOneInvalidActionDiscardsAll(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{
		{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":7,"discipline":9}`},
		{Name: "tool_inexistente", Arguments: `{}`},
	}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	if _, err := svc.SendStream(context.Background(), uuid.New(), "x", time.Now(), func(string) {}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("all-or-nothing: nada persistido")
	}
}
```

- [ ] **Step 2: Implementar.** En `actions.go`: `const maxActionsPerTurn = 5`.
  En `chat.go`, la rama de tool calls de `SendStream`:

```go
if len(toolCalls) > 0 {
	if len(toolCalls) > maxActionsPerTurn {
		return nil, ErrUnavailable
	}
	proposed := make([]ProposedAction, 0, len(toolCalls))
	summaries := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		kind, ok := toolNameToKind[tc.Name]
		if !ok {
			return nil, ErrUnavailable
		}
		payload, perr := parseActionPayload(kind, tc.Arguments)
		if perr != nil {
			return nil, ErrUnavailable
		}
		proposed = append(proposed, ProposedAction{Kind: kind, Payload: payload})
		summaries = append(summaries, actionSummary(kind, payload))
	}
	content := strings.TrimSpace(reply)
	if content == "" {
		content = strings.Join(summaries, " ")
	}
	assistant, actions, cerr := s.store.CreatePairWithActions(ctx, userID, text, content, proposed)
	if cerr != nil {
		return nil, cerr
	}
	v := toMessageView(assistant)
	for _, a := range actions {
		v.Actions = append(v.Actions, toActionView(a))
	}
	return &v, nil
}
```

  En `chatprompt.go`: reemplazar «Una sola acción por turno.» por «Puedes
  proponer hasta 5 acciones en un turno, solo las que el usuario pidió
  explícitamente; si pide varias cosas, propón una acción por cada una.»

- [ ] **Step 3: Verificar backend + commit**

```bash
git add api/internal/ai
git commit -m "feat(ai): multi-acción por turno con propuesta all-or-nothing (tope 5)"
```

---

### Task 5: Ejecutor con `result` + `UndoAction`

**Files:**
- Modify: `api/internal/ai/actions.go`, `api/internal/ai/chat.go`, `api/internal/server/server.go`, `api/internal/ai/handler_test.go` (wiring)
- Test: `api/internal/ai/actions_test.go`, `api/internal/ai/chat_test.go`

- [ ] **Step 1: Tests que fallan** (los esenciales; mantener cobertura de los existentes adaptando firmas):

```go
// execute ahora devuelve result. Asserts clave por kind:
func TestExecutorCheckinResultGuardaPrevYFecha(t *testing.T) {
	// fakeCheckinSvc gana un campo `today *checkin.CheckIn` que su Today() devuelve.
	// Caso A: sin check-in previo → result {"prev":null,"date":"2026-06-12"}.
	// Caso B: con previo {mood:5,...} → result con prev poblado.
}

func TestExecutorMovimientoResultGuardaTxID(t *testing.T) {
	// fakeFinanceSvc.Create devuelve Transaction{ID:"tx-1"} → result {"tx_id":"tx-1"}.
}

// UndoAction por kind (servicio, con memStore + fakes):
func TestUndoCheckinRestauraPrevio(t *testing.T)   // prev != null → Upsert(prev con la fecha del result)
func TestUndoCheckinSinPrevioBorra(t *testing.T)   // prev == null → Delete(userID, fecha del result)
func TestUndoMovimientoBorraYToleraInexistente(t *testing.T) // Delete devuelve false → undone igual
func TestUndoHabitoDesmarca(t *testing.T)          // SetCheck(false) con la fecha del result
func TestUndoMetaRestauraProgreso(t *testing.T)    // Patch(prev_progress); meta nil → undone igual
func TestUndoCreacionesBorran(t *testing.T)        // habito_nuevo/meta_nueva/entrenamiento → Delete del id del result
func TestUndoSoloUnaVez(t *testing.T)              // segundo undo → ErrActionConflict
func TestUndoDeCancelledEsConflicto(t *testing.T)
func TestUndoErrorDeDBNoTransiciona(t *testing.T)  // fake devuelve error real → la acción sigue done
```

Escribir cada test completo siguiendo el patrón de fakes del archivo (los
fakes ganan campos de retorno configurables: `fakeCheckinSvc.today`,
`fakeFinanceSvc.txID/deleted/err`, `fakeHabitsSvc` ya registra `done`,
`fakeGoalsSvc.goal *goals.Goal` para leer progreso previo,
`fakeHabitCreate.createdID`, etc., y fakes nuevos para los Delete:
`fakeFinanceSvc` gana `Delete`, `fakeHabitsSvc` gana `Delete`, `fakeGoalsSvc`
gana `Delete` y `Get`-equivalente vía `List`… — usar interfaces estrechas
nuevas, ver Step 2).

- [ ] **Step 2: Implementar.**

1. `actions.go` — interfaces ampliadas/nuevas (las existentes no cambian; se
   suman métodos vía interfaces nuevas para no romper fakes viejos):

```go
type checkinUndoer interface {
	Today(ctx context.Context, userID uuid.UUID, date time.Time) (*checkin.CheckIn, error)
	Delete(ctx context.Context, userID uuid.UUID, date time.Time) (bool, error)
}

type txDeleter interface {
	Delete(ctx context.Context, userID, id uuid.UUID) (bool, error)
}

type goalReader interface {
	List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error)
}

type habitDeleter interface {
	Delete(ctx context.Context, userID, habitID uuid.UUID) (bool, error)
}

type goalDeleter interface {
	Delete(ctx context.Context, userID, id uuid.UUID) (bool, error)
}

type workoutDeleter interface {
	DeleteWorkout(ctx context.Context, userID, id uuid.UUID) (bool, error)
}
```

   El `actionExecutor` agrupa por servicio (refactor de campos): en vez de 7+6
   campos sueltos, define interfaces compuestas por servicio real:

```go
type checkinSvc interface {
	checkinUpserter
	checkinUndoer
}
type financeSvc interface {
	txCreator
	txDeleter
}
type habitsSvc interface {
	habitChecker
	habitCreator
	habitDeleter
}
type goalsSvc interface {
	goalPatcher
	goalCreator
	goalDeleter
	goalReader
}
type trainingSvc interface {
	workoutCreator
	workoutDeleter
}

type actionExecutor struct {
	checkin  checkinSvc
	finance  financeSvc
	habits   habitsSvc
	goals    goalsSvc
	training trainingSvc
}

func NewActionExecutor(c checkinSvc, f financeSvc, h habitsSvc, g goalsSvc, t trainingSvc) *actionExecutor {
	return &actionExecutor{checkin: c, finance: f, habits: h, goals: g, training: t}
}
```

   (Wiring: `server.go` pasa `checkinSvc, financeSvc, habitsSvc, goalsSvc,
   trainingSvc`; `handler_test.go` `ci, fi, ha, go_, tr`. Los fakes de tests
   se consolidan igual: un fake por servicio que implementa su interfaz
   compuesta.)

2. `execute` pasa a `(json.RawMessage, error)`; tipos de result:

```go
type checkinResult struct {
	Prev *checkinPayload `json:"prev"`
	Date string          `json:"date"`
}
type idResult struct {
	ID string `json:"id"`
}
type metaResult struct {
	PrevProgress int32  `json:"prev_progress"`
	GoalID       string `json:"goal_id"`
}
type dateResult struct {
	HabitID string `json:"habit_id"`
	Date    string `json:"date"`
}
```

   Por kind (cada caso construye su result y lo marshalea):
   - checkin: lee `e.checkin.Today(ctx, userID, today)` ANTES del Upsert;
     prev = payload equivalente o nil; date = `today.Format("2006-01-02")`.
   - movimiento: `idResult{ID: tx.ID}` (Create ya devuelve la vista con ID).
   - habito: `dateResult{HabitID: p.HabitID, Date: today.Format(...)}`.
   - meta: lee el progreso actual vía `goalReader.List(userID, "activa", today)`
     buscando el goal_id (si no está en activas, buscar en "" o usar 0 — si la
     lista por status complica, aceptar `List(userID, "activa")` y si no
     aparece, prev_progress=0 con nota best-effort); result
     `metaResult{PrevProgress, GoalID}`.
   - habito_nuevo / meta_nueva / entrenamiento: `idResult` con el ID creado.

3. `undo(ctx, userID, kind string, payload, result []byte, today time.Time) error` en el ejecutor:

```go
func (e *actionExecutor) undo(ctx context.Context, userID uuid.UUID, kind string, payload, result []byte) error {
	switch kind {
	case actionCheckin:
		var r checkinResult
		if err := json.Unmarshal(result, &r); err != nil {
			return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
		}
		date, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			return fmt.Errorf("%w: fecha corrupta", ErrActionInvalid)
		}
		if r.Prev == nil {
			_, err := e.checkin.Delete(ctx, userID, date)
			return err
		}
		_, err = e.checkin.Upsert(ctx, userID, checkin.Input{
			Date: date, Mood: r.Prev.Mood, Energy: r.Prev.Energy,
			Discipline: r.Prev.Discipline, Note: r.Prev.Note,
		})
		return err
	case actionMovimiento:
		var r idResult
		_ = json.Unmarshal(result, &r)
		id, err := uuid.Parse(r.ID)
		if err != nil {
			return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
		}
		_, err = e.finance.Delete(ctx, userID, id) // false = ya no existe: ok
		return err
	case actionHabito:
		var r dateResult
		_ = json.Unmarshal(result, &r)
		date, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			return fmt.Errorf("%w: fecha corrupta", ErrActionInvalid)
		}
		_, err = e.habits.SetCheck(ctx, userID, uuid.MustParse(r.HabitID), date, false, date)
		return err
	case actionMeta:
		var r metaResult
		_ = json.Unmarshal(result, &r)
		prog := r.PrevProgress
		_, err := e.goals.Patch(ctx, userID, uuid.MustParse(r.GoalID), goals.GoalPatch{Progress: &prog}, time.Now().UTC())
		return err // Patch (nil,nil) si ya no existe → err nil: ok
	case actionHabitoNuevo:
		return undoDeleteByID(ctx, result, func(id uuid.UUID) (bool, error) { return e.habits.Delete(ctx, userID, id) })
	case actionMetaNueva:
		return undoDeleteByID(ctx, result, func(id uuid.UUID) (bool, error) { return e.goals.Delete(ctx, userID, id) })
	case actionEntrenamiento:
		return undoDeleteByID(ctx, result, func(id uuid.UUID) (bool, error) { return e.training.DeleteWorkout(ctx, userID, id) })
	}
	return fmt.Errorf("%w: kind %s", ErrActionInvalid, kind)
}

func undoDeleteByID(_ context.Context, result []byte, del func(uuid.UUID) (bool, error)) error {
	var r idResult
	_ = json.Unmarshal(result, &r)
	id, err := uuid.Parse(r.ID)
	if err != nil {
		return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
	}
	_, err = del(id) // false = ya no existe (best-effort): ok
	return err
}
```

   (Nota: el `habito` undo usa `MustParse(r.HabitID)` — el HabitID viene del
   payload validado; si prefieres, `uuid.Parse` con error → ErrActionInvalid.
   Usar Parse+error, más defensivo, en todos los casos.)

4. `chat.go`:
   - `ConfirmAction` recibe el result del execute y lo persiste:
     `result, err := s.exec.execute(...)` →
     `SetActionStatusFrom(actionID, userID, "done", result, "proposed")`.
   - `UndoAction`:

```go
func (s *ChatService) UndoAction(ctx context.Context, userID, actionID uuid.UUID) (*ActionView, error) {
	row, err := s.store.GetAction(ctx, actionID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.Status != "done" {
		return nil, ErrActionConflict
	}
	if err := s.exec.undo(ctx, userID, row.Kind, row.Payload, row.Result); err != nil {
		return nil, err
	}
	upd, err := s.store.SetActionStatusFrom(ctx, actionID, userID, "undone", nil, "done")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toActionView(upd)
	return &v, nil
}
```

- [ ] **Step 3: Verificar backend completo + commit**

```bash
git add api/internal/ai api/internal/server/server.go
git commit -m "feat(ai): execute devuelve result y UndoAction revierte por kind"
```

---

### Task 6: Endpoint `POST /ai/actions/{id}/undo`

**Files:**
- Modify: `api/internal/ai/handler.go`
- Test: `api/internal/ai/chat_handler_test.go`

- [ ] **Step 1: Tests que fallan:**

```go
func TestActionUndoHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: []ai.ToolCall{checkinToolCallTC()}}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "undo-ok@b.com")
	id := proposeViaChat(t, e, tok)
	if rec, _ := postAction(t, e.h, tok, id, "confirm"); rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d", rec.Code)
	}
	// El check-in existe…
	if _, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{UserID: uid, Date: dayTime(t)}); err != nil {
		t.Fatalf("check-in no escrito: %v", err)
	}
	// …deshacer…
	rec, body := postAction(t, e.h, tok, id, "undo")
	if rec.Code != http.StatusOK {
		t.Fatalf("undo = %d, body = %s", rec.Code, rec.Body.String())
	}
	action, _ := body["action"].(map[string]any)
	if action["status"] != "undone" {
		t.Errorf("status = %v", action["status"])
	}
	// …y el check-in del día desapareció (no había previo).
	if _, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{UserID: uid, Date: dayTime(t)}); err == nil {
		t.Error("el check-in debía borrarse al deshacer")
	}
	// Doble undo → 409.
	if rec2, _ := postAction(t, e.h, tok, id, "undo"); rec2.Code != http.StatusConflict {
		t.Errorf("doble undo = %d, want 409", rec2.Code)
	}
}

func TestActionUndoDeProposedEs409(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: []ai.ToolCall{checkinToolCallTC()}}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "undo-409@b.com")
	id := proposeViaChat(t, e, tok)
	if rec, _ := postAction(t, e.h, tok, id, "undo"); rec.Code != http.StatusConflict {
		t.Errorf("undo de proposed = %d, want 409", rec.Code)
	}
}
```

(`checkinToolCallTC()` = el helper existente `checkinToolCall()` adaptado si
en la Task 3 cambió a slices; usar el nombre real del archivo.)

- [ ] **Step 2: Implementar.** En `Routes`:
`r.Post("/actions/{id}/undo", handleActionUndo(chat))`; handler vía el
`resolveAction` existente:

```go
func handleActionUndo(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolveAction(w, r, func(ctx context.Context, userID, id uuid.UUID) (*ActionView, error) {
			return chat.UndoAction(ctx, userID, id)
		})
	}
}
```

(`resolveAction` ya cambió a `*ActionView` en la Task 2.)

- [ ] **Step 3: Verificar backend + commit**

```bash
git add api/internal/ai/handler.go api/internal/ai/chat_handler_test.go
git commit -m "feat(ai): endpoint POST /ai/actions/{id}/undo"
```

---

### Task 7: Frontend — N tarjetas + Deshacer

**Files:**
- Modify: `web/src/lib/ai.ts`, `web/src/lib/ai.test.ts`, `web/src/routes/asistente.tsx`, `web/src/routes/asistente.test.tsx`

- [ ] **Step 1: Adaptar la lib (TDD).** Tests primero en `ai.test.ts`:
  - `Message` pierde `action` y gana `actions?: Action[]`; `Action` gana `id: string`.
  - `confirmAction/cancelAction` devuelven `Action` (la respuesta es `{action}`) — actualizar mocks/asserts.
  - Nuevo `undoAction(id): Promise<Action>` → POST `/api/v1/ai/actions/{id}/undo`.

```ts
export function undoAction(id: string): Promise<Action> {
  return apiFetch<{ action: Action }>(`/api/v1/ai/actions/${id}/undo`, {
    method: "POST",
  }).then((r) => r.action);
}
```

  (`confirmAction`/`cancelAction` cambian su unwrap a `r.action` y su tipo de
  retorno a `Action`.)

- [ ] **Step 2: Adaptar la página (TDD).** Tests clave en `asistente.test.tsx`
  (adaptar los existentes de `action` singular a `actions: [...]` y agregar):

```tsx
it("un mensaje con dos acciones muestra dos tarjetas independientes", async () => {
  // mock: GET messages con un mensaje cuyo actions = [checkin proposed (id a1), habito proposed (id a2)]
  // mock: POST /ai/actions/a1/confirm → {action: {...a1, status:"done"}}
  // click Confirmar de la primera tarjeta → esa pasa a ✓ Hecha y la segunda sigue con botones.
});

it("una tarjeta done se puede deshacer y queda Deshecha", async () => {
  // mock: GET con acción done (id a1); POST /ai/actions/a1/undo → {action undone}
  // click Deshacer → aparece "Deshecha", no quedan botones.
});
```

  Implementación en `asistente.tsx`:
  - `ActionCard` recibe `action: Action` y `messageId` ya no se usa para
    resolver; `onResolve(action.id, verb)` con `verb: "confirm" | "cancel" | "undo"`.
  - Render: `{m.role === "assistant" && m.actions?.map((a) => <ActionCard key={a.id} action={a} ... />)}`.
  - `actionMutation` llama `confirmAction/cancelAction/undoAction` según verb y
    en `onSuccess` actualiza el caché reemplazando la acción anidada:

```ts
qc.setQueryData<Message[]>(["ai-messages"], (prev) =>
  (prev ?? []).map((m) =>
    m.actions?.some((a) => a.id === updated.id)
      ? { ...m, actions: m.actions.map((a) => (a.id === updated.id ? updated : a)) }
      : m
  )
);
```

  - Estados: `proposed` → Confirmar/Cancelar; `done` → `✓ Hecha` + botón
    `Deshacer` (`Button variant="ghost"` pequeño); `cancelled` → «Cancelada»;
    `undone` → «Deshecha» (`text-xs text-muted`).
  - Pending por tarjeta: `actionMutation.variables?.id === action.id`.

- [ ] **Step 3: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src
git commit -m "feat(web): tarjetas múltiples por mensaje y botón Deshacer"
```

---

### Task 8: Cierre — review, merge, deploy y smoke de producción

- [ ] **Step 1:** Suites completas + smoke local (`/tmp/smoke_actions.sh`,
  adaptarlo si asserta `"action"` singular en el done del stream — ahora es
  `"actions"`).
- [ ] **Step 2:** Review final holística (subagente), nits.
- [ ] **Step 3:** Merge `--no-ff` a `main` + push. **Verificar que el deploy
  ocurra** (si el usuario no activó el auto-deploy, pedirle el Deploy manual).
- [ ] **Step 4:** Smoke de producción: «registra mi check-in: 8 7 9 y marca
  meditación» (requiere un hábito existente: crearlo antes vía
  `crear_habito`) → 2 tarjetas → confirmar ambas → deshacer el check-in →
  `GET /checkins/today` vacío o con el previo.
- [ ] **Step 5:** Bitácora + push.

---

## Notas para el ejecutor

- **El frontend queda roto entre las Tasks 2 y 7** (contrato `action`→`actions`).
  No correr la suite web en las tareas de backend; exigirla verde desde la 7.
- Los nombres de params generados por sqlc mandan (`Status_2`, arrays
  `[]uuid.UUID` vs pgtype). Ajustar el código del plan a lo generado.
- Las migraciones 0011 se aplican solas (testutil y arranque del server).
- Tests existentes: la cobertura R11–R14 se conserva **adaptando la forma**
  (action→actions[0], message id→action id); no se borra ningún caso de
  comportamiento.
- Best-effort del undo: «ya no existe» (Delete devuelve false / Patch nil) NO
  es error; error real de DB SÍ aborta sin transicionar.
