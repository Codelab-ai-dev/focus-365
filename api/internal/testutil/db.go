package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/focus365/api/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDB devuelve un pool contra TEST_DATABASE_URL, corre migraciones y
// trunca las tablas. Hace t.Skip si la variable no está definida.
func NewDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida; se omite test de base de datos")
	}
	if err := db.RunMigrations(url, "../../db/migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.NewPool(context.Background(), url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE users CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}
