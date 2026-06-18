package ai

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	maxCSVRows   = 50
	maxTextChars = 12000
)

// extractClient es lo que el extractor necesita de Groq (fakeable).
type extractClient interface {
	ExtractText(ctx context.Context, system, user string) (string, error)
	ExtractVision(ctx context.Context, system, b64, mime string) (string, error)
}

type extractor struct {
	groq extractClient
}

func newExtractor(c extractClient) *extractor { return &extractor{groq: c} }

// extractResult: las acciones movimiento validadas + cuántas se descartaron +
// si la entrada se truncó (CSV largo).
type extractResult struct {
	actions   []ProposedAction
	dropped   int
	truncated bool
}

type extractedMovs struct {
	Movimientos []json.RawMessage `json:"movimientos"`
}

// extract detecta el tipo, obtiene el JSON del modelo y valida cada movimiento.
func (e *extractor) extract(ctx context.Context, data []byte, mime, filename string) (*extractResult, error) {
	var movs []json.RawMessage
	truncated := false

	// addFromRaw parsea una respuesta del modelo y acumula sus movimientos.
	addFromRaw := func(raw string) error {
		var parsed extractedMovs
		if jerr := json.Unmarshal([]byte(raw), &parsed); jerr != nil {
			return fmt.Errorf("respuesta del modelo no es JSON válido")
		}
		movs = append(movs, parsed.Movimientos...)
		return nil
	}

	switch {
	case mime == "image/jpeg" || mime == "image/png" || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg") || strings.HasSuffix(filename, ".png"):
		b64 := base64.StdEncoding.EncodeToString(data)
		raw, verr := e.groq.ExtractVision(ctx, extractSystemPrompt, b64, imageMime(mime, filename))
		if verr != nil {
			return nil, ErrUnavailable
		}
		if aerr := addFromRaw(raw); aerr != nil {
			return nil, aerr
		}

	case mime == "text/csv" || strings.HasSuffix(filename, ".csv"):
		text, tr := csvToText(data)
		truncated = tr
		raw, terr := e.groq.ExtractText(ctx, extractSystemPrompt, text)
		if terr != nil {
			return nil, ErrUnavailable
		}
		if aerr := addFromRaw(raw); aerr != nil {
			return nil, aerr
		}

	case mime == "application/pdf" || strings.HasSuffix(filename, ".pdf"):
		text, perr := pdfText(data)
		if perr == nil && strings.TrimSpace(text) != "" {
			// PDF con texto → camino de texto (como hoy).
			if len(text) > maxTextChars {
				text = text[:maxTextChars]
				truncated = true
			}
			raw, terr := e.groq.ExtractText(ctx, extractSystemPrompt, text)
			if terr != nil {
				return nil, ErrUnavailable
			}
			if aerr := addFromRaw(raw); aerr != nil {
				return nil, aerr
			}
		} else {
			// PDF escaneado → imágenes embebidas → visión (hasta maxScannedPages).
			imgs, ierr := pdfImages(data, maxScannedPages)
			if ierr != nil || len(imgs) == 0 {
				return nil, fmt.Errorf("el PDF parece escaneado o ilegible; súbelo como foto")
			}
			for _, img := range imgs {
				b64 := base64.StdEncoding.EncodeToString(img.bytes)
				raw, verr := e.groq.ExtractVision(ctx, extractSystemPrompt, b64, img.mime)
				if verr != nil {
					return nil, ErrUnavailable
				}
				if aerr := addFromRaw(raw); aerr != nil {
					return nil, aerr
				}
			}
		}

	default:
		return nil, fmt.Errorf("formato no soportado: %s", mime)
	}

	res := &extractResult{truncated: truncated}
	for _, m := range movs {
		payload, verr := parseMovimientoLenient(string(m))
		if verr != nil {
			res.dropped++
			continue
		}
		res.actions = append(res.actions, ProposedAction{Kind: "movimiento", Payload: payload})
	}
	if len(res.actions) == 0 {
		return nil, fmt.Errorf("no pude leer movimientos en el archivo")
	}
	return res, nil
}

// parseMovimientoLenient valida un movimiento con las MISMAS reglas que
// parseActionPayload("movimiento", ...) pero sin DisallowUnknownFields: el JSON
// del modelo extractor puede traer campos extra que no deben descartar un
// movimiento por lo demás válido. Se usa SOLO en el extractor.
func parseMovimientoLenient(args string) (json.RawMessage, error) {
	var p movimientoPayload
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return nil, err
	}
	if p.Type != "income" && p.Type != "expense" {
		return nil, fmt.Errorf("type debe ser income o expense")
	}
	if p.AmountCentavos < 1 {
		return nil, fmt.Errorf("amount_centavos debe ser positivo")
	}
	if strings.TrimSpace(p.Category) == "" {
		return nil, fmt.Errorf("falta category")
	}
	if p.OccurredOn != "" {
		if _, err := time.Parse("2006-01-02", p.OccurredOn); err != nil {
			return nil, fmt.Errorf("occurred_on inválido (YYYY-MM-DD)")
		}
	}
	return json.Marshal(p)
}

func imageMime(mime, filename string) string {
	if mime == "image/jpeg" || mime == "image/png" {
		return mime
	}
	if strings.HasSuffix(filename, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

// csvToText lee hasta maxCSVRows filas (más cabecera) y las re-serializa como
// texto para el prompt. Devuelve si truncó.
func csvToText(data []byte) (string, bool) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	var b strings.Builder
	rows := 0
	truncated := false
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if rows >= maxCSVRows+1 { // +1 por la cabecera
			truncated = true
			break
		}
		b.WriteString(strings.Join(rec, ","))
		b.WriteByte('\n')
		rows++
	}
	return b.String(), truncated
}
