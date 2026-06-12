package main

import (
	"context"
	"log"
	"net/http"

	"github.com/focus365/api/internal/config"
	"github.com/focus365/api/internal/db"
	"github.com/focus365/api/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	h := server.New(server.Deps{
		Pool:         pool,
		JWTSecret:    cfg.JWTSecret,
		CORSOrigin:   cfg.CORSOrigin,
		GroqAPIKey:   cfg.GroqAPIKey,
		GroqModel:    cfg.GroqModel,
		CookieSecure: cfg.CookieSecure,
	})

	log.Printf("Focus 365 API escuchando en :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, h); err != nil {
		log.Fatal(err)
	}
}
