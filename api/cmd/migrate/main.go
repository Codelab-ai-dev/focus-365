package main

import (
	"log"
	"os"

	"github.com/focus365/api/internal/db"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if err := db.RunMigrations(url, "db/migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Println("migrations applied")
}
