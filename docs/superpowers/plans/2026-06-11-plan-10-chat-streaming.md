# Plan 10 — Streaming de tokens en el chat IA — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** La respuesta del asistente en `/asistente` aparece palabra por palabra vía SSE, manteniendo la semántica «persistir solo ante éxito».

**Architecture:** Camino de streaming paralelo al existente en `api/internal/ai` (cliente `ChatStream` → servicio `SendStream` → handler SSE con `http.Flusher`), endpoint nuevo `POST /ai/chat/stream`. El frontend agrega `sendMessageStream` (fetch + ReadableStream) y estado de streaming en la página. Nada del camino bloqueante cambia.

**Tech Stack:** Go 1.x + chi + sqlc (api), React + TanStack Query/Router + Vitest (web), Groq API OpenAI-compatible con `"stream": true`.

**Spec:** `docs/superpowers/specs/2026-06-11-plan-10-chat-streaming-design.md`

**Entorno (de bitácoras previas):**
- Comandos `go` desde `api/`, con `GOTOOLCHAIN=local` y `PATH` incluyendo `/usr/local/go/bin`.
- Tests backend necesitan `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"` (db de docker arriba).
- Comandos docker: `dangerouslyDisableSandbox: true` + `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"` en la misma línea.
- En scripts zsh usar `USERID`, no `UID` (readonly).

---

### Task 1: `GroqClient.ChatStream` (cliente con streaming SSE)

**Files:**
- Modify: `api/internal/ai/groq.go`
- Test: `api/internal/ai/groq_test.go`

- [ ] **Step 1: Escribir los tests que fallan**

Agregar al final de `api/internal/ai/groq_test.go`:

```go
// sseChunk escribe un evento data: de Groq y hace flush.
func sseChunk(w http.ResponseWriter, payload string) {
	_, _ = w.Write([]byte("data: " + payload + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func TestGroqChatStreamOK(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"Vas "}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"content":"bien."}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
	var deltas []string
	got, err := c.ChatStream(context.Background(), "sys", []ChatMsg{
		{Role: "user", Content: "¿cómo voy?"},
	}, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if got != "Vas bien." {
		t.Errorf("content = %q, want %q", got, "Vas bien.")
	}
	if len(deltas) != 2 || deltas[0] != "Vas " || deltas[1] != "bien." {
		t.Errorf("deltas = %v", deltas)
	}
	body := string(gotBody)
	for _, want := range []string{`"stream":true`, `"role":"system"`, `"content":"sys"`, `"content":"¿cómo voy?"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqChatStreamCutMidwayFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"Vas "}}]}`)
		// Cierra sin [DONE]: simula corte a medias.
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	var deltas []string
	_, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}},
		func(d string) { deltas = append(deltas, d) })
	if err == nil {
		t.Fatal("esperaba error al cortarse el stream sin [DONE]")
	}
}

func TestGroqChatStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}},
		func(string) {}); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}
```

- [ ] **Step 2: Verificar que fallan**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -run TestGroqChatStream -v
```
Esperado: FAIL de compilación — `c.ChatStream undefined`.

- [ ] **Step 3: Implementar `ChatStream`**

En `api/internal/ai/groq.go`:

1. Agregar imports `bufio` y `strings` al bloque de imports.
2. Agregar campo y constructor:

```go
// GroqClient habla con el endpoint OpenAI-compatible de Groq.
type GroqClient struct {
	baseURL    string
	apiKey     string
	model      string
	http       *http.Client
	streamHTTP *http.Client
}
```

y en `newGroqClient`:

```go
func newGroqClient(baseURL, apiKey, model string) *GroqClient {
	return &GroqClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 10 * time.Second},
		// El timeout del cliente cubre la lectura completa del body; un stream
		// necesita más holgura que una respuesta bloqueante.
		streamHTTP: &http.Client{Timeout: 60 * time.Second},
	}
}
```

3. Agregar `Stream` al request:

```go
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream,omitempty"`
}
```

4. Agregar al final del archivo:

```go
// chatStreamChunk es un chunk SSE de Groq en modo stream (estilo OpenAI).
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ChatStream envía el chat con "stream": true y re-emite cada delta vía
// onDelta. Devuelve el contenido completo acumulado. Si el stream se corta
// antes del data: [DONE], devuelve error (el caller no debe persistir).
func (c *GroqClient) ChatStream(ctx context.Context, system string, history []ChatMsg, onDelta func(string)) (string, error) {
	msgs := make([]chatMessage, 0, len(history)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: system})
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}

	reqBody, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.7,
		MaxTokens:   400,
		Stream:      true,
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

	res, err := c.streamHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("groq status %d: %s", res.StatusCode, string(body))
	}

	var full strings.Builder
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
			return "", fmt.Errorf("groq chunk inválido: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if delta := chunk.Choices[0].Delta.Content; delta != "" {
			full.WriteString(delta)
			onDelta(delta)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if !sawDone {
		return "", fmt.Errorf("groq stream cortado antes de [DONE]")
	}
	if full.Len() == 0 {
		return "", fmt.Errorf("groq stream sin contenido")
	}
	return full.String(), nil
}
```

- [ ] **Step 4: Verificar que pasan**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -run TestGroq -v
```
Esperado: PASS (los nuevos y los `TestGroq*` previos).

