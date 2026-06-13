# Plan 16 — Subir comprobantes a Finanzas (la IA extrae los movimientos) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** En Finanzas, subir una foto/CSV/PDF y que la IA extraiga movimientos como tarjetas de acción confirmables (reusando la maquinaria de la R15).

**Architecture:** Acciones de upload (`source='upload'`, sin mensaje de chat) creadas por un `ImportService` que extrae movimientos de un archivo vía Groq (visión para imágenes, texto para CSV/PDF) y los valida como `movimiento`. Las tarjetas se renderizan en Finanzas reusando un `ActionCard` extraído a `ui/`.

**Tech Stack:** Go + chi + sqlc/pgx + `github.com/ledongthuc/pdf` (PDF texto) + Groq (visión + texto, JSON mode); React + TanStack Query + Vitest.

**Spec:** `docs/superpowers/specs/2026-06-13-plan-16-importar-comprobantes-design.md`

**Entorno:** Go desde `api/` con `GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`; `sqlc generate` desde `api/`. Frontend `cd web && npx vitest run && npm run build`. Commits en español con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Rama: `plan-16-importar-comprobantes` desde `main`.

**Hechos del código (verificados):**
- Rutas montadas: `/finances`, `/ai`. Acciones bajo `/ai/actions/{id}/confirm|cancel|undo`. `ai.Routes(svc *Service, chat *ChatService)`.
- `ai_actions` (migración 0011): `id, message_id (NOT NULL), user_id, position, kind, payload JSONB, status, result JSONB, created_at`.
- `movimientoPayload{Type, AmountCentavos int64, Category, Remark string}` en `actions.go`; el caso `movimiento` del ejecutor hace `finance.Create(... OccurredOn: today ...)`.
- GroqClient: `send(ctx, []chatMessage, maxTokens)` hace POST a `/chat/completions`; `chatRequest{Model, Messages, Temperature, MaxTokens, Stream, Tools}`; `chatMessage{Role, Content string}`. Cliente `http` (10s) y `streamHTTP` (60s).
- Finanzas page: query keys `["finance","summary"]`, `["finance","list"]`, `["finance","cycles"]`. Lib `web/src/lib/finances.ts`.
- `ActionCard`, `ACTION_TITLES`, `actionDetails(action: Action)` viven en `web/src/routes/asistente.tsx`. `Action` se exporta desde `web/src/lib/ai.ts`.

---

### Task 1: Migración 0012 (acciones de upload) + queries/store

**Files:**
- Create: `api/db/migrations/0012_ai_actions_source.sql`
- Modify: `api/db/queries/ai_actions.sql`, `api/internal/ai/chatstore.go`, `api/internal/ai/chat.go` (interfaz)
- Test: `api/internal/store/ai_messages_test.go` (ampliar)
- Generated: `api/internal/store/`

- [ ] **Step 1: Migración** `api/db/migrations/0012_ai_actions_source.sql`:

```sql
-- +goose Up
ALTER TABLE ai_actions ALTER COLUMN message_id DROP NOT NULL;
ALTER TABLE ai_actions
    ADD COLUMN source TEXT NOT NULL DEFAULT 'chat'
        CHECK (source IN ('chat','upload'));
CREATE INDEX idx_ai_actions_upload ON ai_actions (user_id, source, status);

-- +goose Down
DROP INDEX idx_ai_actions_upload;
ALTER TABLE ai_actions DROP COLUMN source;
-- message_id se deja nullable: revertir a NOT NULL fallaría si hay filas de upload.
```

- [ ] **Step 2: Queries.** Agregar a `api/db/queries/ai_actions.sql`:

```sql
-- name: CreateUploadAction :one
INSERT INTO ai_actions (user_id, position, kind, payload, status, source)
VALUES ($1, $2, $3, $4, 'proposed', 'upload')
RETURNING *;

-- name: ListPendingUploadActions :many
SELECT * FROM ai_actions
WHERE user_id = $1 AND source = 'upload' AND status = 'proposed'
ORDER BY created_at, position;
```

Correr `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`. (`CreateAction` existente seguirá insertando `message_id` no nulo y `source` toma el default 'chat' — verificar que sqlc no rompa; si `CreateActionParams` requiere ahora `source`, dejar `CreateAction` como está y el default lo cubre. `store.AiAction` gana `MessageID *uuid.UUID` (nullable) y `Source string`.)

- [ ] **Step 3: Test que falla** en `api/internal/store/ai_messages_test.go`:

```go
func TestUploadActionRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Email: "upl@b.com", PasswordHash: "h", Name: "U"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	a, err := q.CreateUploadAction(ctx, store.CreateUploadActionParams{
		UserID: u.ID, Position: 0, Kind: "movimiento",
		Payload: []byte(`{"type":"expense","amount_centavos":25000,"category":"comida"}`),
	})
	if err != nil {
		t.Fatalf("CreateUploadAction: %v", err)
	}
	if a.Source != "upload" || a.Status != "proposed" || a.MessageID != nil {
		t.Errorf("acción upload mal creada: %+v", a)
	}

	pend, err := q.ListPendingUploadActions(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pend) != 1 || pend[0].ID != a.ID {
		t.Errorf("pending = %+v", pend)
	}

	// No aparece en el historial del chat (filtra por message_id).
	msgs, err := q.ListActionsByMessages(ctx, []uuid.UUID{a.ID})
	if err != nil {
		t.Fatalf("ListByMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("la acción upload no debe aparecer por message id: %+v", msgs)
	}
}
```

