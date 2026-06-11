package finance

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createReq struct {
	Type       string `json:"type" validate:"required,oneof=income expense transfer"`
	Amount     int64  `json:"amount" validate:"required,min=1"`
	OccurredOn string `json:"occurred_on" validate:"required"`
	Category   string `json:"category"`
	Remark     string `json:"remark"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/transactions", handleCreate(svc))
	r.Get("/transactions", handleList(svc))
	r.Delete("/transactions/{id}", handleDelete(svc))
	r.Get("/summary", handleSummary(svc))
	r.Get("/cycles", handleCycles(svc))
	return r
}

// nowFrom toma la fecha local del cliente (?today=YYYY-MM-DD) como "hoy"; si no
// viene o es inválida, usa la hora del servidor en UTC.
func nowFrom(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	return time.Now().UTC()
}

// cycleFrom resuelve el ciclo objetivo: el parámetro ?cycle=YYYY-MM o, si falta,
// el ciclo actual derivado de "hoy". Devuelve false si el formato es inválido.
func cycleFrom(r *http.Request, now time.Time) (time.Time, bool) {
	if c := r.URL.Query().Get("cycle"); c != "" {
		parsed, err := ParseCycle(c)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	}
	return Cycle(now), true
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
		occurred, err := time.Parse(dateLayout, req.OccurredOn)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		tx, err := svc.Create(r.Context(), userID, Input{
			Type: req.Type, Amount: req.Amount, OccurredOn: occurred, Category: req.Category, Remark: req.Remark,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, tx)
	}
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		cycle, ok := cycleFrom(r, nowFrom(r))
		if !ok {
			httpx.WriteErr(w, http.StatusBadRequest, "el ciclo no tiene un formato válido (YYYY-MM)")
			return
		}
		list, err := svc.ListByCycle(r.Context(), userID, cycle)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
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

func handleSummary(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		now := nowFrom(r)
		cycle, ok := cycleFrom(r, now)
		if !ok {
			httpx.WriteErr(w, http.StatusBadRequest, "el ciclo no tiene un formato válido (YYYY-MM)")
			return
		}
		sum, err := svc.Summary(r.Context(), userID, cycle, now)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, sum)
	}
}

func handleCycles(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		cycles, err := svc.Cycles(r.Context(), userID, nowFrom(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, cycles)
	}
}
