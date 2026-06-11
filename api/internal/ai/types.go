package ai

import "time"

// Insight es la vista que devuelve el servicio. Cuando Available es false (sin
// clave o fallo de IA), Content queda vacío y el handler serializa
// content/generated_at como null.
type Insight struct {
	Content     string    `json:"content"`
	Available   bool      `json:"available"`
	GeneratedAt time.Time `json:"generated_at"`
}