- [ ] **Step 4: Verificar que falla** (`go test ./internal/store/ -run TestUploadActionRoundTrip` — compila tras sqlc, falla si algo no cuadra; si pasa directo, está OK porque la query es la implementación).

- [ ] **Step 5: Store del chat** (`chatstore.go`). Agregar a `ProposedAction` reuso y métodos nuevos:

```go
// CreateUploadActions persiste N movimientos extraídos de un archivo como
// acciones source='upload' (sin mensaje de chat), en una transacción.
func (s *pgChatStore) CreateUploadActions(ctx context.Context, userID uuid.UUID, actions []ProposedAction) ([]store.AiAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	rows := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row, err := qtx.CreateUploadAction(ctx, store.CreateUploadActionParams{
			UserID: userID, Position: int32(i), Kind: a.Kind, Payload: a.Payload,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *pgChatStore) ListPendingUploadActions(ctx context.Context, userID uuid.UUID) ([]store.AiAction, error) {
	return s.q.ListPendingUploadActions(ctx, userID)
}
```

(No tocar la interfaz `messageStore` de `ChatService`; estos métodos los usará el `ImportService` vía una interfaz propia en la Task 5.)

- [ ] **Step 6: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./internal/ai/ ./internal/store/ -count=1
git add api/db api/internal/store api/internal/ai/chatstore.go
git commit -m "feat(ai): acciones source=upload (migración 0012, queries y store)"
```

---

### Task 2: `occurred_on` en el movimiento

**Files:**
- Modify: `api/internal/ai/actions.go`
- Test: `api/internal/ai/actions_test.go`

- [ ] **Step 1: Tests que fallan** en `actions_test.go`:

```go
func TestParseMovimientoOccurredOn(t *testing.T) {
	// Válido con fecha.
	if _, err := parseActionPayload("movimiento", `{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-05-10"}`); err != nil {
		t.Errorf("con occurred_on válido: %v", err)
	}
	// Sin fecha (retrocompatible).
	if _, err := parseActionPayload("movimiento", `{"type":"income","amount_centavos":1,"category":"x"}`); err != nil {
		t.Errorf("sin occurred_on: %v", err)
	}
	// Fecha malformada.
	if _, err := parseActionPayload("movimiento", `{"type":"expense","amount_centavos":1,"category":"x","occurred_on":"mayo"}`); err == nil {
		t.Error("occurred_on malformado debe fallar")
	}
}

func TestExecutorMovimientoUsaFechaDelPayload(t *testing.T) {
	fin := &fakeFinanceSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	if _, err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-05-10"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	if !fin.in.OccurredOn.Equal(want) {
		t.Errorf("OccurredOn = %v, want %v (la fecha del payload)", fin.in.OccurredOn, want)
	}
}

func TestExecutorMovimientoSinFechaUsaToday(t *testing.T) {
	fin := &fakeFinanceSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	if _, err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":25000,"category":"comida"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fin.in.OccurredOn.Equal(today) {
		t.Errorf("OccurredOn = %v, want today %v", fin.in.OccurredOn, today)
	}
}
```

(El fake `fakeFinanceSvc` ya captura el `Input` en `fin.in` — verificar el nombre real del campo; si difiere, ajustar.)

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar** en `actions.go`:

`movimientoPayload` gana el campo:

```go
type movimientoPayload struct {
	Type           string `json:"type"`
	AmountCentavos int64  `json:"amount_centavos"`
	Category       string `json:"category"`
	Remark         string `json:"remark,omitempty"`
	OccurredOn     string `json:"occurred_on,omitempty"` // YYYY-MM-DD; "" = hoy
}
```

En `parseActionPayload`, el caso `actionMovimiento`, antes del `return json.Marshal(p)`:

```go
if p.OccurredOn != "" {
	if _, err := time.Parse("2006-01-02", p.OccurredOn); err != nil {
		return nil, fmt.Errorf("occurred_on inválido (YYYY-MM-DD)")
	}
}
```

En el ejecutor, el caso `actionMovimiento`:

```go
case actionMovimiento:
	var p movimientoPayload
	_ = json.Unmarshal(normalized, &p)
	occurred := today
	if p.OccurredOn != "" {
		occurred, _ = time.Parse("2006-01-02", p.OccurredOn) // ya validado
	}
	_, err := e.finance.Create(ctx, userID, finance.Input{
		Type: p.Type, Amount: p.AmountCentavos, OccurredOn: occurred, Category: p.Category, Remark: p.Remark,
	})
	return err
```

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai/actions.go api/internal/ai/actions_test.go
git commit -m "feat(ai): occurred_on opcional en movimiento (fecha detectada del comprobante)"
```

---

### Task 3: Cliente Groq — visión y extracción de texto con JSON mode

**Files:**
- Modify: `api/internal/ai/groq.go`
- Test: `api/internal/ai/groq_test.go`

- [ ] **Step 1: Tests que fallan** en `groq_test.go`:

