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
