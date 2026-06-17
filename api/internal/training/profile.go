package training

import (
	"context"
	"errors"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const profileDateLayout = "2006-01-02"

// Profile es la vista del perfil de fitness. birthdate va como YYYY-MM-DD; los
// opcionales no seteados van como null. weight_grams/height_cm en enteros (el
// front convierte kg/cm con sus helpers).
type Profile struct {
	Birthdate   *string   `json:"birthdate"`
	Sex         *string   `json:"sex"`
	HeightCm    *int32    `json:"height_cm"`
	WeightGrams *int32    `json:"weight_grams"`
	Objective   *string   `json:"objective"`
	Location    *string   `json:"location"`
	Level       *string   `json:"level"`
	WeeklyDays  *int32    `json:"weekly_days"`
	Equipment   []string  `json:"equipment"`
	Limitations string    `json:"limitations"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProfileInput son los datos de dominio para guardar el perfil.
type ProfileInput struct {
	Birthdate   *time.Time
	Sex         *string
	HeightCm    *int32
	WeightGrams *int32
	Objective   *string
	Location    *string
	Level       *string
	WeeklyDays  *int32
	Equipment   []string
	Limitations string
}

func buildProfile(p store.FitnessProfile) Profile {
	var bd *string
	if p.Birthdate != nil {
		s := p.Birthdate.Format(profileDateLayout)
		bd = &s
	}
	eq := p.Equipment
	if eq == nil {
		eq = []string{}
	}
	return Profile{
		Birthdate: bd, Sex: p.Sex, HeightCm: p.HeightCm, WeightGrams: p.WeightGrams,
		Objective: p.Objective, Location: p.Location, Level: p.Level,
		WeeklyDays: p.WeeklyDays, Equipment: eq, Limitations: p.Limitations,
		UpdatedAt: p.UpdatedAt,
	}
}

// Profile devuelve el perfil del usuario, o nil si nunca lo guardó.
func (s *Service) Profile(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	p, err := s.q.GetFitnessProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := buildProfile(p)
	return &v, nil
}

// SaveProfile hace upsert del perfil y devuelve el resultado.
func (s *Service) SaveProfile(ctx context.Context, userID uuid.UUID, in ProfileInput) (*Profile, error) {
	eq := in.Equipment
	if eq == nil {
		eq = []string{}
	}
	p, err := s.q.UpsertFitnessProfile(ctx, store.UpsertFitnessProfileParams{
		UserID: userID, Birthdate: in.Birthdate, Sex: in.Sex, HeightCm: in.HeightCm,
		WeightGrams: in.WeightGrams, Objective: in.Objective, Location: in.Location,
		Level: in.Level, WeeklyDays: in.WeeklyDays, Equipment: eq, Limitations: in.Limitations,
	})
	if err != nil {
		return nil, err
	}
	v := buildProfile(p)
	return &v, nil
}