```go
func TestGroqExtractTextJSONMode(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"movimientos\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	got, err := c.ExtractText(context.Background(), "sys", "datos csv")
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if got != `{"movimientos":[]}` {
		t.Errorf("content = %q", got)
	}
	body := string(gotBody)
	if !strings.Contains(body, `"response_format":{"type":"json_object"}`) {
		t.Errorf("falta response_format json_object: %s", body)
	}
}

func TestGroqExtractVisionSendsImage(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"movimientos\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "vision-model")
	got, err := c.ExtractVision(context.Background(), "sys", "aGVsbG8=", "image/png")
	if err != nil {
		t.Fatalf("ExtractVision: %v", err)
	}
	if got != `{"movimientos":[]}` {
		t.Errorf("content = %q", got)
	}
	body := string(gotBody)
	for _, want := range []string{`"vision-model"`, `"type":"image_url"`, `data:image/png;base64,aGVsbG8=`, `"response_format"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqExtractVisionHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()
	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.ExtractVision(context.Background(), "s", "x", "image/png"); err == nil {
		t.Fatal("esperaba error en HTTP 400")
	}
}
```

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar** en `groq.go`.

`chatRequest` gana `ResponseFormat`:

```go
type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	Stream         bool            `json:"stream,omitempty"`
	Tools          []openaiTool    `json:"tools,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}
```

Tipos y request de visión + extracción (al final del archivo). El cliente de
visión usa un modelo distinto (`visionModel`), inyectado:

```go
// GroqClient gana el modelo de visión.
// (Agregar el campo visionModel a la struct y a newGroqClient.)

// jsonRequest es como chatRequest pero el content de un mensaje puede ser un
// array (texto + imagen) para los modelos de visión.
type visionContentPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *visionImageURL   `json:"image_url,omitempty"`
}
type visionImageURL struct {
	URL string `json:"url"`
}
type visionMessage struct {
	Role    string              `json:"role"`
	Content []visionContentPart `json:"content"`
}
type visionRequest struct {
	Model          string          `json:"model"`
	Messages       []json.RawMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// ExtractText manda system+user al modelo de TEXTO con JSON mode (CSV / PDF-texto).
func (c *GroqClient) ExtractText(ctx context.Context, system, user string) (string, error) {
	return c.sendJSON(ctx, c.model, []json.RawMessage{
		rawMsg("system", system), rawMsg("user", user),
	})
}

// ExtractVision manda system + imagen base64 al modelo de VISIÓN con JSON mode.
func (c *GroqClient) ExtractVision(ctx context.Context, system, b64, mime string) (string, error) {
	userMsg := visionMessage{Role: "user", Content: []visionContentPart{
		{Type: "text", Text: "Extrae los movimientos del comprobante."},
		{Type: "image_url", ImageURL: &visionImageURL{URL: "data:" + mime + ";base64," + b64}},
	}}
	raw, _ := json.Marshal(userMsg)
	return c.sendJSON(ctx, c.visionModel, []json.RawMessage{rawMsg("system", system), raw})
}

func rawMsg(role, content string) json.RawMessage {
	b, _ := json.Marshal(chatMessage{Role: role, Content: content})
	return b
}

// sendJSON hace el POST con response_format json_object y max_tokens amplio
// (las extracciones pueden devolver muchos movimientos). Reusa el cliente de
// 60s porque la visión puede tardar.
func (c *GroqClient) sendJSON(ctx context.Context, model string, msgs []json.RawMessage) (string, error) {
	reqBody, err := json.Marshal(visionRequest{
		Model: model, Messages: msgs, Temperature: 0.2, MaxTokens: 2000,
		ResponseFormat: &responseFormat{Type: "json_object"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.streamHTTP.Do(req)
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

Agregar el campo `visionModel string` a `GroqClient`; `newGroqClient` y
`NewGroqClient` ganan el parámetro (firma: `NewGroqClient(apiKey, model,
visionModel string)`). Actualizar el call site en `server.go` (Task 5).

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... 2>&1 | head
# server.go romperá por la firma de NewGroqClient; se arregla en Task 5. Correr solo el test del paquete con build de test:
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -run TestGroqExtract -count=1
```

Si el `go build ./...` falla solo por `server.go` (firma de NewGroqClient), está
esperado — corregir ya el call site mínimamente para no dejar el árbol roto:
en `server.go`, `ai.NewGroqClient(d.GroqAPIKey, d.GroqModel, d.GroqVisionModel)`
requiere `d.GroqVisionModel` (Task 5). Para cerrar esta task de forma atómica,
pasar temporalmente `d.GroqModel` como tercer arg: `ai.NewGroqClient(d.GroqAPIKey, d.GroqModel, d.GroqModel)` y dejar el TODO para Task 5. Verificar `go build ./...` y el test del paquete verde.

```bash
git add api/internal/ai/groq.go api/internal/ai/groq_test.go api/internal/server/server.go
git commit -m "feat(ai): GroqClient.ExtractText/ExtractVision con JSON mode y modelo de visión"
```

---

### Task 4: Extractor (archivo → movimientos validados)

**Files:**
- Create: `api/internal/ai/extract.go`, `api/internal/ai/pdftext.go`, `api/internal/ai/extractprompt.go`
- Create: `api/internal/ai/extract_test.go`
- Create: `api/internal/ai/testdata/sample.csv`, `api/internal/ai/testdata/sample.txt.pdf` (PDF de texto mínimo)
- Modify: `api/go.mod` (dep `ledongthuc/pdf`)

- [ ] **Step 1: Dependencia PDF**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go get github.com/ledongthuc/pdf
```

- [ ] **Step 2: Tests que fallan** — `api/internal/ai/extract_test.go`:

```go
package ai

import (
	"context"
	"strings"
	"testing"
)

// fakeExtractor implementa la interfaz que usa el extractor para llamar a Groq.
type fakeExtractClient struct {
	out      string
	err      error
	gotImage bool
}

