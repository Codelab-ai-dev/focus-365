package commitments

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const dateParam = "2006-01-02"

// Routes monta los endpoints de compromisos (se montan bajo RequireAuth).
func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/due", handleDue(svc))
	r.Get("/pending", handlePending(svc))
	r.Post("/{id}/toggle", handleToggle(svc))
	return r
}

func handleDue(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		date := time.Now().UTC().Truncate(24 * time.Hour)
		if s := r.URL.Query().Get("date"); s != "" {
			if d, err := time.Parse(dateParam, s); err == nil {
				date = d
			}
		}
		due, err := svc.DueOn(r.Context(), userID, date)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"commitments": due})
	}
}

func handlePending(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		pend, err := svc.Pending(r.Context(), userID, today)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"commitments": pend})
	}
}

func handleToggle(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "compromiso no encontrado")
			return
		}
		c, err := svc.Toggle(r.Context(), userID, id)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if c == nil {
			httpx.WriteErr(w, http.StatusNotFound, "compromiso no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"commitment": c})
	}
}
