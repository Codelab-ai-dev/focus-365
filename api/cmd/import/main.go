// Command import trae el histórico de transacciones del servicio externo
// (money.quhou123.com) y lo inserta idempotentemente para un usuario de Focus.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/focus365/api/internal/db"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
)

func main() {
	var (
		email   = flag.String("email", "", "email del usuario de Focus al que asignar las transacciones")
		baseURL = flag.String("base-url", os.Getenv("FOCUS_IMPORT_BASE_URL"), "URL base del servicio externo")
		token   = flag.String("token", os.Getenv("FOCUS_IMPORT_TOKEN"), "token del servicio externo")
		from    = flag.String("from", "", "fecha inicial YYYY-MM-DD")
		to      = flag.String("to", "", "fecha final YYYY-MM-DD")
	)
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" || *email == "" || *baseURL == "" || *token == "" || *from == "" || *to == "" {
		log.Fatal("faltan parámetros: DATABASE_URL, --email, --base-url, --token, --from, --to son obligatorios")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	q := store.New(pool)

	user, err := q.GetUserByEmail(ctx, *email)
	if err != nil {
		log.Fatalf("usuario %q no encontrado: %v", *email, err)
	}

	svc := finance.NewService(q)
	txs, err := finance.FetchTransactions(ctx, finance.ImportConfig{
		BaseURL: *baseURL, Token: *token, From: *from, To: *to,
	})
	if err != nil {
		log.Fatalf("fetch: %v", err)
	}

	started := time.Now()
	n, err := svc.Import(ctx, user.ID, txs)
	if err != nil {
		log.Fatalf("import (procesadas %d): %v", n, err)
	}
	log.Printf("importadas %d transacciones para %s en %s", n, *email, time.Since(started))
}