func (f *fakeExtractClient) ExtractText(ctx context.Context, system, user string) (string, error) {
	return f.out, f.err
}
func (f *fakeExtractClient) ExtractVision(ctx context.Context, system, b64, mime string) (string, error) {
	f.gotImage = true
	return f.out, f.err
}

func TestExtractCSVMovements(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[
		{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-06-10"},
		{"type":"income","amount_centavos":500000,"category":"sueldo"}]}`}
	ex := newExtractor(gc)
	res, err := ex.extract(context.Background(), []byte("fecha,monto,desc\n..."), "text/csv", "x.csv")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(res.actions) != 2 || res.dropped != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestExtractDropsInvalid(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[
		{"type":"expense","amount_centavos":25000,"category":"comida"},
		{"type":"transfer","amount_centavos":1,"category":"x"},
		{"type":"expense","amount_centavos":0,"category":"y"}]}`}
	ex := newExtractor(gc)
	res, err := ex.extract(context.Background(), []byte("..."), "text/csv", "x.csv")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(res.actions) != 1 || res.dropped != 2 {
		t.Errorf("res = %+v (esperaba 1 válido, 2 descartados)", res)
	}
}

func TestExtractZeroValidIsError(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[{"type":"transfer","amount_centavos":1,"category":"x"}]}`}
	ex := newExtractor(gc)
	if _, err := ex.extract(context.Background(), []byte("..."), "text/csv", "x.csv"); err == nil {
		t.Error("cero válidos debe ser error")
	}
}

func TestExtractImageUsesVision(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[{"type":"expense","amount_centavos":25000,"category":"comida"}]}`}
	ex := newExtractor(gc)
	if _, err := ex.extract(context.Background(), []byte{0x89, 0x50}, "image/png", "r.png"); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !gc.gotImage {
		t.Error("imagen debe ir por ExtractVision")
	}
}

func TestExtractUnsupportedType(t *testing.T) {
	ex := newExtractor(&fakeExtractClient{})
	if _, err := ex.extract(context.Background(), []byte("x"), "application/zip", "x.zip"); err == nil {
		t.Error("tipo no soportado debe fallar")
	}
}

func TestPdfTextFromSample(t *testing.T) {
	// sample.txt.pdf contiene texto extraíble; verificar que pdfText lo lee.
	data := readTestdata(t, "sample.txt.pdf")
	txt, err := pdfText(data)
	if err != nil {
		t.Fatalf("pdfText: %v", err)
	}
	if strings.TrimSpace(txt) == "" {
		t.Error("pdfText devolvió vacío para un PDF con texto")
	}
}

