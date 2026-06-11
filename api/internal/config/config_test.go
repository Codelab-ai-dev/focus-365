package config

import (
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "x")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoadDefaultsAndValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/focus")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("API_PORT", "")
	t.Setenv("CORS_ORIGIN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("default port = %q, want 8080", cfg.Port)
	}
	if cfg.CORSOrigin != "http://localhost:5173" {
		t.Errorf("default CORS = %q", cfg.CORSOrigin)
	}
	if cfg.DatabaseURL != "postgres://localhost/focus" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoadGroqDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/focus")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("GROQ_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "" {
		t.Errorf("GroqAPIKey = %q, want empty (modo degradado)", cfg.GroqAPIKey)
	}
	if cfg.GroqModel != "llama-3.3-70b-versatile" {
		t.Errorf("GroqModel default = %q", cfg.GroqModel)
	}
}

func TestLoadGroqValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/focus")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("GROQ_API_KEY", "gsk_abc")
	t.Setenv("GROQ_MODEL", "llama-custom")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "gsk_abc" {
		t.Errorf("GroqAPIKey = %q", cfg.GroqAPIKey)
	}
	if cfg.GroqModel != "llama-custom" {
		t.Errorf("GroqModel = %q", cfg.GroqModel)
	}
}
