# OCR de PDFs escaneados — Plan de implementación (Rebanada 30)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el importador de Finanzas procese PDFs escaneados extrayendo la imagen embebida de cada página (hasta 5) y mandándola al modelo de visión de Groq, juntando los movimientos.

**Architecture:** Un helper `pdfImages` (Go puro, `pdfcpu`) extrae las imágenes embebidas JPEG/PNG por página. La rama PDF de `extract.go` usa el texto si existe, y si el PDF está escaneado (sin texto) extrae las imágenes y las pasa a `ExtractVision`, acumulando los movimientos. Sin CGO ni cambios en la imagen de Docker.

**Tech Stack:** Go (paquete `ai`), `github.com/pdfcpu/pdfcpu` (pure Go), Groq vision (ya integrado).

**Contexto del repo (leer antes de empezar):**
- Comandos Go: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`; tests con `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"` y `-p 1`.
- `internal/ai/extract.go`: `extract(ctx, data, mime, filename)` rutea por tipo. Rama PDF (líneas ~56-65): `pdfText(data)` → si vacío/err → error "el PDF parece escaneado o ilegible; súbelo como foto"; si hay texto → `ExtractText`. Tras el switch, si `err != nil` → `ErrUnavailable`; parsea `{"movimientos":[...]}`, valida con `parseMovimientoLenient`, cuenta `dropped`, y "0 movimientos → no pude leer…". Constantes `maxTextChars=12000`.
- `internal/ai/pdftext.go`: `pdfText(data) (string, error)` (texto plano; "" si escaneado; recupera panics).
- `extractClient` (interfaz): `ExtractText(ctx, system, user)` y `ExtractVision(ctx, system, b64, mime)`. `imageMime(mime, filename)` mapea a `image/jpeg`/`image/png`. `extractSystemPrompt` es el system prompt de extracción.
- Tests del extractor: `internal/ai/extract_test.go` (`package ai`), helper `readTestdata(t, name)`, y el fake `fakeExtractClient{out, err, gotImage}` con `ExtractText`/`ExtractVision`. `internal/ai/testdata/` tiene `sample.csv` y `sample.txt.pdf` (PDF **con texto**).
- `pdfcpu` **no** está en `go.mod` todavía (solo `github.com/ledongthuc/pdf`).

---

## Estructura de archivos

- Crear `api/internal/ai/pdfimages.go` — `pdfImages` (extracción de imágenes embebidas, Go puro).
- Crear `api/internal/ai/pdfimages_test.go` — unit de `pdfImages` + helpers `tinyJPEG`/`buildScannedPDF`.
- Modificar `api/internal/ai/extract.go` — rama PDF: texto-o-visión; acumular movimientos.
- Modificar `api/internal/ai/extract_test.go` — fake con multi-llamada + tests de la rama escaneada.
- Modificar `api/go.mod` / `api/go.sum` — dependencia `pdfcpu`.

---

## Task 1: `pdfImages` (extracción de imágenes embebidas) + tests

**Files:**
- Create: `api/internal/ai/pdfimages.go`
- Create: `api/internal/ai/pdfimages_test.go`
- Modify: `api/go.mod`, `api/go.sum`

- [ ] **Step 1: Agregar la dependencia y descubrir la API de extracción**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go get github.com/pdfcpu/pdfcpu@latest
```
Luego inspeccioná la API de imágenes en memoria (los nombres pueden variar por versión):
```bash
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go doc github.com/pdfcpu/pdfcpu/pkg/api | grep -i -E "image|import"
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go doc github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model Image
```
Necesitás: (a) una función que **extraiga imágenes en memoria** (sin escribir a
disco) devolviendo, por imagen, su **lector de bytes**, su **tipo/formato**
(jpg/png/tif…) y el **número de página** — típicamente `api.ExtractImagesRaw(rs,
selectedPages, conf)`; y (b) `api.ImportImages(rs, w, imgs, imp, conf)` para armar
el fixture en los tests. Anotá las firmas reales y adaptá el código de abajo si
difieren.

- [ ] **Step 2: Tests (que fallan)**

Crear `api/internal/ai/pdfimages_test.go` (`package ai`):

```go
package ai

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// tinyJPEG genera un JPEG chiquito en memoria (para fixtures de PDF escaneado).
func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 32), G: uint8(y * 32), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// buildScannedPDF arma un PDF de `pages` páginas, cada una con el JPEG embebido
// (simula un PDF escaneado). Sin capa de texto.
func buildScannedPDF(t *testing.T, pages int) []byte {
	t.Helper()
	jp := tinyJPEG(t)
	readers := make([]io.Reader, pages)
	for i := range readers {
		readers[i] = bytes.NewReader(jp)
	}
	var out bytes.Buffer
	// api.ImportImages(rs=nil para PDF nuevo, w, imgs, imp=nil default, conf=nil default)
	if err := api.ImportImages(nil, &out, readers, nil, nil); err != nil {
		t.Fatalf("ImportImages: %v", err)
	}
	return out.Bytes()
}