func TestPdfEmptyIsScannedError(t *testing.T) {
	// Un PDF sin texto debe dar el error de "escaneado" en extract.
	ex := newExtractor(&fakeExtractClient{out: `{"movimientos":[]}`})
	_, err := ex.extract(context.Background(), []byte("%PDF-1.4 sin texto real"), "application/pdf", "s.pdf")
	if err == nil {
		t.Error("PDF ilegible/escaneado debe fallar con mensaje claro")
	}
}
```

(El helper `readTestdata(t, name)` lee `testdata/<name>` con `os.ReadFile`;
agregarlo si no existe. El `sample.txt.pdf` se genera en el Step siguiente.)

- [ ] **Step 3: Generar los testdata.** `testdata/sample.csv`:

```
fecha,descripcion,monto
2026-06-10,Rappi comida,-250.00
2026-06-11,Sueldo,5000.00
```

`testdata/sample.txt.pdf`: un PDF mínimo con texto. Generarlo con Go en un
pequeño programa efímero, o incluir bytes de un PDF de texto conocido. La forma
robusta: crearlo en el test setup no es ideal (binario). Generarlo una vez con:

```bash
cd /Users/gustavo/Desktop/focus-365/api && printf '%%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n4 0 obj<</Length 58>>stream\nBT /F1 12 Tf 72 700 Td (Rappi comida 250.00 MXN) Tj ET\nendstream endobj\n5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\nxref\n0 6\n0000000000 65535 f \ntrailer<</Root 1 0 R/Size 6>>\nstartxref\n0\n%%%%EOF\n' > internal/ai/testdata/sample.txt.pdf
```

Si `ledongthuc/pdf` no lee ese PDF mínimo (xref simplificado), generar uno
válido con un script Go efímero usando una lib de generación, o tomar un PDF de
texto real pequeño. **El test `TestPdfTextFromSample` valida exactamente esto:
si el PDF de muestra no es legible, ajustarlo hasta que `pdfText` devuelva el
texto.**

- [ ] **Step 4: Implementar.**

`api/internal/ai/pdftext.go`:

```go
package ai

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// pdfText extrae el texto plano de un PDF en memoria. Devuelve "" (sin error)
// si el PDF no tiene texto extraíble (escaneado). Recupera de panics de la lib.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf ilegible: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf inválido: %w", err)
	}
	rd, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("pdf sin texto: %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rd); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

`api/internal/ai/extractprompt.go`:

```go
package ai

// extractSystemPrompt instruye al modelo a devolver SOLO JSON con los
// movimientos detectados en el comprobante.
const extractSystemPrompt = `Eres un extractor de movimientos financieros. Recibes el contenido de un comprobante (recibo, ticket, estado de cuenta o CSV) y devuelves SOLO un objeto JSON con esta forma exacta:
{"movimientos":[{"type":"income|expense","amount_centavos":<entero positivo en centavos>,"category":"<categoría corta>","remark":"<opcional>","occurred_on":"YYYY-MM-DD (opcional, la fecha del movimiento si aparece)"}]}
Reglas: gastos/cargos = expense, ingresos/abonos = income. El monto SIEMPRE en centavos enteros y positivo (ej: $250.00 → 25000). Si no hay fecha clara, omite occurred_on. No incluyas transferencias internas. No inventes montos. Si no hay movimientos, devuelve {"movimientos":[]}. Responde ÚNICAMENTE el JSON, sin texto adicional.`
```

`api/internal/ai/extract.go`:

```go
package ai

import (
	"context"
	"encoding/csv"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxCSVRows   = 50
	maxTextChars = 12000
)

// extractClient es lo que el extractor necesita de Groq (fakeable).
type extractClient interface {
	ExtractText(ctx context.Context, system, user string) (string, error)
	ExtractVision(ctx context.Context, system, b64, mime string) (string, error)
}

type extractor struct {
	groq extractClient
}

func newExtractor(c extractClient) *extractor { return &extractor{groq: c} }

// extractResult: las acciones movimiento validadas + cuántas se descartaron +
// si la entrada se truncó (CSV largo).
type extractResult struct {
	actions   []ProposedAction
	dropped   int
	truncated bool
}

type extractedMovs struct {
	Movimientos []json.RawMessage `json:"movimientos"`
}

// extract detecta el tipo, obtiene el JSON del modelo y valida cada movimiento.
func (e *extractor) extract(ctx context.Context, data []byte, mime, filename string) (*extractResult, error) {
	var raw string
	var err error
	truncated := false

	switch {
	case mime == "image/jpeg" || mime == "image/png" || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg") || strings.HasSuffix(filename, ".png"):
		b64 := base64.StdEncoding.EncodeToString(data)
		raw, err = e.groq.ExtractVision(ctx, extractSystemPrompt, b64, imageMime(mime, filename))
	case mime == "text/csv" || strings.HasSuffix(filename, ".csv"):
		var text string
		text, truncated = csvToText(data)
		raw, err = e.groq.ExtractText(ctx, extractSystemPrompt, text)
	case mime == "application/pdf" || strings.HasSuffix(filename, ".pdf"):
		text, perr := pdfText(data)
		if perr != nil || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("el PDF parece escaneado o ilegible; súbelo como foto")
		}
		if len(text) > maxTextChars {
			text = text[:maxTextChars]
			truncated = true
		}
		raw, err = e.groq.ExtractText(ctx, extractSystemPrompt, text)
	default:
		return nil, fmt.Errorf("formato no soportado: %s", mime)
	}
	if err != nil {
		return nil, err
	}

	var parsed extractedMovs
	if jerr := json.Unmarshal([]byte(raw), &parsed); jerr != nil {
		return nil, fmt.Errorf("respuesta del modelo no es JSON válido")
	}

	res := &extractResult{truncated: truncated}
	for _, m := range parsed.Movimientos {
		payload, verr := parseActionPayload("movimiento", string(m))
		if verr != nil {
			res.dropped++
			continue
		}
		res.actions = append(res.actions, ProposedAction{Kind: "movimiento", Payload: payload})
	}
	if len(res.actions) == 0 {
		return nil, fmt.Errorf("no pude leer movimientos en el archivo")
	}
	return res, nil
}

func imageMime(mime, filename string) string {
	if mime == "image/jpeg" || mime == "image/png" {
		return mime
	}
	if strings.HasSuffix(filename, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

// csvToText lee hasta maxCSVRows filas (más cabecera) y las re-serializa como
// texto para el prompt. Devuelve si truncó.
func csvToText(data []byte) (string, bool) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	var b strings.Builder
	rows := 0
	truncated := false
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if rows >= maxCSVRows+1 { // +1 por la cabecera
			truncated = true
			break
		}
		b.WriteString(strings.Join(rec, ","))
		b.WriteByte('\n')
		rows++
	}
	return b.String(), truncated
}
```

(Nota: `parseActionPayload("movimiento", ...)` usa `DisallowUnknownFields`; el
JSON del modelo puede traer campos extra. Si eso descarta válidos, relajar SOLO
para el extractor: una variante `parseMovimientoLenient` que ignore campos
desconocidos pero aplique las mismas reglas. Decidir según el comportamiento
real del test `TestExtractCSVMovements` — si pasa con DisallowUnknownFields,
dejar como está; si el modelo mete extras, agregar la variante lenient.)

- [ ] **Step 5: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai/extract.go api/internal/ai/pdftext.go api/internal/ai/extractprompt.go api/internal/ai/extract_test.go api/internal/ai/testdata api/go.mod api/go.sum
git commit -m "feat(ai): extractor de movimientos desde imagen/CSV/PDF con validación lenient"
```

---

### Task 5: ImportService, endpoints, wiring y config

**Files:**
- Create: `api/internal/ai/import.go`
- Modify: `api/internal/ai/handler.go`, `api/internal/server/server.go`, `api/internal/config/config.go`
- Modify: `docker-compose.yml`, `docker-compose.coolify.yml`, `.env.example`
- Test: `api/internal/ai/chat_handler_test.go` (o nuevo `import_handler_test.go`), `api/internal/config/config_test.go`

- [ ] **Step 1: Config `GROQ_VISION_MODEL`.** En `config.go`, `Config` gana
`GroqVisionModel string`; en `Load`:

```go
GroqVisionModel: os.Getenv("GROQ_VISION_MODEL"),
// default tras leer:
if cfg.GroqVisionModel == "" {
	cfg.GroqVisionModel = "meta-llama/llama-4-scout-17b-16e-instruct"
}
```

Test en `config_test.go`: sin env → el default; con env → el valor.

- [ ] **Step 2: ImportService** `api/internal/ai/import.go`:

```go
package ai

import (
	"context"
	"errors"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// uploadStore es lo que ImportService necesita del store (fakeable).
type uploadStore interface {
	CreateUploadActions(ctx context.Context, userID uuid.UUID, actions []ProposedAction) ([]store.AiAction, error)
	ListPendingUploadActions(ctx context.Context, userID uuid.UUID) ([]store.AiAction, error)
}

// ImportService extrae movimientos de un archivo y los persiste como acciones
// de upload propuestas.
type ImportService struct {
	ex     *extractor
	store  uploadStore
	hasKey bool
}

func NewImportService(c extractClient, st uploadStore, hasKey bool) *ImportService {
	return &ImportService{ex: newExtractor(c), store: st, hasKey: hasKey}
}

// ImportResult es la vista del resultado de una importación.
type ImportResult struct {
	Created   []ActionView
	Dropped   int
	Truncated bool
}

func (s *ImportService) Import(ctx context.Context, userID uuid.UUID, data []byte, mime, filename string) (*ImportResult, error) {
	if !s.hasKey {
		return nil, ErrUnavailable
	}
	res, err := s.ex.extract(ctx, data, mime, filename)
	if err != nil {
		return nil, err // errores "de negocio" (escaneado, cero, formato) → el handler los mapea
	}
	rows, err := s.store.CreateUploadActions(ctx, userID, res.actions)
	if err != nil {
		return nil, err
	}
	out := &ImportResult{Dropped: res.dropped, Truncated: res.truncated}
	for _, r := range rows {
		out.Created = append(out.Created, toActionView(r))
	}
	return out, nil
}

func (s *ImportService) Pending(ctx context.Context, userID uuid.UUID) ([]ActionView, error) {
	rows, err := s.store.ListPendingUploadActions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ActionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActionView(r))
	}
	return out, nil
}

