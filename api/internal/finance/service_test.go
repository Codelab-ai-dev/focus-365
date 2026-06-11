package finance_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestServiceFinanzas(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := finance.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "f@b.com", PasswordHash: "h", Name: "Fin"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	now := d(2026, time.June, 15) // ciclo actual = junio (payday jun = 30)
	junio := finance.Cycle(now)

	// Crea ingreso, gasto y transferencia en el ciclo de junio.
	in1, err := svc.Create(ctx, user.ID, finance.Input{Type: "income", Amount: 500000, OccurredOn: d(2026, time.June, 10), Category: "sueldo"})
	if err != nil {
		t.Fatalf("Create income: %v", err)
	}
	if in1.Cycle != "2026-06" {
		t.Errorf("cycle = %q, want 2026-06", in1.Cycle)
	}
	if _, err := svc.Create(ctx, user.ID, finance.Input{Type: "expense", Amount: 200000, OccurredOn: d(2026, time.June, 12), Category: "renta"}); err != nil {
		t.Fatalf("Create expense: %v", err)
	}
	transfer, err := svc.Create(ctx, user.ID, finance.Input{Type: "transfer", Amount: 100000, OccurredOn: d(2026, time.June, 11)})
	if err != nil {
		t.Fatalf("Create transfer: %v", err)
	}

	// ListByCycle: 3 transacciones, descendente por occurred_on.
	list, err := svc.ListByCycle(ctx, user.ID, junio)
	if err != nil {
		t.Fatalf("ListByCycle: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	if list[0].OccurredOn != "2026-06-12" {
		t.Errorf("orden incorrecto: list[0] = %q, want 2026-06-12", list[0].OccurredOn)
	}

	// Summary del ciclo actual: net excluye la transferencia, status pendiente.
	sum, err := svc.Summary(ctx, user.ID, junio, now)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.Income != 500000 || sum.Expense != 200000 || sum.Net != 300000 {
		t.Errorf("summary = %+v, want income 500000 expense 200000 net 300000", sum)
	}
	if sum.Status != "pendiente" {
		t.Errorf("status = %q, want pendiente", sum.Status)
	}

	// Un gasto en un ciclo pasado (abril) deja ese ciclo cerrado en rojo.
	if _, err := svc.Create(ctx, user.ID, finance.Input{Type: "expense", Amount: 999999, OccurredOn: d(2026, time.April, 10)}); err != nil {
		t.Fatalf("Create abril: %v", err)
	}
	abril := finance.Cycle(d(2026, time.April, 10))
	sumAbr, err := svc.Summary(ctx, user.ID, abril, now)
	if err != nil {
		t.Fatalf("Summary abril: %v", err)
	}
	if sumAbr.Net != -999999 || sumAbr.Status != "rojo" {
		t.Errorf("summary abril = %+v, want net -999999 status rojo", sumAbr)
	}

	// Cycles: historial descendente con su status.
	cycles, err := svc.Cycles(ctx, user.ID, now)
	if err != nil {
		t.Fatalf("Cycles: %v", err)
	}
	if len(cycles) != 2 {
		t.Fatalf("len(cycles) = %d, want 2", len(cycles))
	}
	if cycles[0].Cycle != "2026-06" || cycles[0].Status != "pendiente" {
		t.Errorf("cycles[0] = %+v, want 2026-06 pendiente", cycles[0])
	}
	if cycles[1].Cycle != "2026-04" || cycles[1].Status != "rojo" {
		t.Errorf("cycles[1] = %+v, want 2026-04 rojo", cycles[1])
	}

	// Delete: borra la transferencia; borrar de nuevo no afecta filas.
	ok, err := svc.Delete(ctx, user.ID, mustUUID(t, transfer.ID))
	if err != nil || !ok {
		t.Fatalf("Delete: ok=%v err=%v", ok, err)
	}
	ok2, err := svc.Delete(ctx, user.ID, mustUUID(t, transfer.ID))
	if err != nil {
		t.Fatalf("Delete repetido: %v", err)
	}
	if ok2 {
		t.Errorf("segundo Delete devolvió true; debería ser false")
	}

	// Aislamiento: otro usuario no puede borrar la transacción de Fin.
	other, _ := q.CreateUser(ctx, store.CreateUserParams{Email: "o@b.com", PasswordHash: "h", Name: "Otro"})
	okOther, err := svc.Delete(ctx, other.ID, mustUUID(t, in1.ID))
	if err != nil {
		t.Fatalf("Delete ajeno: %v", err)
	}
	if okOther {
		t.Errorf("otro usuario borró una transacción ajena")
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", s, err)
	}
	return id
}