func TestPdfImagesExtractsEmbedded(t *testing.T) {
	pdf := buildScannedPDF(t, 2)
	imgs, err := pdfImages(pdf, 5)
	if err != nil {
		t.Fatalf("pdfImages: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("imágenes = %d, want 2", len(imgs))
	}
	for i, im := range imgs {
		if im.mime != "image/jpeg" || len(im.bytes) == 0 {
			t.Errorf("img %d = mime %q, %d bytes", i, im.mime, len(im.bytes))
		}
	}
}

func TestPdfImagesRespectsMaxPages(t *testing.T) {
	pdf := buildScannedPDF(t, 4)
	imgs, err := pdfImages(pdf, 2)
	if err != nil {
		t.Fatalf("pdfImages: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("con maxPages=2 sobre 4 páginas: %d, want 2", len(imgs))
	}
}

func TestPdfImagesTextPdfHasNone(t *testing.T) {
	// sample.txt.pdf es un PDF de texto, sin imágenes embebidas.
	imgs, err := pdfImages(readTestdata(t, "sample.txt.pdf"), 5)
	if err != nil {
		t.Fatalf("pdfImages texto: %v", err)
	}
	if len(imgs) != 0 {
		t.Fatalf("PDF de texto: %d imágenes, want 0", len(imgs))
	}
}

func TestPdfImagesInvalidBytes(t *testing.T) {
	if _, err := pdfImages([]byte("no soy un pdf"), 5); err == nil {
		t.Fatal("esperaba error con bytes que no son PDF")
	}
}
```

- [ ] **Step 3: Implementar `api/internal/ai/pdfimages.go`**

Adaptá los nombres de la API de pdfcpu a los reales descubiertos en el Step 1. Base:

```go
package ai

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

const maxScannedPages = 5

// pdfPageImage es una imagen embebida extraída de una página del PDF.
type pdfPageImage struct {
	bytes []byte
	mime  string
}

// pdfImages extrae, por cada una de las primeras maxPages páginas, la imagen
// embebida más grande, solo si es JPEG o PNG (lo que acepta la visión). Go puro
// (pdfcpu). Recupera de panics de la librería. Lista vacía si no hay imágenes
// usables; error si el PDF es inválido/ilegible.
func pdfImages(data []byte, maxPages int) (out []pdfPageImage, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf ilegible: %v", r)
		}
	}()

	// Extracción en memoria de todas las imágenes (verificá el nombre real:
	// api.ExtractImagesRaw o equivalente). Cada item trae lector, tipo y página.
	imgs, xerr := api.ExtractImagesRaw(bytes.NewReader(data), nil, nil)
	if xerr != nil {
		return nil, fmt.Errorf("no pude leer imágenes del pdf: %w", xerr)
	}

	type cand struct {
		b    []byte
		mime string
	}
	best := map[int]cand{} // mejor imagen por página

	for _, im := range imgs {
		// Adaptá los nombres de campo a model.Image: tipo (FileType/Ext) y página (PageNr).
		mime := imageMimeFromType(im.FileType)
		if mime == "" {
			continue // formato no soportado por la visión (tif/ccitt/jbig2…)
		}
		b, rerr := io.ReadAll(im.Reader)
		if rerr != nil || len(b) == 0 {
			continue
		}
		if cur, ok := best[im.PageNr]; !ok || len(b) > len(cur.b) {
			best[im.PageNr] = cand{b: b, mime: mime}
		}
	}

	pages := make([]int, 0, len(best))
	for p := range best {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	for _, p := range pages {
		if len(out) >= maxPages {
			break
		}
		out = append(out, pdfPageImage{bytes: best[p].b, mime: best[p].mime})
	}
	return out, nil
}

// imageMimeFromType mapea el tipo de imagen de pdfcpu al mime que acepta la visión.
func imageMimeFromType(ft string) string {
	switch strings.ToLower(strings.TrimPrefix(ft, ".")) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	}
	return ""
}
```

> Si `ExtractImagesRaw` devuelve `map[int][]model.Image` (por página) en vez de un
> slice plano, ajustá el recorrido (la página es la clave del map). Si el campo del
> formato se llama distinto (`Ext`, `Type`…), usalo en `imageMimeFromType`.

- [ ] **Step 4: Verde**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go test ./internal/ai/ -run TestPdfImages -v`
Expected: los 4 PASS. Iterá hasta verde ajustando a la API real de pdfcpu.

