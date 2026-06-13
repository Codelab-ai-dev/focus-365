package ai

import (
	"context"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// uploadStore es lo que ImportService necesita del store (fakeable).
type uploadStore interface {
	CreateUploadActions(ctx context.Context, userID uuid.UUID, actions []ProposedAction) ([]store.AiAction, error)
	ListPendingUploadActions(ctx context.Context, userID uuid.UUID) ([]store.AiAction, error)
}

// ImportService extrae movimientos de un archivo y los persiste como acciones
// de upload propuestas.
type ImportService struct {
	ex     *extractor
	store  uploadStore
	hasKey bool
}

func NewImportService(c extractClient, st uploadStore, hasKey bool) *ImportService {
	return &ImportService{ex: newExtractor(c), store: st, hasKey: hasKey}
}

// ImportResult es la vista del resultado de una importación.
type ImportResult struct {
	Created   []ActionView
	Dropped   int
	Truncated bool
}

func (s *ImportService) Import(ctx context.Context, userID uuid.UUID, data []byte, mime, filename string) (*ImportResult, error) {
	if !s.hasKey {
		return nil, ErrUnavailable
	}
	res, err := s.ex.extract(ctx, data, mime, filename)
	if err != nil {
		return nil, err // errores "de negocio" (escaneado, cero, formato) → el handler los mapea
	}
	rows, err := s.store.CreateUploadActions(ctx, userID, res.actions)
	if err != nil {
		return nil, err
	}
	out := &ImportResult{Dropped: res.dropped, Truncated: res.truncated}
	for _, r := range rows {
		out.Created = append(out.Created, toActionView(r))
	}
	return out, nil
}

func (s *ImportService) Pending(ctx context.Context, userID uuid.UUID) ([]ActionView, error) {
	rows, err := s.store.ListPendingUploadActions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ActionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActionView(r))
	}
	return out, nil
}
