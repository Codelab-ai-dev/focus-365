package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroqCompleteOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Buen ritmo hoy."}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
	got, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "Buen ritmo hoy." {
		t.Errorf("content = %q", got)
	}
}

func TestGroqCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}

func TestGroqCompleteNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error sin choices")
	}
}

func TestGroqCompleteInvalidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`no-json`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error con body inválido")
	}
}