- [ ] **Step 5: Commit**

```bash
git add api/internal/ai/groq.go api/internal/ai/groq_test.go
git commit -m "feat(ai): GroqClient.ChatStream con SSE y cliente HTTP de 60s"
```

---

### Task 2: `ChatService.SendStream` (servicio + wiring)

**Files:**
- Modify: `api/internal/ai/chat.go`
- Modify: `api/internal/ai/chat_test.go`
- Modify: `api/internal/ai/handler_test.go` (fake + `newEnv`)
- Modify: `api/internal/server/server.go` (línea del `ai.NewChatService`)

- [ ] **Step 1: Escribir los tests que fallan**

Agregar a `api/internal/ai/chat_test.go`, después de `fakeChatGroq`:

```go
// ChatStream del fake: emite chatDeltas en orden y devuelve su concatenación,
// o err si está seteado (simula corte a medias tras emitir los deltas).
func (f *fakeChatGroq) ChatStream(ctx context.Context, system string, history []ChatMsg, onDelta func(string)) (string, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	var full string
	for _, d := range f.chatDeltas {
		full += d
		onDelta(d)
	}
	if f.err != nil {
		return "", f.err
	}
	return full, nil
}
```

y el campo nuevo en el struct `fakeChatGroq`:

```go
type fakeChatGroq struct {
	out         string
	err         error
	called      int
	lastSystem  string
	lastHistory []ChatMsg
	chatDeltas  []string
}
```

Tests nuevos al final del archivo:

```go
func TestChatSendStreamEmitsDeltasAndPersists(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"Vas ", "bien."}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, true)
	uid := uuid.New()

	var deltas []string
	msg, err := svc.SendStream(context.Background(), uid, "¿cómo voy?", time.Now(),
		func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if len(deltas) != 2 || deltas[0] != "Vas " || deltas[1] != "bien." {
		t.Errorf("deltas = %v", deltas)
	}
	if msg.Role != "assistant" || msg.Content != "Vas bien." {
		t.Errorf("reply = %+v", msg)
	}
	if len(st.rows) != 2 || st.rows[0].Content != "¿cómo voy?" || st.rows[1].Content != "Vas bien." {
		t.Errorf("persistencia = %+v", st.rows)
	}
}

func TestChatSendStreamFailureDoesNotPersist(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"Vas "}, err: errors.New("stream cortado")}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, true)

	_, err := svc.SendStream(context.Background(), uuid.New(), "hola", time.Now(), func(string) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("corte a medias no debe persistir nada")
	}
}

func TestChatSendStreamNoKeyDegrades(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"no usar"}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, false)

	_, err := svc.SendStream(context.Background(), uuid.New(), "hola", time.Now(), func(string) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if groq.called != 0 {
		t.Error("sin clave no debe llamar a Groq")
	}
}
```

**Además**, actualizar TODAS las llamadas existentes a `NewChatService` en `chat_test.go` (hay 5) agregando el streamer como cuarto parámetro — el mismo fake sirve porque implementa ambas interfaces. Ejemplo: `NewChatService(fakeCtx{out: "{}"}, st, groq, groq, true)`. En `TestChatHistoryMapsRows` el fake está inline: `f := &fakeChatGroq{}` primero y pasar `f, f`.

