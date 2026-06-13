package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL     string
	JWTSecret       string
	Port            string
	CORSOrigin      string
	GroqAPIKey      string
	GroqModel       string
	GroqVisionModel string
	CookieSecure    bool
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		Port:            os.Getenv("API_PORT"),
		CORSOrigin:      os.Getenv("CORS_ORIGIN"),
		GroqAPIKey:      os.Getenv("GROQ_API_KEY"),
		GroqModel:       os.Getenv("GROQ_MODEL"),
		GroqVisionModel: os.Getenv("GROQ_VISION_MODEL"),
		// COOKIE_SECURE=true en producción (detrás de HTTPS): la cookie de
		// refresh lleva el flag Secure.
		CookieSecure: os.Getenv("COOKIE_SECURE") == "true",
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.CORSOrigin == "" {
		cfg.CORSOrigin = "http://localhost:5173"
	}
	if cfg.GroqModel == "" {
		cfg.GroqModel = "llama-3.3-70b-versatile"
	}
	if cfg.GroqVisionModel == "" {
		cfg.GroqVisionModel = "meta-llama/llama-4-scout-17b-16e-instruct"
	}
	return cfg, nil
}
