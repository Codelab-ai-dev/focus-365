package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

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

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/register", handleRegister(svc))
	r.Post("/login", handleLogin(svc))
	r.Post("/refresh", handleRefresh(svc))
	r.With(RequireAuth(svc.Tokens())).Get("/me", handleMe(svc))
	return r
}

func handleRegister(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerReq
		if !decodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Register(r.Context(), req.Email, req.Password, req.Name)
		if err != nil {
			if errors.Is(err, ErrEmailTaken) {
				writeErr(w, http.StatusConflict, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		respondWithTokens(w, svc, user, http.StatusCreated)
	}
}

func handleLogin(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if !decodeAndValidate(w, r, &req) {
			return
		}
		user, err := svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK)
	}
}

func handleRefresh(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "sin refresh token")
			return
		}
		id, err := svc.Tokens().ParseRefresh(cookie.Value)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "refresh inválido")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "usuario no encontrado")
			return
		}
		respondWithTokens(w, svc, user, http.StatusOK)
	}
}

func handleMe(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserIDFromContext(r.Context())
		if !ok {
			writeErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		user, err := svc.q.GetUserByID(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "usuario no encontrado")
			return
		}
		writeJSON(w, http.StatusOK, toView(user))
	}
}

func respondWithTokens(w http.ResponseWriter, svc *Service, user store.User, status int) {
	access, refresh, err := svc.IssueTokens(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error emitiendo tokens")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refresh,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(RefreshTTL()),
	})
	writeJSON(w, status, authResp{AccessToken: access, User: toView(user)})
}

func toView(u store.User) userView {
	return userView{ID: u.ID.String(), Email: u.Email, Name: u.Name}
}

func decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return false
	}
	if err := validate.Struct(dst); err != nil {
		writeErr(w, http.StatusBadRequest, validationMessage(err))
		return false
	}
	return true
}

// validationMessage traduce el primer error de validación a un mensaje claro
// en español, indicando qué campo falló y por qué.
func validationMessage(err error) string {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) || len(verrs) == 0 {
		return "datos inválidos"
	}
	fe := verrs[0]
	label := fieldLabel(fe.Field())
	switch fe.Tag() {
	case "required":
		return "Falta " + label
	case "email":
		return "El email no tiene un formato válido"
	case "min":
		return capitalize(label) + " debe tener al menos " + fe.Param() + " caracteres"
	default:
		return capitalize(label) + " no es válido"
	}
}

func fieldLabel(field string) string {
	switch field {
	case "Email":
		return "el email"
	case "Password":
		return "la contraseña"
	case "Name":
		return "el nombre"
	default:
		return field
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
