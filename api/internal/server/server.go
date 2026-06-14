package server

import (
	"encoding/json"
	"net/http"

	"github.com/focus365/api/internal/ai"
	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/commitments"
	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/training"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Pool            *pgxpool.Pool
	JWTSecret       string
	CORSOrigin      string
	GroqAPIKey      string
	GroqModel       string
	GroqVisionModel string
	CookieSecure    bool
}

func New(d Deps) http.Handler {
	q := store.New(d.Pool)
	tm := auth.NewTokenManager(d.JWTSecret)
	authSvc := auth.NewService(q, tm)
	checkinSvc := checkin.NewService(q)
	financeSvc := finance.NewService(q)
	trainingSvc := training.NewService(q, d.Pool)
	habitsSvc := habits.NewService(q)
	goalsSvc := goals.NewService(q)
	commitmentsSvc := commitments.NewService(q, d.Pool)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors(d.CORSOrigin))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health)
		r.Mount("/auth", auth.Routes(authSvc, d.CookieSecure))
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(tm))
			r.Mount("/checkins", checkin.Routes(checkinSvc, commitmentsSvc))
			r.Mount("/commitments", commitments.Routes(commitmentsSvc))
			r.Mount("/finances", finance.Routes(financeSvc))
			r.Mount("/training", training.Routes(trainingSvc))
			r.Mount("/habits", habits.Routes(habitsSvc))
			r.Mount("/goals", goals.Routes(goalsSvc))
			dashboardSvc := dashboard.NewService(checkinSvc, financeSvc, trainingSvc, habitsSvc, goalsSvc)
			r.Mount("/dashboard", dashboard.Routes(dashboardSvc))
			groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel, d.GroqVisionModel)
			aiSvc := ai.NewService(dashboardSvc, q, groq, d.GroqAPIKey != "")
			chatCtx := ai.NewChatContextBuilder(dashboardSvc, financeSvc, checkinSvc, habitsSvc, goalsSvc, commitmentsSvc)
			chatStore := ai.NewChatStore(q, d.Pool)
			actionExec := ai.NewActionExecutor(checkinSvc, financeSvc, habitsSvc, goalsSvc, trainingSvc)
			chatSvc := ai.NewChatService(chatCtx, chatStore, groq, groq, actionExec, d.GroqAPIKey != "")
			importSvc := ai.NewImportService(groq, chatStore, d.GroqAPIKey != "")
			r.Mount("/ai", ai.Routes(aiSvc, chatSvc, importSvc))
		})
	})

	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "focus-365-api",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cors(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