- [ ] **Step 2: Verificar que fallan**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -run TestChatSendStream -v
```
Esperado: FAIL de compilación — `NewChatService` no acepta 5 args / `SendStream` undefined.

- [ ] **Step 3: Implementar `SendStream`**

En `api/internal/ai/chat.go`:

1. Interfaz nueva, debajo de `chatCompleter`:

```go
// chatStreamer abstrae la llamada de chat en streaming (testeable con fake).
type chatStreamer interface {
	ChatStream(ctx context.Context, system string, history []ChatMsg, onDelta func(string)) (string, error)
}
```

2. Campo y constructor:

```go
// ChatService orquesta la conversación: contexto + historial + Groq + persistencia.
type ChatService struct {
	ctxb     contextBuilder
	store    messageStore
	groq     chatCompleter
	streamer chatStreamer
	hasKey   bool
}

// NewChatService inyecta el constructor de contexto, el store de mensajes, los
// clientes de chat bloqueante y streaming (GroqClient implementa ambos) y si
// hay clave configurada.
func NewChatService(ctxb contextBuilder, q messageStore, c chatCompleter, s chatStreamer, hasKey bool) *ChatService {
	return &ChatService{ctxb: ctxb, store: q, groq: c, streamer: s, hasKey: hasKey}
}
```

3. Método nuevo, después de `Send`:

```go
// SendStream es la variante streaming de Send: re-emite cada delta vía onDelta
// y, solo si el stream de Groq terminó sin error, persiste el par completo de
// forma atómica. Corte a medias → ErrUnavailable y nada persistido.
func (s *ChatService) SendStream(ctx context.Context, userID uuid.UUID, text string, today time.Time, onDelta func(string)) (*Message, error) {
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

	reply, err := s.streamer.ChatStream(ctx, buildChatSystemPrompt(contextJSON), history, onDelta)
	if err != nil {
		return nil, ErrUnavailable
	}

	assistant, err := s.store.CreatePair(ctx, userID, text, reply)
	if err != nil {
		return nil, err
	}
	v := Message{Role: assistant.Role, Content: assistant.Content, CreatedAt: assistant.CreatedAt}
	return &v, nil
}
```

4. En `api/internal/ai/handler_test.go`, dar al `fakeCompleter` la interfaz de streaming (campos nuevos + método):

```go
type fakeCompleter struct {
	out    string
	err    error
	called int

	chatOut    string
	chatErr    error
	chatCalled int

	chatDeltas    []string
	chatStreamErr error
}

func (f *fakeCompleter) ChatStream(ctx context.Context, system string, history []ai.ChatMsg, onDelta func(string)) (string, error) {
	f.chatCalled++
	var full string
	for _, d := range f.chatDeltas {
		full += d
		onDelta(d)
	}
	if f.chatStreamErr != nil {
		return "", f.chatStreamErr
	}
	return full, nil
}
```

y en `newEnv` actualizar la construcción:

```go
chatSvc := ai.NewChatService(chatCtx, chatStore, comp, comp, hasKey)
```

5. En `api/internal/server/server.go` actualizar la línea existente:

```go
chatSvc := ai.NewChatService(chatCtx, chatStore, groq, groq, d.GroqAPIKey != "")
```

- [ ] **Step 4: Verificar que pasa todo el paquete**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
```
Esperado: build OK y PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/ai/chat.go api/internal/ai/chat_test.go api/internal/ai/handler_test.go api/internal/server/server.go
git commit -m "feat(ai): ChatService.SendStream con persistencia solo-ante-éxito"
```

---

### Task 3: Endpoint SSE `POST /ai/chat/stream`

**Files:**
- Modify: `api/internal/ai/handler.go`
- Test: `api/internal/ai/chat_handler_test.go`

- [ ] **Step 1: Escribir los tests que fallan**

Agregar a `api/internal/ai/chat_handler_test.go`:

```go
func postChatStream(t *testing.T, h http.Handler, tok, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ai/chat/stream?today="+today, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestChatStreamHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatDeltas: []string{"Vas ", "verde."}}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "stream@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"¿cómo voy?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if ab := rec.Header().Get("X-Accel-Buffering"); ab != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no (nginx)", ab)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: delta", `{"text":"Vas "}`, `{"text":"verde."}`,
		"event: done", `"content":"Vas verde."`, `"role":"assistant"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body SSE no contiene %q:\n%s", want, body)
		}
	}

	rec2, body2 := getMessages(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("messages code = %d", rec2.Code)
	}
	msgs, _ := body2["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2 (par persistido)", len(msgs))
	}
}

func TestChatStreamGroqFailureMidwayEmitsErrorEvent(t *testing.T) {
	comp := &fakeCompleter{chatDeltas: []string{"Vas "}, chatStreamErr: errors.New("stream cortado")}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "streamcut@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"hola"}`)
	// Los headers ya salieron con el primer delta: el código es 200.
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: delta") || !strings.Contains(body, "event: error") {
		t.Errorf("esperaba delta y error en el body:\n%s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Errorf("no debe haber done tras un corte:\n%s", body)
	}

	_, body2 := getMessages(t, e.h, tok)
	msgs, _ := body2["messages"].([]any)
	if len(msgs) != 0 {
		t.Errorf("corte a medias no debe persistir, got %d mensajes", len(msgs))
	}
}

