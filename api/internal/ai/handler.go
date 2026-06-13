package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const dateLayout = "2006-01-02"

// maxChatChars limita el largo del mensaje del chat en caracteres (runes), no en
// bytes, para que el español con acentos no se rechace antes de tiempo.
const maxChatChars = 2000

// Routes monta los endpoints del asistente (bajo RequireAuth en server.go):
// el insight proactivo y el chat on-demand.
func Routes(svc *Service, chat *ChatService, imp *ImportService) http.Handler {
	r := chi.NewRouter()
	r.Get("/insight", handleInsight(svc))
	r.Get("/messages", handleMessages(chat))
	r.Post("/chat", handleChat(chat))
	r.Post("/chat/stream", handleChatStream(chat))
	r.Post("/actions/{id}/confirm", handleActionConfirm(chat))
	r.Post("/actions/{id}/cancel", handleActionCancel(chat))
	r.Post("/actions/{id}/undo", handleActionUndo(chat))
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
		// Si multipart derramó a un temp file en disco, limpiarlo al terminar.
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()
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
		mimeType := hdr.Header.Get("Content-Type")
		res, err := imp.Import(r.Context(), userID, data, mimeType, hdr.Filename)
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

// chatRequestBody es el body de POST /ai/chat. El largo máximo se valida en el
// handler por caracteres (ver maxChatChars), no con el tag `max` que cuenta bytes.
type chatRequestBody struct {
	Message string `json:"message" validate:"required"`
}

// chatReplyResponse envuelve la respuesta del asistente.
type chatReplyResponse struct {
	Reply Message `json:"reply"`
}

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

// actionResponse envuelve la acción actualizada tras confirm/cancel.
type actionResponse struct {
	Action ActionView `json:"action"`
}

// resolveAction maneja lo común de confirm/cancel: auth, parseo del id y la
// traducción de errores del servicio a HTTP.
func resolveAction(w http.ResponseWriter, r *http.Request,
	do func(ctx context.Context, userID, id uuid.UUID) (*ActionView, error)) {
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
	action, err := do(r.Context(), userID, id)
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
	httpx.WriteJSON(w, http.StatusOK, actionResponse{Action: *action})
}

func handleActionConfirm(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolveAction(w, r, func(ctx context.Context, userID, id uuid.UUID) (*ActionView, error) {
			return chat.ConfirmAction(ctx, userID, id, parseTodayParam(r))
		})
	}
}

func handleActionCancel(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolveAction(w, r, func(ctx context.Context, userID, id uuid.UUID) (*ActionView, error) {
			return chat.CancelAction(ctx, userID, id)
		})
	}
}

func handleActionUndo(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolveAction(w, r, func(ctx context.Context, userID, id uuid.UUID) (*ActionView, error) {
			return chat.UndoAction(ctx, userID, id)
		})
	}
}
