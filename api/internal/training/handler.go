package training

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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
	r.Get("/profile", handleGetProfile(svc))
	r.Put("/profile", handleSaveProfile(svc))
	r.Get("/suggestion", handleGetSuggestion(svc))
	r.Post("/suggestion", handleSuggest(svc))
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

type profileReq struct {
	Birthdate   *string  `json:"birthdate"`
	Sex         *string  `json:"sex" validate:"omitempty,oneof=masculino femenino otro"`
	HeightCm    *int32   `json:"height_cm" validate:"omitempty,min=1"`
	WeightGrams *int32   `json:"weight_grams" validate:"omitempty,min=1"`
	Objective   *string  `json:"objective" validate:"omitempty,oneof=perder_grasa hipertrofia fuerza resistencia salud"`
	Location    *string  `json:"location" validate:"omitempty,oneof=casa gym ambos"`
	Level       *string  `json:"level" validate:"omitempty,oneof=principiante intermedio avanzado"`
	WeeklyDays  *int32   `json:"weekly_days" validate:"omitempty,min=1,max=7"`
	Equipment   []string `json:"equipment" validate:"omitempty,dive,oneof=peso_corporal mancuernas barra banco bandas kettlebell dominadas gym"`
	Limitations *string  `json:"limitations"`
}

func handleGetProfile(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		p, err := svc.Profile(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// p puede ser nil -> se serializa como null (200).
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}

func handleSaveProfile(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req profileReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		var birthdate *time.Time
		if req.Birthdate != nil && *req.Birthdate != "" {
			d, err := time.Parse(profileDateLayout, *req.Birthdate)
			if err != nil {
				httpx.WriteErr(w, http.StatusBadRequest, "la fecha de nacimiento no tiene un formato válido (YYYY-MM-DD)")
				return
			}
			birthdate = &d
		}
		limitations := ""
		if req.Limitations != nil {
			limitations = *req.Limitations
		}
		out, err := svc.SaveProfile(r.Context(), userID, ProfileInput{
			Birthdate: birthdate, Sex: req.Sex, HeightCm: req.HeightCm,
			WeightGrams: req.WeightGrams, Objective: req.Objective, Location: req.Location,
			Level: req.Level, WeeklyDays: req.WeeklyDays, Equipment: req.Equipment,
			Limitations: limitations,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

type suggestReq struct {
	Focus string `json:"focus"`
}

func handleGetSuggestion(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		s, err := svc.Suggestion(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// s puede ser nil -> null (200).
		httpx.WriteJSON(w, http.StatusOK, s)
	}
}

func handleSuggest(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req suggestReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		focus := strings.TrimSpace(req.Focus)
		if utf8.RuneCountInString(focus) > maxFocusChars {
			httpx.WriteErr(w, http.StatusBadRequest, "el enfoque es demasiado largo")
			return
		}
		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		out, err := svc.SuggestTraining(r.Context(), userID, focus, today)
		if err != nil {
			if errors.Is(err, ErrUnavailable) {
				httpx.WriteErr(w, http.StatusServiceUnavailable, "el entrenador no está disponible por ahora")
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
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
