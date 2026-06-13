package ai

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// pdfText extrae el texto plano de un PDF en memoria. Devuelve "" (sin error)
// si el PDF no tiene texto extraíble (escaneado). Recupera de panics de la lib.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf ilegible: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf inválido: %w", err)
	}
	rd, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("pdf sin texto: %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rd); err != nil {
		return "", err
	}
	return buf.String(), nil
}