- [ ] **Step 5: tidy + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go mod tidy
git add api/internal/ai/pdfimages.go api/internal/ai/pdfimages_test.go api/go.mod api/go.sum
git commit -m "feat(ai): extracción de imágenes embebidas de PDF (pdfImages, pdfcpu)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Rama PDF escaneado → visión, en `extract.go`

**Files:**
- Modify: `api/internal/ai/extract.go`
- Modify: `api/internal/ai/extract_test.go`

- [ ] **Step 1: Refactor de `extract()` para acumular movimientos**

Reemplazar el cuerpo de `func (e *extractor) extract(...)` (el switch + el parseo de
una sola `raw`) por la versión que acumula `[]json.RawMessage` de una o varias
llamadas, y mete la rama PDF-escaneado:

```go
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
```

> Mantené los imports existentes de `extract.go` (`base64`, `json`, `fmt`, `strings`,
> `time`, `context`). No se agregan imports nuevos (pdfImages/maxScannedPages están
> en el mismo paquete).

- [ ] **Step 2: Fake con multi-llamada (en extract_test.go)**

Reemplazar el `fakeExtractClient` por la versión que soporta varias llamadas de
visión con salidas distintas (para el test multipágina), conservando el campo `out`
para los tests existentes:

```go
type fakeExtractClient struct {
	out         string
	err         error
	gotImage    bool
	visionCalls int
	visionOuts  []string // si no está vacío, ExtractVision devuelve visionOuts[i] por llamada
}

func (f *fakeExtractClient) ExtractText(ctx context.Context, system, user string) (string, error) {
	return f.out, f.err
}
func (f *fakeExtractClient) ExtractVision(ctx context.Context, system, b64, mime string) (string, error) {
	f.gotImage = true
	i := f.visionCalls
	f.visionCalls++
	if f.err != nil {
		return "", f.err
	}
	if i < len(f.visionOuts) {
		return f.visionOuts[i], nil
	}
	return f.out, f.err
}
```

- [ ] **Step 3: Tests de la rama escaneada (que fallan)**

Agregar en `extract_test.go` (usan `buildScannedPDF` de la Task 1, mismo paquete):

```go
func TestExtractScannedPDFViaVision(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[
		{"type":"expense","amount_centavos":1200,"category":"café"}]}`}
	ex := newExtractor(gc)
	pdf := buildScannedPDF(t, 1)
	res, err := ex.extract(context.Background(), pdf, "application/pdf", "scan.pdf")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !gc.gotImage {
		t.Error("no usó la visión para el PDF escaneado")
	}
	if len(res.actions) != 1 {
		t.Fatalf("acciones = %d, want 1", len(res.actions))
	}
}

func TestExtractScannedPDFMergesPages(t *testing.T) {
	gc := &fakeExtractClient{visionOuts: []string{
		`{"movimientos":[{"type":"expense","amount_centavos":1000,"category":"a"}]}`,
		`{"movimientos":[{"type":"income","amount_centavos":2000,"category":"b"}]}`,
		`{"movimientos":[{"type":"expense","amount_centavos":3000,"category":"c"}]}`,
	}}
	ex := newExtractor(gc)
	pdf := buildScannedPDF(t, 3)
	res, err := ex.extract(context.Background(), pdf, "application/pdf", "scan.pdf")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if gc.visionCalls != 3 {
		t.Errorf("visionCalls = %d, want 3", gc.visionCalls)
	}
	if len(res.actions) != 3 {
		t.Fatalf("acciones (juntadas) = %d, want 3", len(res.actions))
	}
}