func TestChatStreamFailureBeforeFirstDeltaIs503(t *testing.T) {
	comp := &fakeCompleter{chatStreamErr: errors.New("groq caído")} // cero deltas
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "streamdown@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"hola"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503 (HTTP normal, sin SSE)", rec.Code)
	}
}

func TestChatStreamNoKey503(t *testing.T) {
	e := newEnv(t, false, &fakeCompleter{chatDeltas: []string{"no usar"}})
	_, tok := e.user(t, "streamnokey@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"hola"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestChatStreamValidationAndAuth(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatDeltas: []string{"x"}})
	_, tok := e.user(t, "streamval@b.com")

	if rec := postChatStream(t, e.h, tok, `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("solo espacios code = %d, want 400", rec.Code)
	}
	if rec := postChatStream(t, e.h, "", `{"message":"hola"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}
```

Nota: `errors` ya se importa en `handler_test.go` (mismo paquete `ai_test`); si `chat_handler_test.go` no lo importa, agregarlo.

- [ ] **Step 2: Verificar que fallan**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -run TestChatStream -v
```
Esperado: FAIL — `POST /ai/chat/stream` devuelve 404/405 (la ruta no existe).

- [ ] **Step 3: Implementar el handler**

En `api/internal/ai/handler.go`:

1. Imports nuevos: `encoding/json`, `fmt`, `io`, `github.com/google/uuid`.
2. Ruta en `Routes`:

```go
r.Post("/chat", handleChat(chat))
r.Post("/chat/stream", handleChatStream(chat))
```

3. Extraer la validación compartida (reemplaza el cuerpo duplicado de `handleChat`):

```go
// decodeChatMessage hace la validación compartida de los endpoints de chat:
// auth, decode, no-vacío tras trim y máximo de runes. Si algo falla escribe la
// respuesta de error y devuelve ok=false.
func decodeChatMessage(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
		return uuid.Nil, "", false
	}
	var req chatRequestBody
	if !httpx.DecodeAndValidate(w, r, &req) {
		return uuid.Nil, "", false
	}
	// Rechazamos mensajes vacíos tras trim (el validator `required` deja pasar
	// cadenas de solo espacios).
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		httpx.WriteErr(w, http.StatusBadRequest, "Falta el mensaje")
		return uuid.Nil, "", false
	}
	if utf8.RuneCountInString(req.Message) > maxChatChars {
		httpx.WriteErr(w, http.StatusBadRequest, "El mensaje es demasiado largo")
		return uuid.Nil, "", false
	}
	return userID, req.Message, true
}
```

y `handleChat` queda:

```go
func handleChat(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, msg, ok := decodeChatMessage(w, r)
		if !ok {
			return
		}
		reply, err := chat.Send(r.Context(), userID, msg, parseTodayParam(r))
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
```

4. Handler de streaming y tipos de evento:

```go
type deltaEvent struct {
	Text string `json:"text"`
}

type errorEvent struct {
	Error string `json:"error"`
}

type doneEvent struct {
	Reply Message `json:"reply"`
}

// writeSSEEvent serializa data y lo escribe como evento SSE, con flush
// inmediato para que el navegador lo reciba sin esperar al cierre.
func writeSSEEvent(w io.Writer, flusher http.Flusher, event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	flusher.Flush()
}

// handleChatStream responde el chat por SSE. Los errores previos al primer
// delta son respuestas HTTP normales (400/401/503); una vez iniciado el
// stream, los fallos se emiten como `event: error` (y nada se persistió).
func handleChatStream(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, msg, ok := decodeChatMessage(w, r)
		if !ok {
			return
		}
		flusher, okF := w.(http.Flusher)
		if !okF {
			httpx.WriteErr(w, http.StatusInternalServerError, "streaming no soportado")
			return
		}

		// Los headers SSE se escriben recién con el primer delta, para poder
		// responder 503/500 HTTP normal si Groq falla antes de emitir nada.
		started := false
		startSSE := func() {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			// nginx respeta este header por respuesta: sin él, bufferea el proxy
			// y los deltas llegarían todos juntos.
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}

		reply, err := chat.SendStream(r.Context(), userID, msg, parseTodayParam(r), func(delta string) {
			if !started {
				startSSE()
			}
			writeSSEEvent(w, flusher, "delta", deltaEvent{Text: delta})
		})
		if err != nil {
			if !started {
				if errors.Is(err, ErrUnavailable) {
					httpx.WriteErr(w, http.StatusServiceUnavailable, "asistente no disponible por ahora")
					return
				}
				httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
				return
			}
			msgTxt := "error interno"
			if errors.Is(err, ErrUnavailable) {
				msgTxt = "asistente no disponible por ahora"
			}
			writeSSEEvent(w, flusher, "error", errorEvent{Error: msgTxt})
			return
		}
		if !started {
			startSSE()
		}
		writeSSEEvent(w, flusher, "done", doneEvent{Reply: *reply})
	}
}
```

- [ ] **Step 4: Verificar que pasa todo el backend**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Esperado: PASS completo (incluidos los tests previos de `/ai/chat`, que no cambian de contrato).

- [ ] **Step 5: Commit**

```bash
git add api/internal/ai/handler.go api/internal/ai/chat_handler_test.go
git commit -m "feat(ai): endpoint SSE POST /ai/chat/stream con eventos delta/done/error"
```

---

### Task 4: Lib frontend `sendMessageStream`

**Files:**
- Modify: `web/src/lib/ai.ts`
- Test: `web/src/lib/ai.test.ts`

- [ ] **Step 1: Escribir los tests que fallan**

Agregar a `web/src/lib/ai.test.ts` (el import de la línea 2 gana `sendMessageStream`, y agregar `import { ApiError } from "./api";`):

```ts
function sseResponse(chunks: string[], status = 200) {
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
  return Promise.resolve(
    new Response(stream, { status, headers: { "Content-Type": "text/event-stream" } })
  );
}

describe("sendMessageStream", () => {
  afterEach(() => vi.restoreAllMocks());

  const doneEvent =
    'event: done\ndata: {"reply":{"role":"assistant","content":"Vas bien.","created_at":"2026-06-11T10:00:02Z"}}\n\n';

  it("acumula deltas vía onDelta y resuelve con el reply del done", async () => {
    const fetchMock = vi.fn(() =>
      sseResponse([
        'event: delta\ndata: {"text":"Vas "}\n\n',
        'event: delta\ndata: {"text":"bien."}\n\n',
        doneEvent,
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const deltas: string[] = [];
    const reply = await sendMessageStream("¿cómo voy?", (d) => deltas.push(d));

    expect(deltas).toEqual(["Vas ", "bien."]);
    expect(reply.content).toBe("Vas bien.");
    const [url, opts] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/v1/ai/chat/stream");
    expect(opts.method).toBe("POST");
    expect(JSON.parse(opts.body as string)).toEqual({ message: "¿cómo voy?" });
  });

  it("reensambla un evento partido entre dos chunks", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        sseResponse(['event: delta\ndata: {"te', 'xt":"Hola"}\n\n', doneEvent])
      )
    );

    const deltas: string[] = [];
    await sendMessageStream("hola", (d) => deltas.push(d));
    expect(deltas).toEqual(["Hola"]);
  });

  it("rechaza con el mensaje del evento error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        sseResponse([
          'event: delta\ndata: {"text":"Vas "}\n\n',
          'event: error\ndata: {"error":"asistente no disponible por ahora"}\n\n',
        ])
      )
    );

    await expect(sendMessageStream("hola", () => {})).rejects.toThrowError(
      "asistente no disponible por ahora"
    );
  });

  it("rechaza con ApiError en HTTP no-ok (sin stream)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "asistente no disponible por ahora" }), {
            status: 503,
          })
        )
      )
    );

    const p = sendMessageStream("hola", () => {});
    await expect(p).rejects.toBeInstanceOf(ApiError);
    await expect(sendMessageStream("hola", () => {})).rejects.toThrowError(/no disponible/);
  });

  it("rechaza si el stream se cierra sin done ni error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => sseResponse(['event: delta\ndata: {"text":"Vas "}\n\n']))
    );

    await expect(sendMessageStream("hola", () => {})).rejects.toThrowError(/cortó/);
  });
});
```

- [ ] **Step 2: Verificar que fallan**

```bash
cd web && npx vitest run src/lib/ai.test.ts
```
Esperado: FAIL — `sendMessageStream` no existe (error de import).

- [ ] **Step 3: Implementar `sendMessageStream`**

En `web/src/lib/ai.ts`, cambiar la primera línea a:

```ts
import { apiFetch, getAccessToken, ApiError } from "./api";
```

y agregar al final:

```ts
// sendMessageStream envía el mensaje al endpoint SSE y entrega los deltas vía
// onDelta a medida que llegan. Resuelve con el reply persistido (evento done)
// o rechaza con ApiError (HTTP no-ok, evento error, o stream cortado) — en
// cuyo caso nada quedó persistido y el caller debe descartar el parcial.
export async function sendMessageStream(
  message: string,
  onDelta: (text: string) => void
): Promise<Message> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch("/api/v1/ai/chat/stream", {
    method: "POST",
    headers,
    body: JSON.stringify({ message }),
    credentials: "include",
  });

  if (!res.ok) {
    let msg = `Error ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* respuesta sin JSON */
    }
    throw new ApiError(msg, res.status);
  }
  if (!res.body) throw new ApiError("streaming no soportado", 500);

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let reply: Message | null = null;

  const handleEvent = (raw: string) => {
    let event = "";
    let data = "";
    for (const line of raw.split("\n")) {
      if (line.startsWith("event: ")) event = line.slice(7).trim();
      else if (line.startsWith("data: ")) data += line.slice(6);
    }
    if (!event || !data) return;
    if (event === "delta") {
      onDelta((JSON.parse(data) as { text: string }).text);
    } else if (event === "done") {
      reply = (JSON.parse(data) as { reply: Message }).reply;
    } else if (event === "error") {
      throw new ApiError((JSON.parse(data) as { error: string }).error, 503);
    }
  };

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const raw = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      handleEvent(raw);
    }
  }

  if (!reply) throw new ApiError("la respuesta se cortó, intenta de nuevo", 502);
  return reply;
}
```

- [ ] **Step 4: Verificar que pasan**

```bash
cd web && npx vitest run src/lib/ai.test.ts
```
Esperado: PASS (los nuevos y los previos del archivo).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/ai.ts web/src/lib/ai.test.ts
git commit -m "feat(web): sendMessageStream lee el SSE del chat con deltas"
```

