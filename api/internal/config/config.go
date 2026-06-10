package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
	CORSOrigin  string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        os.Getenv("API_PORT"),
		CORSOrigin:  os.Getenv("CORS_ORIGIN"),
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
	return cfg, nil
}
