package finance_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestImportEsIdempotente(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := finance.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "imp@b.com", PasswordHash: "h", Name: "Imp"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	day := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC) // ciclo junio
	txs := []finance.ExternalTx{
		{ExternalID: "ext-1", Type: "expense", Amount: 12345, OccurredOn: day, Category: "luz"},
		{ExternalID: "ext-2", Type: "income", Amount: 500000, OccurredOn: day, Category: "sueldo"},
	}
	n, err := svc.Import(ctx, user.ID, txs)
	if err != nil || n != 2 {
		t.Fatalf("Import: n=%d err=%v", n, err)
	}

	// Reimportar ext-1 con monto distinto: actualiza, no duplica.
	n2, err := svc.Import(ctx, user.ID, []finance.ExternalTx{
		{ExternalID: "ext-1", Type: "expense", Amount: 999, OccurredOn: day, Category: "luz"},
	})
	if err != nil || n2 != 1 {
		t.Fatalf("Reimport: n=%d err=%v", n2, err)
	}

	junio := finance.Cycle(day)
	list, err := svc.ListByCycle(ctx, user.ID, junio)
	if err != nil {
		t.Fatalf("ListByCycle: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2 (sin duplicar)", len(list))
	}
	var updated bool
	for _, tx := range list {
		if tx.Amount == 999 {
			updated = true
		}
	}
	if !updated {
		t.Errorf("ext-1 no se actualizó al monto 999")
	}
}
