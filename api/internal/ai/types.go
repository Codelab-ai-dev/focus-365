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

// ActionView es una acción del mensaje (propuesta o resuelta).
type ActionView struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Status  string          `json:"status"`
}

// ThreadView es la vista de un hilo en la lista (se serializa a JSON).
type ThreadView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message es un mensaje del chat (vista que se serializa a JSON).
type Message struct {
	ID        string       `json:"id"`
	Role      string       `json:"role"`
	Content   string       `json:"content"`
	Actions   []ActionView `json:"actions,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}
