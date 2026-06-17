package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func ptrI(v int32) *int32   { return &v }
func ptrS(v string) *string { return &v }

func TestFitnessProfileUpsertAndGet(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	// no existe -> ErrNoRows
	if _, err := q.GetFitnessProfile(ctx, u); err != pgx.ErrNoRows {
		t.Fatalf("perfil inexistente: err = %v, want ErrNoRows", err)
	}

	// insert
	p, err := q.UpsertFitnessProfile(ctx, store.UpsertFitnessProfileParams{
		UserID: u, Sex: ptrS("masculino"), HeightCm: ptrI(178),
		WeightGrams: ptrI(80500), Objective: ptrS("hipertrofia"),
		Location: ptrS("casa"), Level: ptrS("intermedio"), WeeklyDays: ptrI(4),
		Equipment: []string{"mancuernas", "bandas"}, Limitations: "cuido la rodilla",
	})
	if err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	if p.Sex == nil || *p.Sex != "masculino" || len(p.Equipment) != 2 || p.Limitations != "cuido la rodilla" {
		t.Fatalf("perfil insertado = %+v", p)
	}

	// update (mismo user) -> sigue una sola fila, valores nuevos
	p2, err := q.UpsertFitnessProfile(ctx, store.UpsertFitnessProfileParams{
		UserID: u, Objective: ptrS("fuerza"), WeeklyDays: ptrI(5),
		Equipment: []string{"barra"}, Limitations: "",
	})
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if p2.Objective == nil || *p2.Objective != "fuerza" || len(p2.Equipment) != 1 || p2.Equipment[0] != "barra" {
		t.Fatalf("perfil actualizado = %+v", p2)
	}
	// los campos no pasados quedan en NULL tras el upsert (reemplazo completo)
	if p2.Sex != nil {
		t.Errorf("sex debería ser NULL tras el reemplazo, got %v", *p2.Sex)
	}

	// get devuelve el actualizado
	got, err := q.GetFitnessProfile(ctx, u)
	if err != nil || got.Objective == nil || *got.Objective != "fuerza" {
		t.Fatalf("get tras update: %v %+v", err, got)
	}
}
