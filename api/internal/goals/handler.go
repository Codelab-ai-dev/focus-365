package goals

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createReq struct {
	Title     string  `json:"title" validate:"required"`
	Dimension string  `json:"dimension" validate:"required,oneof=checkin finanzas entrenamiento mente general"`
	Deadline  *string `json:"deadline"`
}

type patchReq struct {
	Title     *string         `json:"title"`
	Dimension *string         `json:"dimension" validate:"omitempty,oneof=checkin finanzas entrenamiento mente general"`
	Status    *string         `json:"status" validate:"omitempty,oneof=active done paused"`
	Progress  *int32          `json:"progress" validate:"omitempty,min=0,max=100"`
	Deadline  json.RawMessage `json:"deadline"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/", handleList(svc))
	r.Post("/", handleCreate(svc))
	r.Patch("/{id}", handlePatch(svc))
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
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}
		if status != "active" && status != "done" && status != "paused" {
			httpx.WriteErr(w, http.StatusBadRequest, "estado inválido")
			return
		}
		list, err := svc.List(r.Context(), userID, status, parseTodayParam(r))
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
		var req createReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		deadline, ok := parseDeadline(w, req.Deadline)
		if !ok {
			return
		}
		out, err := svc.Create(r.Context(), userID, GoalInput{
			Title: req.Title, Dimension: req.Dimension, Deadline: deadline,
		}, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

func handlePatch(svc *Service) http.HandlerFunc {
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
		var req patchReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		patch := GoalPatch{
			Title:     req.Title,
			Dimension: req.Dimension,
			Status:    req.Status,
			Progress:  req.Progress,
		}
		// deadline de tres estados: ausente = no tocar; null = limpiar; fecha = fijar.
		if req.Deadline != nil {
			patch.SetDeadline = true
			if string(req.Deadline) != "null" {
				var s string
				if err := json.Unmarshal(req.Deadline, &s); err != nil {
					httpx.WriteErr(w, http.StatusBadRequest, "la fecha límite no tiene un formato válido (YYYY-MM-DD)")
					return
				}
				d, err := time.Parse(dateLayout, s)
				if err != nil {
					httpx.WriteErr(w, http.StatusBadRequest, "la fecha límite no tiene un formato válido (YYYY-MM-DD)")
					return
				}
				patch.Deadline = &d
			}
		}
		out, err := svc.Patch(r.Context(), userID, id, patch, parseTodayParam(r))
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

// parseDeadline convierte el string opcional del create a *time.Time.
// nil (ausente) → sin deadline. Formato inválido → 400 y ok=false.
func parseDeadline(w http.ResponseWriter, s *string) (*time.Time, bool) {
	if s == nil || *s == "" {
		return nil, true
	}
	d, err := time.Parse(dateLayout, *s)
	if err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "la fecha límite no tiene un formato válido (YYYY-MM-DD)")
		return nil, false
	}
	return &d, true
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
