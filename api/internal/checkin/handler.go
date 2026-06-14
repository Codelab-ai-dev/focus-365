package checkin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	defaultLimit = 30
	maxLimit     = 100
)

type upsertReq struct {
	Date        string   `json:"date" validate:"required"`
	Mood        int      `json:"mood" validate:"required,min=1,max=10"`
	Energy      int      `json:"energy" validate:"required,min=1,max=10"`
	Espiritual  string   `json:"espiritual"`
	Emocional   string   `json:"emocional"`
	Fisica      string   `json:"fisica"`
	Financiera  string   `json:"financiera"`
	Win         string   `json:"win"`
	Avoided     string   `json:"avoided"`
	Commitments []string `json:"commitments"`
}

// commitmentWriter es lo que el check-in usa para guardar los compromisos de
// mañana (lo implementa commitments.Service).
type commitmentWriter interface {
	ReplaceForDate(ctx context.Context, userID uuid.UUID, target time.Time, texts []string) error
}

func Routes(svc *Service, commits commitmentWriter) http.Handler {
	r := chi.NewRouter()
	r.Post("/", handleUpsert(svc, commits))
	r.Get("/today", handleToday(svc))
	r.Get("/", handleList(svc))
	return r
}

func handleUpsert(svc *Service, commits commitmentWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req upsertReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		date, err := time.Parse(dateLayout, req.Date)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		ci, err := svc.Upsert(r.Context(), userID, Input{
			Date: date, Mood: req.Mood, Energy: req.Energy,
			Espiritual: req.Espiritual, Emocional: req.Emocional,
			Fisica: req.Fisica, Financiera: req.Financiera,
			Win: req.Win, Avoided: req.Avoided,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// Los compromisos del body son para MAÑANA (target = fecha del check-in + 1).
		// El upsert del check-in y la escritura de compromisos no son una sola
		// transacción: si esto falla tras el upsert, el check-in queda guardado sin
		// los compromisos y el usuario recibe 500. Reintentar es seguro y se
		// auto-sana (upsert idempotente + ReplaceForDate es delete-then-insert).
		if err := commits.ReplaceForDate(r.Context(), userID, date.AddDate(0, 0, 1), req.Commitments); err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, ci)
	}
}

func handleToday(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			httpx.WriteErr(w, http.StatusBadRequest, "Falta la fecha")
			return
		}
		date, err := time.Parse(dateLayout, dateStr)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		ci, err := svc.Today(r.Context(), userID, date)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// ci puede ser nil → se serializa como null (200).
		httpx.WriteJSON(w, http.StatusOK, ci)
	}
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		limit := defaultLimit
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		list, err := svc.List(r.Context(), userID, limit)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}
