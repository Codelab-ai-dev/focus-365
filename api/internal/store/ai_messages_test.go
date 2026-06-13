package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

// createAssistantMsg crea un mensaje de asistente al que colgar acciones.
func createAssistantMsg(t *testing.T, q *store.Queries, ctx context.Context, userID uuid.UUID) store.AiMessage {
	t.Helper()
	m, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "assistant", Content: "Propongo una acción.",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	return m
}

func TestAiActionRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "action-rt@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	msg := createAssistantMsg(t, q, ctx, u.ID)

	a, err := q.CreateAction(ctx, store.CreateActionParams{
		MessageID: pgtype.UUID{Bytes: msg.ID, Valid: true},
		UserID: u.ID, Position: 0, Kind: "checkin",
		Payload: []byte(`{"mood":8,"energy":6,"discipline":9}`), Status: "proposed",
	})
	if err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	if a.Kind != "checkin" || a.Status != "proposed" {
		t.Errorf("acción mal persistida: %+v", a)
	}

	got, err := q.GetAction(ctx, store.GetActionParams{ID: a.ID, UserID: u.ID})
	if err != nil {
		t.Fatalf("GetAction: %v", err)
	}
	if string(got.Payload) != `{"mood":8,"energy":6,"discipline":9}` &&
		string(got.Payload) != `{"mood": 8, "energy": 6, "discipline": 9}` {
		t.Errorf("payload = %s", got.Payload)
	}

	// ListActionsByMessages las cuelga del mensaje.
	list, err := q.ListActionsByMessages(ctx, []uuid.UUID{msg.ID})
	if err != nil {
		t.Fatalf("ListActionsByMessages: %v", err)
	}
	if len(list) != 1 || list[0].ID != a.ID {
		t.Errorf("list = %+v", list)
	}

	// Transición válida: proposed → done, guardando result.
	upd, err := q.SetActionStatusFrom(ctx, store.SetActionStatusFromParams{
		ID: a.ID, UserID: u.ID, Status: "done", Result: []byte(`{"tx_id":"x"}`), Status_2: "proposed",
	})
	if err != nil {
		t.Fatalf("SetActionStatusFrom done: %v", err)
	}
	if upd.Status != "done" || string(upd.Result) != `{"tx_id":"x"}` && string(upd.Result) != `{"tx_id": "x"}` {
		t.Errorf("upd = %+v", upd)
	}

	// Transición done → undone (modelo nuevo).
	und, err := q.SetActionStatusFrom(ctx, store.SetActionStatusFromParams{
		ID: a.ID, UserID: u.ID, Status: "undone", Result: nil, Status_2: "done",
	})
	if err != nil {
		t.Fatalf("SetActionStatusFrom undone: %v", err)
	}
	if und.Status != "undone" {
		t.Errorf("status = %v, want undone", und.Status)
	}
	// COALESCE: result nil no pisa el guardado.
	if string(und.Result) != `{"tx_id":"x"}` && string(und.Result) != `{"tx_id": "x"}` {
		t.Errorf("result pisado = %s", und.Result)
	}

	// Doble transición desde un from que ya no aplica → ErrNoRows (conflicto).
	if _, err := q.SetActionStatusFrom(ctx, store.SetActionStatusFromParams{
		ID: a.ID, UserID: u.ID, Status: "cancelled", Result: nil, Status_2: "proposed",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("doble transición err = %v, want pgx.ErrNoRows", err)
	}

	// Otro usuario no puede leer la acción (scoping).
	otro, _ := q.CreateUser(ctx, store.CreateUserParams{Email: "action-otro@b.com", PasswordHash: "h", Name: "Eve"})
	if _, err := q.GetAction(ctx, store.GetActionParams{ID: a.ID, UserID: otro.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("scoping err = %v, want pgx.ErrNoRows", err)
	}
}

func TestAiActionAllKinds(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "kinds-rt@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	msg := createAssistantMsg(t, q, ctx, u.ID)
	for i, kind := range []string{
		"checkin", "movimiento", "habito", "meta",
		"habito_nuevo", "meta_nueva", "entrenamiento",
	} {
		if _, err := q.CreateAction(ctx, store.CreateActionParams{
			MessageID: pgtype.UUID{Bytes: msg.ID, Valid: true},
			UserID: u.ID, Position: int32(i),
			Kind: kind, Payload: []byte(`{}`), Status: "proposed",
		}); err != nil {
			t.Errorf("kind %s rechazado por el CHECK: %v", kind, err)
		}
	}
}

func TestUploadActionRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Email: "upl@b.com", PasswordHash: "h", Name: "U"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	a, err := q.CreateUploadAction(ctx, store.CreateUploadActionParams{
		UserID: u.ID, Position: 0, Kind: "movimiento",
		Payload: []byte(`{"type":"expense","amount_centavos":25000,"category":"comida"}`),
	})
	if err != nil {
		t.Fatalf("CreateUploadAction: %v", err)
	}
	if a.Source != "upload" || a.Status != "proposed" || a.MessageID.Valid {
		t.Errorf("acción upload mal creada: %+v", a)
	}

	pend, err := q.ListPendingUploadActions(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pend) != 1 || pend[0].ID != a.ID {
		t.Errorf("pending = %+v", pend)
	}

	// No aparece en el historial del chat (filtra por message_id).
	msgs, err := q.ListActionsByMessages(ctx, []uuid.UUID{a.ID})
	if err != nil {
		t.Fatalf("ListByMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("la acción upload no debe aparecer por message id: %+v", msgs)
	}
}

// TestAiActionsTableExists verifica vía SQL directo que la tabla de la
// migración 0011 existe y acepta una fila (la migración de datos en sí no se
// puede sembrar post-migración porque las columnas viejas ya no existen).
func TestAiActionsTableExists(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "table-exists@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	msg := createAssistantMsg(t, q, ctx, u.ID)
	var n int
	if err := pool.QueryRow(ctx,
		`INSERT INTO ai_actions (message_id, user_id, position, kind, payload, status)
		 VALUES ($1, $2, 0, 'checkin', '{}'::jsonb, 'proposed') RETURNING 1`,
		msg.ID, u.ID,
	).Scan(&n); err != nil {
		t.Fatalf("INSERT directo a ai_actions: %v", err)
	}
}