---

### Task 5: Página `/asistente` con burbuja en vivo

**Files:**
- Modify: `web/src/routes/asistente.tsx`
- Test: `web/src/routes/asistente.test.tsx`

- [ ] **Step 1: Escribir/actualizar los tests que fallan**

En `web/src/routes/asistente.test.tsx`:

1. Agregar helper arriba de `describe` (debajo de `renderPage`):

```ts
function sseBody(chunks: string[]) {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
}
```

2. Reemplazar el test `"al enviar dispara un POST con el mensaje"` por:

```ts
it("al enviar streamea la respuesta y muestra la burbuja creciendo", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (url === "/api/v1/ai/chat/stream" && opts?.method === "POST") {
      return Promise.resolve(
        new Response(
          sseBody([
            'event: delta\ndata: {"text":"Vas "}\n\n',
            'event: delta\ndata: {"text":"verde."}\n\n',
            'event: done\ndata: {"reply":{"role":"assistant","content":"Vas verde.","created_at":"2026-06-11T10:00:02Z"}}\n\n',
          ]),
          { status: 200, headers: { "Content-Type": "text/event-stream" } }
        )
      );
    }
    return Promise.resolve(new Response(JSON.stringify({ messages: [] }), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);

  renderPage();
  const input = await screen.findByLabelText("Mensaje");
  await userEvent.type(input, "¿cómo voy?");
  await userEvent.click(screen.getByRole("button", { name: "Enviar" }));

  // El POST fue al endpoint de streaming.
  await waitFor(() => {
    const posted = fetchMock.mock.calls.some(
      ([url, opts]) => url === "/api/v1/ai/chat/stream" && (opts as RequestInit)?.method === "POST"
    );
    expect(posted).toBe(true);
  });
  // La respuesta completa queda visible (burbuja streameada).
  expect(await screen.findByText("Vas verde.")).toBeInTheDocument();
});
```

