// Package checkin implementa el dominio del check-in diario: upsert por día,
// consulta del día e historial, siempre scoped por user_id.
package checkin

import (
	"context"
	"errors"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// dateLayout es el formato de fecha que viaja por la API (YYYY-MM-DD).
const dateLayout = "2006-01-02"

type Service struct {
	q *store.Queries
}

func NewService(q *store.Queries) *Service {
	return &Service{q: q}
}

// Input son los datos de dominio para crear/actualizar un check-in.
type Input struct {
	Date       time.Time
	Mood       int
	Energy     int
	Discipline int
	Note       string
}

// CheckIn es la vista de dominio que se serializa a JSON. Date va como string
// YYYY-MM-DD para evitar supuestos de timezone.
type CheckIn struct {
	ID         string    `json:"id"`
	Date       string    `json:"date"`
	Mood       int       `json:"mood"`
	Energy     int       `json:"energy"`
	Discipline int       `json:"discipline"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *Service) Upsert(ctx context.Context, userID uuid.UUID, in Input) (*CheckIn, error) {
	row, err := s.q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID:     userID,
		Date:       in.Date,
		Mood:       int32(in.Mood),
		Energy:     int32(in.Energy),
		Discipline: int32(in.Discipline),
		Note:       in.Note,
	})
	if err != nil {
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// Today devuelve el check-in del día o (nil, nil) si no existe.
func (s *Service) Today(ctx context.Context, userID uuid.UUID, date time.Time) (*CheckIn, error) {
	row, err := s.q.GetCheckInByDate(ctx, store.GetCheckInByDateParams{UserID: userID, Date: date})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// Delete borra el check-in del día. Devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, date time.Time) (bool, error) {
	n, err := s.q.DeleteCheckIn(ctx, store.DeleteCheckInParams{UserID: userID, Date: date})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, limit int) ([]CheckIn, error) {
	rows, err := s.q.ListCheckIns(ctx, store.ListCheckInsParams{UserID: userID, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]CheckIn, 0, len(rows))
	for _, row := range rows {
		out = append(out, toView(row))
	}
	return out, nil
}

func toView(row store.CheckIn) CheckIn {
	return CheckIn{
		ID:         row.ID.String(),
		Date:       row.Date.Format(dateLayout),
		Mood:       int(row.Mood),
		Energy:     int(row.Energy),
		Discipline: int(row.Discipline),
		Note:       row.Note,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
