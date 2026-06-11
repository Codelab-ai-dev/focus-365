package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
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

// chatContextBuilder arma el JSON de contexto que recibe la IA: snapshot del día
// + histórico financiero + check-ins recientes. Reutiliza la interfaz
// snapshotter ya definida en service.go (DRY).
type chatContextBuilder struct {
	dash    snapshotter
	finance cycler
	checkin checkinLister
}

// NewChatContextBuilder inyecta el dashboard (snapshot), finanzas (ciclos) y
// check-ins. Exportado para el wiring en server.go.
func NewChatContextBuilder(d snapshotter, f cycler, c checkinLister) *chatContextBuilder {
	return &chatContextBuilder{dash: d, finance: f, checkin: c}
}

// newChatContextBuilder es el alias interno usado por los tests.
func newChatContextBuilder(d snapshotter, f cycler, c checkinLister) *chatContextBuilder {
	return NewChatContextBuilder(d, f, c)
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
	out, err := json.Marshal(map[string]any{
		"snapshot": snap,
		"cycles":   cycles,
		"checkins": checkins,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