3. El test `"muestra error inline sin romper la página cuando el POST falla"` sigue válido pero el mock debe responder al endpoint nuevo: cambiar la condición `if (opts?.method === "POST")` (queda igual, matchea cualquier POST) — **no requiere cambio**; solo verificar que sigue pasando.

4. Agregar test del descarte del parcial:

```ts
it("descarta el parcial y muestra error si el stream se corta", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(
        new Response(
          sseBody([
            'event: delta\ndata: {"text":"Vas "}\n\n',
            'event: error\ndata: {"error":"asistente no disponible por ahora"}\n\n',
          ]),
          { status: 200, headers: { "Content-Type": "text/event-stream" } }
        )
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
  // El parcial "Vas " no queda como burbuja fantasma.
  expect(screen.queryByText(/^Vas /)).toBeNull();
  // El input conserva el texto para reintentar.
  expect(input.value).toBe("hola");
});
```

- [ ] **Step 2: Verificar que fallan**

```bash
cd web && npx vitest run src/routes/asistente.test.tsx
```
Esperado: FAIL — la página sigue posteando a `/api/v1/ai/chat` (no streaming).

- [ ] **Step 3: Implementar el streaming en la página**

En `web/src/routes/asistente.tsx`:

1. Cambiar el import de la lib:

```ts
import { getMessages, sendMessageStream, type Message } from "@/lib/ai";
```