// ErrExtract distingue fallos de extracción "de negocio" (4xx) de los de Groq.
var ErrExtractFailed = errors.New("extracción fallida")
```

(El extractor ya devuelve errores con mensajes claros; el handler decide el
status. Para distinguir "sin clave" → 503 vs extracción → 422, `Import` devuelve
`ErrUnavailable` solo en el guard de clave; los demás errores del extractor se
mapean a 422 con su mensaje. Los errores de Groq dentro de `extract` salen como
errores genéricos → el handler los trata como 503: para distinguirlos, el
extractor puede envolver los fallos de red de Groq en `ErrUnavailable`. Decisión
simple: en `extract.go`, tras `err != nil` de las llamadas a Groq,
`return nil, ErrUnavailable`. Los errores de validación/escaneado/formato/cero
quedan como errores con mensaje → 422.)

**Ajuste en `extract.go`:** envolver los fallos de las llamadas Groq:
`if err != nil { return nil, ErrUnavailable }` (en vez de `return nil, err`).

- [ ] **Step 3: Handlers** en `handler.go`. `Routes` gana el parámetro y rutas:

```go
func Routes(svc *Service, chat *ChatService, imp *ImportService) http.Handler {
	// ... rutas existentes ...
	r.Post("/import", handleImport(imp))
	r.Get("/import/pending", handleImportPending(imp))
	return r
}

const maxUploadBytes = 8 << 20 // 8 MB

func handleImport(imp *ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			httpx.WriteErr(w, http.StatusRequestEntityTooLarge, "archivo demasiado grande (máx 8 MB)")
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "falta el archivo")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error leyendo el archivo")
			return
		}
		if len(data) > maxUploadBytes {
			httpx.WriteErr(w, http.StatusRequestEntityTooLarge, "archivo demasiado grande (máx 8 MB)")
			return
		}
		mime := hdr.Header.Get("Content-Type")
		res, err := imp.Import(r.Context(), userID, data, mime, hdr.Filename)
		if err != nil {
			switch {
			case errors.Is(err, ErrUnavailable):
				httpx.WriteErr(w, http.StatusServiceUnavailable, "asistente no disponible por ahora")
			default:
				// errores de extracción (formato, escaneado, cero movimientos)
				httpx.WriteErr(w, http.StatusUnprocessableEntity, err.Error())
			}
			return
		}
		httpx.WriteJSON(w, http.StatusOK, importResponse{
			Created: res.Created, Dropped: res.Dropped, Truncated: res.Truncated,
		})
	}
}

type importResponse struct {
	Created   []ActionView `json:"created"`
	Dropped   int          `json:"dropped"`
	Truncated bool         `json:"truncated"`
}

