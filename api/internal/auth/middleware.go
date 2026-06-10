package auth

import (
	"net/http"
	"strings"
)

func RequireAuth(tm *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error":"no autorizado"}`, http.StatusUnauthorized)
				return
			}
			id, err := tm.ParseAccess(parts[1])
			if err != nil {
				http.Error(w, `{"error":"no autorizado"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), id)))
		})
	}
}
