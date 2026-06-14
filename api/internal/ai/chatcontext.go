package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/commitments"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/google/uuid"
)

// recentCheckins es cuántos check-ins recientes incluimos en el contexto.
const recentCheckins = 14

// cycler es la porción de finance.Service que usamos (histórico de ciclos).
type cycler interface {
	Cycles(ctx context.Context, userID uuid.UUID, now time.Time) ([]finance.CycleSummary, error)
}

// checkinLister es la porción de checkin.Service que usamos (check-ins recientes).
type checkinLister interface {
	List(ctx context.Context, userID uuid.UUID, limit int) ([]checkin.CheckIn, error)
}

// habitLister es la porción de habits.Service que usamos (hábitos activos).
type habitLister interface {
	List(ctx context.Context, userID uuid.UUID, archived bool, today time.Time) ([]habits.Habit, error)
}

// goalLister es la porción de goals.Service que usamos (metas activas).
type goalLister interface {
	List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error)
}

// commitmentLister es la porción de commitments.Service que usamos.
type commitmentLister interface {
	Recent(ctx context.Context, userID uuid.UUID, since time.Time) ([]commitments.Commitment, error)
}

// chatContextBuilder arma el JSON de contexto que recibe la IA: snapshot del día
// + histórico financiero + check-ins recientes + hábitos activos + metas activas
// + compromisos recientes.
// Reutiliza la interfaz snapshotter ya definida en service.go (DRY).
type chatContextBuilder struct {
	dash    snapshotter
	finance cycler
	checkin checkinLister
	habits  habitLister
	goals   goalLister
	commits commitmentLister
}

// NewChatContextBuilder inyecta el dashboard (snapshot), finanzas (ciclos),
// check-ins, hábitos, metas y compromisos. Exportado para el wiring en server.go.
func NewChatContextBuilder(d snapshotter, f cycler, c checkinLister, h habitLister, g goalLister, co commitmentLister) *chatContextBuilder {
	return &chatContextBuilder{dash: d, finance: f, checkin: c, habits: h, goals: g, commits: co}
}

// newChatContextBuilder es el alias interno usado por los tests.
func newChatContextBuilder(d snapshotter, f cycler, c checkinLister, h habitLister, g goalLister, co commitmentLister) *chatContextBuilder {
	return NewChatContextBuilder(d, f, c, h, g, co)
}

// build compone el contexto en un JSON compacto. Propaga errores reales (DB).
func (b *chatContextBuilder) build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error) {
	snap, err := b.dash.Snapshot(ctx, userID, today)
	if err != nil {
		return "", err
	}
	cycles, err := b.finance.Cycles(ctx, userID, today)
	if err != nil {
		return "", err
	}
	checkins, err := b.checkin.List(ctx, userID, recentCheckins)
	if err != nil {
		return "", err
	}
	habs, err := b.habits.List(ctx, userID, false, today)
	if err != nil {
		return "", err
	}
	gls, err := b.goals.List(ctx, userID, "activa", today)
	if err != nil {
		return "", err
	}
	comms, err := b.commits.Recent(ctx, userID, today.AddDate(0, 0, -7))
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]any{
		"snapshot":    snap,
		"cycles":      cycles,
		"checkins":    checkins,
		"habits":      habs,
		"goals":       gls,
		"commitments": comms,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