func handleImportPending(imp *ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		acts, err := imp.Pending(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"actions": acts})
	}
}
```

(Asegurar imports `io`, `errors` en handler.go.)

- [ ] **Step 4: Wiring** en `server.go`:

```go
groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel, d.GroqVisionModel)
// ... aiSvc, chatSvc como están ...
importSvc := ai.NewImportService(groq, chatStore, d.GroqAPIKey != "")
r.Mount("/ai", ai.Routes(aiSvc, chatSvc, importSvc))
```

`Deps` gana `GroqVisionModel string`; `New` lo pasa desde `cfg.GroqVisionModel`
en `main.go`. (El `groq` ya implementa `extractClient` con ExtractText/ExtractVision.)

- [ ] **Step 5: Env.** En `docker-compose.yml` y `docker-compose.coolify.yml`,
servicio `api`, agregar `GROQ_VISION_MODEL: ${GROQ_VISION_MODEL:-meta-llama/llama-4-scout-17b-16e-instruct}`. En `.env.example`, documentar la variable.

- [ ] **Step 6: Tests de integración** (en `chat_handler_test.go` o nuevo). El
`newEnv` debe construir un `ImportService` con un fake `extractClient`. Helper
para POST multipart:

```go
func postImport(t *testing.T, h http.Handler, tok, filename, ctype string, body []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	hdr.Set("Content-Type", ctype)
	part, _ := mw.CreatePart(hdr)
	part.Write(body)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/ai/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}
```

Tests: import CSV feliz (fake extractClient devuelve 2 movimientos → 200,
`created` con 2, `dropped` 0; y `GET /ai/import/pending` los lista); tipo no
soportado → 422; sin token → 401; sin clave (env hasKey=false) → 503.

**Importante:** `newEnv` necesita inyectar el fake extractClient en el
ImportService. Como `ai.Routes` ahora recibe 3 args, actualizar `newEnv` para
construir `ai.NewImportService(fakeExtract, chatStore, hasKey)` y pasarlo. El
fake puede ser un `*fakeExtractClient` (ya existe en extract_test.go pero es de
otro paquete `ai` vs `ai_test`; crear un fake exportable mínimo o un
`fakeCompleter`-style en el test de integración).

- [ ] **Step 7: Verificar backend completo + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/internal/ai api/internal/server/server.go api/internal/config api/cmd docker-compose.yml docker-compose.coolify.yml .env.example
git commit -m "feat(ai): ImportService y endpoints POST /ai/import + GET /ai/import/pending"
```

---

### Task 6: Frontend — extraer `ActionCard` a `ui/`

**Files:**
- Create: `web/src/ui/ActionCard.tsx`
- Modify: `web/src/routes/asistente.tsx`
- Test: `web/src/ui/ActionCard.test.tsx` (nuevo, mínimo)

- [ ] **Step 1: Test** `web/src/ui/ActionCard.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionCard } from "./ActionCard";
import type { Action } from "@/lib/ai";

const base: Action = { id: "a1", kind: "movimiento", payload: { type: "expense", amount_centavos: 25000, category: "comida" }, status: "proposed" };

describe("ActionCard", () => {
  it("muestra título y detalle del movimiento y botones en proposed", () => {
    render(<ActionCard action={base} pending={false} onResolve={() => {}} />);
    expect(screen.getByText("Movimiento")).toBeInTheDocument();
    expect(screen.getByText(/Gasto de \$250\.00 en comida/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Confirmar" })).toBeInTheDocument();
  });

  it("done muestra Hecha + Deshacer; resolver llama onResolve", async () => {
    const onResolve = vi.fn();
    render(<ActionCard action={{ ...base, status: "done" }} pending={false} onResolve={onResolve} />);
    await userEvent.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(onResolve).toHaveBeenCalledWith("a1", "undo");
  });
});
```

- [ ] **Step 2: Verificar que falla.**

- [ ] **Step 3: Extraer.** Mover de `asistente.tsx` a `web/src/ui/ActionCard.tsx`
`ACTION_TITLES`, `actionDetails` y el componente `ActionCard` **sin cambiar su
código** (export named `ActionCard`). En `asistente.tsx`, borrar esas
definiciones e `import { ActionCard } from "@/ui/ActionCard"`. El `ActionCard`
debe recibir exactamente las props que ya usa (`action`, `pending`, `onResolve`).

- [ ] **Step 4: Verificar suite + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/ui/ActionCard.tsx web/src/ui/ActionCard.test.tsx web/src/routes/asistente.tsx
git commit -m "refactor(web): ActionCard compartido en ui/ (chat y finanzas)"
```

---

### Task 7: Frontend — zona de subida en Finanzas

**Files:**
- Modify: `web/src/lib/ai.ts`, `web/src/lib/ai.test.ts`, `web/src/routes/finanzas.tsx`, `web/src/routes/finanzas.test.tsx`

- [ ] **Step 1: Lib (TDD).** En `ai.ts`:

```ts
export type ImportResult = { created: Action[]; dropped: number; truncated: boolean };

export async function importFile(file: File): Promise<ImportResult> {
  const headers: Record<string, string> = {};
  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const form = new FormData();
  form.append("file", file);
  const res = await fetch("/api/v1/ai/import", { method: "POST", headers, body: form, credentials: "include" });
  if (!res.ok) {
    let msg = `Error ${res.status}`;
    try { const b = await res.json(); if (b?.error) msg = b.error; } catch { /* */ }
    throw new ApiError(msg, res.status);
  }
  return (await res.json()) as ImportResult;
}

