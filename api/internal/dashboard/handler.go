package dashboard

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

const dateLayout = "2006-01-02"

// Routes monta el endpoint del dashboard (bajo RequireAuth en server.go).
func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/", handleSnapshot(svc))
	return r
}

func handleSnapshot(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		snap, err := svc.Snapshot(r.Context(), userID, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, snap)
	}
}

// parseTodayParam lee ?today=YYYY-MM-DD (zona del cliente). Si falta o es
// inválido, cae al día UTC del server. Mismo patrón que metas/hábitos.
func parseTodayParam(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