2. Agregar estado de streaming junto a los estados existentes:

```ts
const [streaming, setStreaming] = useState<{ question: string; partial: string } | null>(null);
```

3. Reemplazar la mutación:

```ts
const mutation = useMutation({
  mutationFn: (message: string) => {
    setStreaming({ question: message, partial: "" });
    return sendMessageStream(message, (delta) =>
      setStreaming((s) => (s ? { ...s, partial: s.partial + delta } : s))
    );
  },
  onSuccess: async () => {
    setError(null);
    setText("");
    // Esperar el refetch antes de quitar las burbujas optimistas evita el
    // parpadeo entre el parcial y el par persistido.
    await qc.invalidateQueries({ queryKey: ["ai-messages"] });
    setStreaming(null);
  },
  onError: (err) => {
    // Nada se persistió: se descarta el parcial y el input conserva el texto.
    setStreaming(null);
    setError(err instanceof Error ? err.message : "No se pudo enviar");
  },
});
```

4. Reemplazar el bloque `{mutation.isPending && (...Pensando…...)}` por:

```tsx
{streaming && (
  <>
    <div className="self-end rounded-lg bg-amber-brand/20 px-3 py-2 text-sm text-sand-100">
      {streaming.question}
    </div>
    {streaming.partial === "" ? (
      <div className="self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-400">
        Pensando…
      </div>
    ) : (
      <div className="self-start rounded-lg border border-ink-700 bg-ink-900 px-3 py-2 text-sm text-sand-100">
        {streaming.partial}
      </div>
    )}
  </>
)}
```

