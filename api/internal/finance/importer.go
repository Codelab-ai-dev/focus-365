package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// ExternalTx es la representación normalizada de una transacción traída del
// servicio externo, lista para insertarse.
type ExternalTx struct {
	ExternalID string
	Type       string // income | expense | transfer
	Amount     int64  // centavos
	OccurredOn time.Time
	Category   string
	Remark     string
}

// Import inserta (idempotentemente por external_id) un lote de transacciones
// externas para un usuario, calculando el ciclo de cada una. Devuelve cuántas
// procesó.
func (s *Service) Import(ctx context.Context, userID uuid.UUID, txs []ExternalTx) (int, error) {
	n := 0
	for _, tx := range txs {
		ext := tx.ExternalID
		_, err := s.q.UpsertImportedTransaction(ctx, store.UpsertImportedTransactionParams{
			UserID:     userID,
			Type:       tx.Type,
			Amount:     tx.Amount,
			OccurredOn: tx.OccurredOn,
			Cycle:      Cycle(tx.OccurredOn),
			Category:   tx.Category,
			Remark:     tx.Remark,
			ExternalID: &ext,
		})
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ImportConfig configura el cliente del servicio externo (money.quhou123.com).
type ImportConfig struct {
	BaseURL string
	Token   string
	From    string // YYYY-MM-DD
	To      string // YYYY-MM-DD
}

// externalResponse refleja la forma ASUMIDA de la respuesta de getTransactions.
// AJUSTAR cuando se disponga de una muestra real del servicio.
type externalResponse struct {
	Data []struct {
		ID       string `json:"id"`
		Kind     string `json:"type"`   // "income" | "expense" | "transfer"
		Amount   int64  `json:"amount"` // se asume en centavos
		Date     string `json:"date"`   // YYYY-MM-DD
		Category string `json:"category"`
		Remark   string `json:"remark"`
	} `json:"data"`
}

// FetchTransactions llama al servicio externo y normaliza la respuesta. El
// mapeo de campos es provisional (ver externalResponse).
func FetchTransactions(ctx context.Context, cfg ImportConfig) ([]ExternalTx, error) {
	u, err := url.Parse(cfg.BaseURL + "/getTransactions")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("from", cfg.From)
	q.Set("to", cfg.To)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getTransactions devolvió %d", res.StatusCode)
	}

	var body externalResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]ExternalTx, 0, len(body.Data))
	for _, item := range body.Data {
		day, err := time.Parse(dateLayout, item.Date)
		if err != nil {
			return nil, fmt.Errorf("fecha inválida %q: %w", item.Date, err)
		}
		out = append(out, ExternalTx{
			ExternalID: item.ID,
			Type:       item.Kind,
			Amount:     item.Amount,
			OccurredOn: day,
			Category:   item.Category,
			Remark:     item.Remark,
		})
	}
	return out, nil
}