export function getPendingUploads(): Promise<Action[]> {
  return apiFetch<{ actions: Action[] }>("/api/v1/ai/import/pending").then((r) => r.actions);
}
```

Tests en `ai.test.ts`: `importFile` hace POST multipart a `/api/v1/ai/import` y
devuelve `{created, dropped, truncated}`; propaga 422 como ApiError;
`getPendingUploads` hace GET y devuelve el array. (Para el multipart, basta
verificar `url`, `method` y que `body` sea un `FormData`.)

- [ ] **Step 2: Página (TDD).** En `finanzas.test.tsx`, agregar tests:

```tsx
it("subir un archivo muestra las tarjetas extraídas", async () => {
  // mock fetch: POST /ai/import → {created:[movimiento proposed], dropped:0, truncated:false}
  //            GET /ai/import/pending → {actions:[]}
  //            otras queries de finanzas → datos mínimos
  // simular input file change con un File, esperar a ver "Movimiento" y "Confirmar".
});

it("Confirmar todos confirma cada pendiente", async () => {
  // pending devuelve 2 movimientos proposed; click "Confirmar todos" → 2 POST a /ai/actions/{id}/confirm
});
```

(Escribir completos siguiendo el harness existente de `finanzas.test.tsx`:
`MotionConfig reducedMotion="always"`, mock de `@/lib/auth`, router de prueba.
Usar `userEvent.upload(input, file)` para el input file.)

- [ ] **Step 3: Implementar** en `finanzas.tsx`:
- Importar `ActionCard` de `@/ui/ActionCard`, `importFile`/`getPendingUploads` y
  `confirmAction`/`cancelAction`/`undoAction` de `@/lib/ai`, tipo `Action`.
- `useQuery(["finance","uploads"], getPendingUploads)` para las pendientes.
- Estado local de la subida: `uploading`, `error`, `note` (ej. «extraje 5, 1 no
  se pudo leer» / «truncado a 50 filas»).
- Una `Card` «Subir comprobante» con `<input type="file" accept="image/*,.csv,.pdf">`
  (label estilada como drop zone). `onChange`: `importFile(file)` → en éxito,
  invalidar `["finance","uploads"]` y setear la nota con `dropped`/`truncated`;
  en error, mostrar `err.message`.
- Render de las tarjetas: por cada acción de `uploads`, `<ActionCard action={a}
  pending={mut.isPending && mut.variables?.id===a.id} onResolve={(id,verb)=>mut.mutate({id,verb})} />`.
- `actionMutation` (confirm/cancel/undo) con `onSuccess`: invalidar
  `["finance","uploads"]`, `["finance","list"]`, `["finance","summary"]`,
  `["finance","cycles"]` (el movimiento confirmado aparece en la lista normal).
- Botón **«Confirmar todos»** sobre las `proposed`: deshabilitado si no hay;
  recorre `Promise.all(proposed.map(a => confirmAction(a.id)))` y luego invalida
  las queries. (Loop secuencial si se prefiere; `Promise.all` está bien para ≤50.)

- [ ] **Step 4: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/lib/ai.ts web/src/lib/ai.test.ts web/src/routes/finanzas.tsx web/src/routes/finanzas.test.tsx
git commit -m "feat(web): zona de subida de comprobantes en Finanzas con tarjetas extraídas"
```

---

### Task 8: Cierre — review, merge, deploy, smoke de producción

- [ ] **Step 1:** Suites completas (backend `-p 1 ./...` + frontend + build) y
  smoke local de acciones (`/tmp/smoke_actions.sh`, sigue verde — el chat no cambió).
- [ ] **Step 2:** Smoke local del import: con docker reconstruido, `curl -F
  file=@api/internal/ai/testdata/sample.csv http://localhost:8088/api/v1/ai/import -H "Authorization: Bearer <tok>"` → 200 con `created`; confirmar una y verla en `/finances/transactions`. (Requiere clave Groq real; el CSV usa el modelo de texto.)
- [ ] **Step 3:** Review final holística (subagente), nits.
- [ ] **Step 4:** Merge `--no-ff` a `main` + push (auto-deploy Coolify; si no, Deploy manual). **Recordar fijar `GROQ_VISION_MODEL` en Coolify** si el default no es el modelo de visión vigente.
- [ ] **Step 5:** Smoke de producción: subir `sample.csv` vía `curl -F` → tarjetas → confirmar todos → `GET /finances/transactions` muestra los movimientos en el ciclo correcto (fecha del CSV).
- [ ] **Step 6:** Bitácora en `docs/superpowers/sesiones/` y push.

---

## Notas para el ejecutor

- Los nombres/params generados por sqlc mandan (`MessageID *uuid.UUID`, `Source string`, `CreateUploadActionParams`). Ajustar el código a lo generado.
- **Riesgo del modelo de visión:** si Groq rechaza el `GROQ_VISION_MODEL` default, las imágenes darán 503 pero CSV/PDF-texto funcionan (usan el modelo de texto). El default es operator-configurable; documentarlo.
- `ledongthuc/pdf` puede entrar en panic con PDFs raros: el `recover` en `pdfText` lo convierte en error (→ 422). El `testdata/sample.txt.pdf` debe ser un PDF de texto REAL legible por la lib; si el PDF mínimo del Step no se lee, generar uno válido y ajustar hasta que `TestPdfTextFromSample` pase.
- Si `parseActionPayload("movimiento")` con `DisallowUnknownFields` descarta movimientos válidos por campos extra del modelo, agregar `parseMovimientoLenient` (mismas reglas, sin DisallowUnknownFields) y usarla SOLO en el extractor.
- Las 7 acciones del chat y el deshacer son INVARIANTES: si un test previo falla, es regresión.
- El frontend no se rompe entre tareas (el contrato de acciones no cambia; solo se agregan endpoints y se extrae un componente).
