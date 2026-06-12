package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/focus365/api/internal/httpx"
	"github.com/focus365/api/internal/store"
	"github.com/go-chi/chi/v5"
)

type registerReq struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
	Name     string `json:"name" validate:"required"`
}

type loginReq struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type authResp struct {
	AccessToken string   `json:"access_token"`
	User        userView `json:"user"`
}

// Routes monta los endpoints de auth. secureCookies activa el flag Secure de
// la cookie de refresh (true en producción, detrás de HTTPS).
func Routes(svc *Service, secureCookies bool) http.Handler {
	r := chi.NewRouter()
	r.Post("/register", handleRegister(svc, secureCookies))
	r.Post("/login", handleLogin(svc, secureCookies))
	r.Post("/refresh", handleRefresh(svc, secureCookies))
	r.Post("/logout", handleLogout(secureCookies))
	r.With(RequireAuth(svc.Tokens())).Get("/me", handleMe(svc))
	return r
}

func handleRegister(svc *Service, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Register(r.Context(), req.Email, req.Password, req.Name)
		if err != nil {
			if errors.Is(err, ErrEmailTaken) {
				httpx.WriteErr(w, http.StatusConflict, err.Error())
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		respondWithTokens(w, svc, user, http.StatusCreated, secure)
	}
}

func handleLogin(svc *Service, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK, secure)
	}
}

func handleRefresh(svc *Service, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "sin refresh token")
			return
		}
		id, err := svc.Tokens().ParseRefresh(cookie.Value)
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "refresh inválido")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			httpx.WriteErr(w, http.StatusUnauthorized, "usuario no encontrado")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK, secure)
	}
}

func handleLogout(secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    "",
			Path:     "/api/v1/auth",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMe(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "usuario no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toView(user))
	}
}

func respondWithTokens(w http.ResponseWriter, svc *Service, user store.User, status int, secure bool) {
	access, refresh, err := svc.IssueTokens(user.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, "error emitiendo tokens")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refresh,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(RefreshTTL()),
	})
	httpx.WriteJSON(w, status, authResp{AccessToken: access, User: toView(user)})
}

func toView(u store.User) userView {
	return userView{ID: u.ID.String(), Email: u.Email, Name: u.Name}
}
