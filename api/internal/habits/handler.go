package habits

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type habitReq struct {
	Name       string `json:"name" validate:"required"`
	TargetDays *int32 `json:"target_days" validate:"omitempty,min=1"`
}

type checkReq struct {
	Day  string `json:"day"`
	Done *bool  `json:"done" validate:"required"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/", handleList(svc))
	r.Post("/", handleCreate(svc))
	r.Post("/{id}/check", handleCheck(svc))
	r.Post("/{id}/archive", handleArchive(svc))
	r.Delete("/{id}", handleDelete(svc))
	return r
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		archived := r.URL.Query().Get("archived") == "true"
		list, err := svc.List(r.Context(), userID, archived, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleCreate(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req habitReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		out, err := svc.Create(r.Context(), userID, HabitInput{Name: req.Name, TargetDays: req.TargetDays}, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

func handleCheck(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		var req checkReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		today := parseTodayParam(r)
		day := today
		if req.Day != "" {
			parsed, err := time.Parse(dateLayout, req.Day)
			if err != nil {
				httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
				return
			}
			day = parsed
		}
		// Ventana de gracia: solo se acepta marcar hoy o ayer.
		if !sameDay(day, today) && !sameDay(day, today.AddDate(0, 0, -1)) {
			httpx.WriteErr(w, http.StatusBadRequest, "solo podés marcar hoy o ayer")
			return
		}
		out, err := svc.SetCheck(r.Context(), userID, id, day, *req.Done, today)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if out == nil {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

func handleArchive(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		out, err := svc.Archive(r.Context(), userID, id, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if out == nil {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

func handleDelete(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		deleted, err := svc.Delete(r.Context(), userID, id)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if !deleted {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// parseTodayParam lee ?today=YYYY-MM-DD (zona del cliente). Si falta o es
// inválido, cae al día UTC del server.
func parseTodayParam(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// sameDay compara dos fechas por su día (YYYY-MM-DD).
func sameDay(a, b time.Time) bool {
	return a.Format(dateLayout) == b.Format(dateLayout)
}
