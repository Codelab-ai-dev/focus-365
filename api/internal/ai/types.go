package ai

import (
	"encoding/json"
	"time"
)

// Insight es la vista que devuelve el servicio. Cuando Available es false (sin
// clave o fallo de IA), Content queda vacío y el handler serializa
// content/generated_at como null.
type Insight struct {
	Content     string    `json:"content"`
	Available   bool      `json:"available"`
	GeneratedAt time.Time `json:"generated_at"`
}

// ActionView es la acción propuesta/resuelta embebida en un mensaje.
type ActionView struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Status  string          `json:"status"`
}

// Message es un mensaje del chat (vista que se serializa a JSON).
type Message struct {
	ID        string      `json:"id"`
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	Action    *ActionView `json:"action,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}
