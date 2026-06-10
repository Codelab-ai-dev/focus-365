package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func newHandler(t *testing.T) http.Handler {
	pool := testutil.NewDB(t)
	svc := auth.NewService(store.New(pool), auth.NewTokenManager("secret"))
	return auth.Routes(svc)
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterEndpoint(t *testing.T) {
	h := newHandler(t)
	rec := postJSON(t, h, "/register", map[string]string{
		"email": "new@focus.com", "password": "p4ssword", "name": "Gus",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["access_token"] == nil {
		t.Error("falta access_token en la respuesta")
	}
}

func TestLoginEndpoint(t *testing.T) {
	h := newHandler(t)
	_ = postJSON(t, h, "/register", map[string]string{
		"email": "log@focus.com", "password": "p4ssword", "name": "G",
	})
	rec := postJSON(t, h, "/login", map[string]string{
		"email": "log@focus.com", "password": "p4ssword",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLoginBadCredentials(t *testing.T) {
	h := newHandler(t)
	rec := postJSON(t, h, "/login", map[string]string{
		"email": "nope@focus.com", "password": "x",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestRegisterValidation(t *testing.T) {
	h := newHandler(t)
	rec := postJSON(t, h, "/register", map[string]string{
		"email": "not-an-email", "password": "123", "name": "",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestRegisterValidationMessages(t *testing.T) {
	h := newHandler(t)
	cases := []struct {
		name string
		body map[string]string
		want string
	}{
		{"email inválido", map[string]string{"email": "bad", "password": "p4ssword", "name": "G"}, "El email no tiene un formato válido"},
		{"contraseña corta", map[string]string{"email": "ok@focus.com", "password": "123", "name": "G"}, "La contraseña debe tener al menos 6 caracteres"},
		{"nombre faltante", map[string]string{"email": "ok@focus.com", "password": "p4ssword", "name": ""}, "Falta el nombre"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := postJSON(t, h, "/register", c.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, want 400", rec.Code)
			}
			var resp map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			if resp["error"] != c.want {
				t.Errorf("error = %q, want %q", resp["error"], c.want)
			}
		})
	}
}
