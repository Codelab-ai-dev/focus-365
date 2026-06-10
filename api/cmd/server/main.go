package main

import (
	"log"
	"net/http"
	"os"

	"github.com/focus365/api/internal/server"
)

func main() {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	r := server.New()

	log.Printf("Focus 365 API escuchando en :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
