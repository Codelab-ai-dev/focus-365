package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestCreateAndListMessages(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	ada, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "msg-a@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser Ada: %v", err)
	}
	bob, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "msg-b@b.com", PasswordHash: "h", Name: "Bob",
	})
	if err != nil {
		t.Fatalf("CreateUser Bob: %v", err)
	}

	// Ada escribe una pregunta y recibe una respuesta.
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: ada.ID, Role: "user", Content: "¿cómo voy en junio?",
	}); err != nil {
		t.Fatalf("CreateMessage user: %v", err)
	}
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: ada.ID, Role: "assistant", Content: "Vas verde este ciclo.",
	}); err != nil {
		t.Fatalf("CreateMessage assistant: %v", err)
	}
	// Mensaje de Bob: no debe aparecer en el historial de Ada (scoping).
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: bob.ID, Role: "user", Content: "hola",
	}); err != nil {
		t.Fatalf("CreateMessage Bob: %v", err)
	}

	rows, err := q.ListMessages(ctx, ada.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Ada tiene %d mensajes, want 2 (scoping falló)", len(rows))
	}
	// Orden ASC por created_at: primero la pregunta, luego la respuesta.
	if rows[0].Role != "user" || rows[0].Content != "¿cómo voy en junio?" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[1].Role != "assistant" || rows[1].Content != "Vas verde este ciclo." {
		t.Errorf("rows[1] = %+v", rows[1])
	}
	if rows[1].CreatedAt.Before(rows[0].CreatedAt) {
		t.Errorf("orden incorrecto: rows[1] antes que rows[0]")
	}
}

func TestAiMessageActionRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "action-rt@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	kind := "checkin"
	status := "proposed"
	m, err := q.CreateMessageWithAction(ctx, store.CreateMessageWithActionParams{
		UserID: u.ID, Role: "assistant", Content: "Propongo registrar tu check-in.",
		ActionKind: &kind, ActionPayload: []byte(`{"mood":8,"energy":6,"discipline":9}`),
		ActionStatus: &status,
	})
	if err != nil {
		t.Fatalf("CreateMessageWithAction: %v", err)
	}
	if m.ActionKind == nil || *m.ActionKind != "checkin" || m.ActionStatus == nil || *m.ActionStatus != "proposed" {
		t.Errorf("acción mal persistida: %+v", m)
	}

	got, err := q.GetMessageForAction(ctx, store.GetMessageForActionParams{ID: m.ID, UserID: u.ID})
	if err != nil {
		t.Fatalf("GetMessageForAction: %v", err)
	}
	if string(got.ActionPayload) != `{"mood":8,"energy":6,"discipline":9}` &&
		string(got.ActionPayload) != `{"mood": 8, "energy": 6, "discipline": 9}` {
		t.Errorf("payload = %s", got.ActionPayload)
	}

	// Transición válida: proposed → done.
	upd, err := q.SetActionStatus(ctx, store.SetActionStatusParams{ID: m.ID, UserID: u.ID, ActionStatus: ptr("done")})
	if err != nil {
		t.Fatalf("SetActionStatus: %v", err)
	}
	if upd.ActionStatus == nil || *upd.ActionStatus != "done" {
		t.Errorf("status = %v", upd.ActionStatus)
	}

	// Segunda transición: ya no está proposed → ErrNoRows (conflicto).
	if _, err := q.SetActionStatus(ctx, store.SetActionStatusParams{ID: m.ID, UserID: u.ID, ActionStatus: ptr("cancelled")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("doble transición err = %v, want pgx.ErrNoRows", err)
	}

	// Otro usuario no puede tocar la acción.
	otro, _ := q.CreateUser(ctx, store.CreateUserParams{Email: "action-otro@b.com", PasswordHash: "h", Name: "Eve"})
	if _, err := q.GetMessageForAction(ctx, store.GetMessageForActionParams{ID: m.ID, UserID: otro.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("scoping err = %v, want pgx.ErrNoRows", err)
	}
}

func ptr(s string) *string { return &s }
