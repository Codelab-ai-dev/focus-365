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
