package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const kindProactive = "proactive"

// snapshotter es la porción de dashboard.Service que necesitamos (el contexto
// del día). Interfaz para testear el servicio sin la DB del dashboard.
type snapshotter interface {
	Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*dashboard.Snapshot, error)
}

// insightStore es la porción de store.Queries que usamos (cache diario).
type insightStore interface {
	GetInsight(ctx context.Context, arg store.GetInsightParams) (store.AiInsight, error)
	CreateInsight(ctx context.Context, arg store.CreateInsightParams) (store.AiInsight, error)
}

// Service genera y cachea el insight proactivo del día.
type Service struct {
	dash   snapshotter
	store  insightStore
	groq   Completer
	hasKey bool
}

// NewService inyecta el dashboard (contexto), el store (cache), el Completer
// (Groq o fake) y si hay clave configurada.
func NewService(dash snapshotter, q insightStore, c Completer, hasKey bool) *Service {
	return &Service{dash: dash, store: q, groq: c, hasKey: hasKey}
}

// DailyInsight devuelve el insight de hoy: lee cache, o lo genera vía Groq y lo
// persiste. Degrada a Available:false sin clave o ante fallo de Groq; solo
// propaga error si fallan Snapshot o la DB (problema real, no de IA).
func (s *Service) DailyInsight(ctx context.Context, userID uuid.UUID, today time.Time) (*Insight, error) {
	// 1. Cache.
	row, err := s.store.GetInsight(ctx, store.GetInsightParams{
		UserID: userID, InsightDate: today, Kind: kindProactive,
	})
	if err == nil {
		return &Insight{Content: row.Content, Available: true, GeneratedAt: row.GeneratedAt}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// 2. Sin clave → degradado, sin tocar Groq ni persistir.
	if !s.hasKey {
		return &Insight{Available: false}, nil
	}

	// 3. Contexto del día.
	snap, err := s.dash.Snapshot(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	ctxJSON, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}

	// 4-5. Prompt + Groq. Fallo de IA → degradado (no rompe el dashboard).
	system, user := buildPrompt(string(ctxJSON))
	content, err := s.groq.Complete(ctx, system, user)
	if err != nil {
		return &Insight{Available: false}, nil
	}

	// 6. Persistir y devolver.
	created, err := s.store.CreateInsight(ctx, store.CreateInsightParams{
		UserID:          userID,
		InsightDate:     today,
		Kind:            kindProactive,
		Content:         content,
		ContextSnapshot: ctxJSON,
	})
	if err != nil {
		return nil, err
	}
	return &Insight{Content: created.Content, Available: true, GeneratedAt: created.GeneratedAt}, nil
}