- [ ] **Step 4: Verificar suite completa + build**

```bash
cd web && npx vitest run && npm run build
```
Esperado: PASS completo (incluido `index.test.tsx` y demás, que mockean `@/lib/auth`) y build estricto sin errores.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/asistente.tsx web/src/routes/asistente.test.tsx
git commit -m "feat(web): burbuja del asistente en vivo con sendMessageStream"
```

---

### Task 6: Verificación E2E en docker (a través del proxy nginx)

**Files:**
- Create: `/tmp/smoke_stream.sh` (efímero, no se commitea)

- [ ] **Step 1: Reconstruir contenedores**

```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"; cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
```
(Requiere `dangerouslyDisableSandbox: true`.)

- [ ] **Step 2: Escribir y correr el smoke**

Crear `/tmp/smoke_stream.sh`:

```bash
#!/bin/zsh
set -u
# A través del proxy nginx del contenedor web (5174), no directo al api:
# valida que el buffering de nginx no rompa el SSE.
BASE=http://localhost:5174/api/v1
PASS=0; FAIL=0
check() { if [ "$1" = "$2" ]; then echo "OK   $3"; PASS=$((PASS+1)); else echo "FAIL $3 (got $1, want $2)"; FAIL=$((FAIL+1)); fi }

EMAIL="smoke-stream-$RANDOM@focus.com"
REG=$(curl -s -X POST $BASE/auth/register -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"p4ssword\",\"name\":\"Smoke\"}")
TOKEN=$(echo "$REG" | python3 -c "import json,sys;print(json.load(sys.stdin)['access_token'])" 2>/dev/null)
[ -n "$TOKEN" ] && check yes yes "registro y token" || check no yes "registro y token"

# Stream: curl -N sin buffering; capturamos el body SSE completo.
SSE=$(curl -s -N -X POST $BASE/ai/chat/stream \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"message":"En una frase: ¿cómo voy hoy?"}')
echo "$SSE" | grep -q "event: delta" && check yes yes "llegaron eventos delta" || check no yes "llegaron eventos delta"
echo "$SSE" | grep -q "event: done" && check yes yes "llegó el evento done" || check no yes "llegó el evento done"
echo "$SSE" | grep -q "event: error" && check yes no "sin evento error" || check no no "sin evento error"

# El par quedó persistido.
COUNT=$(curl -s $BASE/ai/messages -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import json,sys;print(len(json.load(sys.stdin)['messages']))" 2>/dev/null)
check "$COUNT" "2" "par persistido en el historial"

# Validación y auth del endpoint nuevo.
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE/ai/chat/stream \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"message":"   "}')
check "$CODE" "400" "mensaje vacío 400"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE/ai/chat/stream \
  -H 'Content-Type: application/json' -d '{"message":"hola"}')
check "$CODE" "401" "sin token 401"

echo "---"; echo "$PASS OK / $FAIL FAIL"
exit $FAIL
```

Correr:

```bash
chmod +x /tmp/smoke_stream.sh && /tmp/smoke_stream.sh
```
Esperado: **7 OK / 0 FAIL** (usa la clave Groq real del `.env`).

- [ ] **Step 3: Verificación final completa**

```bash
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
cd ../web && npx vitest run && npm run build
```
Esperado: todo verde.

- [ ] **Step 4: Commit final (si hubo ajustes) y cierre**

Usar superpowers:finishing-a-development-branch para mergear la rama a `master` (`--no-ff`, convención del repo) y borrar la rama.

---

## Notas para el ejecutor

- **Rama:** crear `plan-10-chat-streaming` desde `master` antes de la Task 1 (rama de feature, NO worktree: docker está atado a la ruta del repo).
- El camino bloqueante (`POST /ai/chat`, `Send`, `Chat`) no cambia de contrato; si algún test previo falla, es una regresión introducida — no "arreglar" el test.
- `httptest.ResponseRecorder` implementa `http.Flusher`, por eso los tests del handler SSE funcionan sin servidor real.
- En vitest/jsdom, `Response` acepta `ReadableStream` como body (fetch de Node 18+).
