package finance

import (
	"context"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

type Service struct {
	q *store.Queries
}

func NewService(q *store.Queries) *Service {
	return &Service{q: q}
}

// Input son los datos de dominio para crear una transacción manual.
type Input struct {
	Type       string
	Amount     int64 // centavos
	OccurredOn time.Time
	Category   string
	Remark     string
}

// Transaction es la vista de dominio que se serializa a JSON. occurred_on va
// como YYYY-MM-DD y cycle como YYYY-MM.
type Transaction struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Amount     int64     `json:"amount"`
	OccurredOn string    `json:"occurred_on"`
	Cycle      string    `json:"cycle"`
	Category   string    `json:"category"`
	Remark     string    `json:"remark"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CycleSummary resume un ciclo: totales en centavos, net (ingresos − gastos,
// sin transferencias) y status (pendiente | verde | rojo).
type CycleSummary struct {
	Cycle   string `json:"cycle"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
	Net     int64  `json:"net"`
	Status  string `json:"status"`
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in Input) (*Transaction, error) {
	row, err := s.q.CreateTransaction(ctx, store.CreateTransactionParams{
		UserID:     userID,
		Type:       in.Type,
		Amount:     in.Amount,
		OccurredOn: in.OccurredOn,
		Cycle:      Cycle(in.OccurredOn),
		Category:   in.Category,
		Remark:     in.Remark,
	})
	if err != nil {
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

func (s *Service) ListByCycle(ctx context.Context, userID uuid.UUID, cycle time.Time) ([]Transaction, error) {
	rows, err := s.q.ListTransactionsByCycle(ctx, store.ListTransactionsByCycleParams{UserID: userID, Cycle: cycle})
	if err != nil {
		return nil, err
	}
	out := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		out = append(out, toView(row))
	}
	return out, nil
}

// Delete borra la transacción si pertenece al usuario; devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteTransaction(ctx, store.DeleteTransactionParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) Summary(ctx context.Context, userID uuid.UUID, cycle, now time.Time) (*CycleSummary, error) {
	row, err := s.q.SummarizeCycle(ctx, store.SummarizeCycleParams{UserID: userID, Cycle: cycle})
	if err != nil {
		return nil, err
	}
	sum := CycleSummary{
		Cycle:   FormatCycle(cycle),
		Income:  row.Income,
		Expense: row.Expense,
		Net:     row.Income - row.Expense,
	}
	sum.Status = statusFor(cycle, Cycle(now), sum.Net)
	return &sum, nil
}

func (s *Service) Cycles(ctx context.Context, userID uuid.UUID, now time.Time) ([]CycleSummary, error) {
	rows, err := s.q.SummarizeCycles(ctx, userID)
	if err != nil {
		return nil, err
	}
	current := Cycle(now)
	out := make([]CycleSummary, 0, len(rows))
	for _, row := range rows {
		net := row.Income - row.Expense
		out = append(out, CycleSummary{
			Cycle:   FormatCycle(row.Cycle),
			Income:  row.Income,
			Expense: row.Expense,
			Net:     net,
			Status:  statusFor(row.Cycle, current, net),
		})
	}
	return out, nil
}

// statusFor decide el estado de un ciclo: si es el actual o futuro está
// "pendiente"; si ya cerró, "verde" cuando hubo superávit y "rojo" si no.
func statusFor(cycle, current time.Time, net int64) string {
	if !cycle.Before(current) { // cycle >= current → en curso
		return "pendiente"
	}
	if net >= 0 {
		return "verde"
	}
	return "rojo"
}

func toView(row store.Transaction) Transaction {
	return Transaction{
		ID:         row.ID.String(),
		Type:       row.Type,
		Amount:     row.Amount,
		OccurredOn: row.OccurredOn.Format(dateLayout),
		Cycle:      FormatCycle(row.Cycle),
		Category:   row.Category,
		Remark:     row.Remark,
		Source:     row.Source,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