func TestExtractScannedNoImageFallback(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[]}`}
	ex := newExtractor(gc)
	// bytes que no son un PDF con imagen → pdfText vacío/err y pdfImages error/vacío
	_, err := ex.extract(context.Background(), []byte("no soy un pdf"), "application/pdf", "x.pdf")
	if err == nil || !strings.Contains(err.Error(), "súbelo como foto") {
		t.Fatalf("esperaba el fallback 'súbelo como foto', got %v", err)
	}
	if gc.gotImage {
		t.Error("no debería llamar a visión cuando no hay imagen")
	}
}
```

> `strings` ya está importado en `extract_test.go`.

- [ ] **Step 4: Verificar el paquete + build + suite**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde (incluidos los tests existentes del extractor: imagen, CSV, PDF-texto, dropped).

- [ ] **Step 5: Commit**

```bash
git add api/internal/ai/extract.go api/internal/ai/extract_test.go
git commit -m "feat(ai): importador procesa PDFs escaneados vía visión (hasta 5 páginas)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Cierre — review, merge y smoke

**Files:** verificación + `scripts/smoke-r30.sh` + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-30-ocr-pdf-escaneado-design.md`. Verificar: el camino PDF-texto no cambió de comportamiento; el escaneado-con-imagen llama visión y junta; el escaneado-sin-imagen mantiene el fallback; sin CGO (el binario sigue compilando con `CGO_ENABLED=0`). Aplicar nits.

- [ ] **Step 2: Confirmar build sin CGO** (la imagen de Docker usa `CGO_ENABLED=0`):
```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" CGO_ENABLED=0 GOOS=linux go build -o /tmp/focus-server ./cmd/server && echo "build CGO=0 OK"
```
Y la suite completa verde (Task 2 Step 4).

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + smoke.** Crear `scripts/smoke-r30.sh`: registra un usuario y **sube un PDF escaneado** (un PDF chico con una imagen embebida — el script puede generarlo con un PDF de prueba incluido en base64, o documentar que se prueba a mano subiendo un comprobante real). Verificá que `POST /ai/import` con ese PDF responde 200 con `created` no vacío (la visión real extrae). Si generar el PDF en bash es complejo, el smoke valida lo mínimo end-to-end posible y la verificación fuerte queda en los tests + una prueba manual subiendo un comprobante escaneado real desde la UI. (Recordatorio: si el deploy falla en build con el código compilando local → disco lleno del VPS → `docker system prune -af`.)

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-30-ocr-pdf-escaneado.md`.

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 `pdfImages` (Go puro, imagen embebida más grande por página, JPEG/PNG, hasta
  maxPages, recover) → Task 1. ✓
- §2 rama PDF de `extract.go` (texto-o-visión; acumular movimientos; fallback sin
  imagen) + `maxScannedPages` → Task 2. ✓
- §3 dependencia pdfcpu (pure Go, sin CGO) → Task 1 (go get + go mod tidy); build
  CGO=0 verificado en Task 3. ✓
- §4 errores (texto igual; escaneado-imagen→visión; escaneado-sin-imagen→fallback;
  Groq→503; 0 movimientos→422) → Task 2. ✓
- §5 testing (pdfImages unit con fixture generado; extract escaneado 1-pág,
  multipágina, fallback; no-regresión) → Tasks 1–2. ✓
- §6 aceptación → Task 3. ✓

**Placeholders:** los puntos «verificá la firma real de pdfcpu (`go doc`)» son
adaptaciones deterministas a la librería, con el comando exacto para descubrirla;
no son TODOs de diseño. El smoke (§Task 3 Step 4) reconoce que generar un PDF
escaneado en bash es opcional y deja la verificación fuerte en los tests + prueba
manual — no es un placeholder, es un alcance explícito.

**Consistencia de tipos/firmas:** `pdfImages(data, maxPages) ([]pdfPageImage,
error)` con `pdfPageImage{bytes, mime}` y `maxScannedPages=5` (Task 1) ↔ uso en
`extract.go` (Task 2). El fake `fakeExtractClient` mantiene `out`/`err`/`gotImage`
(tests viejos) y suma `visionCalls`/`visionOuts` (multipágina). `buildScannedPDF`/
`tinyJPEG` definidos en Task 1, reusados en Task 2 (mismo `package ai`).
`imageMimeFromType` (jpg/png→mime) nuevo; `imageMime` existente se mantiene para el
camino de imagen directa. ✓
