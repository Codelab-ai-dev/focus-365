package commitments_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/commitments"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

func newSvc(t *testing.T) (*commitments.Service, uuid.UUID) {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	u, err := q.CreateUser(context.Background(), store.CreateUserParams{Email: "s@b.com", PasswordHash: "h", Name: "S"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return commitments.NewService(q, pool), u.ID
}

func TestReplaceForDateAndDueOn(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := svc.ReplaceForDate(ctx, uid, d, []string{"Tender la cama", "  ", "Pasear a Ruffo"}); err != nil {
		t.Fatalf("ReplaceForDate: %v", err)
	}
	due, err := svc.DueOn(ctx, uid, d)
	if err != nil {
		t.Fatalf("DueOn: %v", err)
	}
	if len(due) != 2 || due[0].Text != "Tender la cama" || due[1].Text != "Pasear a Ruffo" {
		t.Fatalf("due = %+v (esperaba 2, vacío filtrado)", due)
	}
	// Reemplazar de nuevo no duplica.
	if err := svc.ReplaceForDate(ctx, uid, d, []string{"Solo uno"}); err != nil {
		t.Fatalf("Replace 2: %v", err)
	}
	due2, _ := svc.DueOn(ctx, uid, d)
	if len(due2) != 1 || due2[0].Text != "Solo uno" {
		t.Errorf("replace duplicó: %+v", due2)
	}
}

func TestToggle(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	_ = svc.ReplaceForDate(ctx, uid, d, []string{"X"})
	due, _ := svc.DueOn(ctx, uid, d)
	id := due[0].ID
	c, err := svc.Toggle(ctx, uid, uuid.MustParse(id))
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !c.Done {
		t.Error("toggle debe marcar done")
	}
	// Toggle de otro usuario → (nil, nil).
	other, _ := newSvc(t)
	c2, err := other.Toggle(ctx, uuid.New(), uuid.MustParse(id))
	if err != nil || c2 != nil {
		t.Errorf("toggle ajeno = (%v, %v), want (nil, nil)", c2, err)
	}
}

func TestRecent(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	_ = svc.ReplaceForDate(ctx, uid, time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), []string{"viejo"})
	_ = svc.ReplaceForDate(ctx, uid, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), []string{"nuevo"})
	rec, err := svc.Recent(ctx, uid, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rec) != 1 || rec[0].Text != "nuevo" {
		t.Errorf("recent = %+v (since 15 debe excluir el 14)", rec)
	}
}

func TestPending(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	today := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	ayer := today.AddDate(0, 0, -1)
	manana := today.AddDate(0, 0, 1)

	if err := svc.ReplaceForDate(ctx, uid, ayer, []string{"Vencido"}); err != nil {
		t.Fatalf("Replace ayer: %v", err)
	}
	if err := svc.ReplaceForDate(ctx, uid, today, []string{"De hoy"}); err != nil {
		t.Fatalf("Replace hoy: %v", err)
	}
	if err := svc.ReplaceForDate(ctx, uid, manana, []string{"Futuro"}); err != nil {
		t.Fatalf("Replace manana: %v", err)
	}

	pend, err := svc.Pending(ctx, uid, today)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pend) != 2 || pend[0].Text != "Vencido" || pend[1].Text != "De hoy" {
		t.Fatalf("pending = %+v (esperaba [Vencido, De hoy])", pend)
	}

	if _, err := svc.Toggle(ctx, uid, uuid.MustParse(pend[0].ID)); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	pend2, err := svc.Pending(ctx, uid, today)
	if err != nil {
		t.Fatalf("Pending 2: %v", err)
	}
	if len(pend2) != 1 || pend2[0].Text != "De hoy" {
		t.Fatalf("pending2 = %+v (esperaba [De hoy])", pend2)
	}
}
