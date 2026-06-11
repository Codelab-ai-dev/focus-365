package training

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type exerciseReq struct {
	Name string `json:"name" validate:"required"`
}

type setReq struct {
	Exercise    string `json:"exercise" validate:"required"`
	Reps        *int32 `json:"reps" validate:"omitempty,min=0"`
	WeightGrams *int32 `json:"weight_grams" validate:"omitempty,min=0"`
}

type workoutReq struct {
	Date string   `json:"date" validate:"required"`
	Type string   `json:"type"`
	Note string   `json:"note"`
	Sets []setReq `json:"sets" validate:"required,min=1,dive"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/exercises", handleListExercises(svc))
	r.Post("/exercises", handleCreateExercise(svc))
	r.Post("/workouts", handleCreateWorkout(svc))
	r.Get("/workouts", handleListWorkouts(svc))
	r.Get("/workouts/{id}", handleGetWorkout(svc))
	r.Delete("/workouts/{id}", handleDeleteWorkout(svc))
	return r
}

func handleListExercises(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		list, err := svc.ListExercises(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleCreateExercise(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req exerciseReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		ex, err := svc.CreateExercise(r.Context(), userID, req.Name)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, ex)
	}
}

func handleCreateWorkout(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req workoutReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		date, err := time.Parse(dateLayout, req.Date)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		sets := make([]SetInput, 0, len(req.Sets))
		for _, s := range req.Sets {
			sets = append(sets, SetInput{Exercise: s.Exercise, Reps: s.Reps, WeightGrams: s.WeightGrams})
		}
		out, err := svc.CreateWorkout(r.Context(), userID, WorkoutInput{
			Date: date, Type: req.Type, Note: req.Note, Sets: sets,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

func handleListWorkouts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		from := parseDateParam(r, "from")
		to := parseDateParam(r, "to")
		list, err := svc.ListWorkouts(r.Context(), userID, from, to)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleGetWorkout(svc *Service) http.HandlerFunc {
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
		out, err := svc.GetWorkout(r.Context(), userID, id)
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

func handleDeleteWorkout(svc *Service) http.HandlerFunc {
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
		deleted, err := svc.DeleteWorkout(r.Context(), userID, id)
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

// parseDateParam lee ?name=YYYY-MM-DD y devuelve *time.Time (nil si falta o es inválido).
func parseDateParam(r *http.Request, name string) *time.Time {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}
