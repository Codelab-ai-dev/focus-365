# Plan 30 — OCR de PDFs escaneados (Finanzas) — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

El importador de Finanzas (`POST /ai/import`) hoy, ante un **PDF escaneado**
(imagen pura, sin capa de texto), falla con "el PDF parece escaneado o ilegible;
súbelo como foto". Esta rebanada cierra ese hueco: extrae la **imagen embebida de
cada página** del PDF y la manda al **modelo de visión de Groq** (el mismo que ya
se usa para fotos jpg/png), juntando los movimientos de hasta **5 páginas**.

**Decisiones (brainstorming):**
- **Hasta 5 páginas**, juntando los movimientos de todas.
- **Extracción de imagen embebida** (Go puro, `pdfcpu`), no rasterización — así
  no se agrega CGO ni binarios del sistema y la imagen de Docker no cambia.
- Solo imágenes **JPEG/PNG** (las que la visión acepta); otros formatos de scan se
  omiten y caen al fallback actual.

**Fuera de alcance:** rasterizar PDFs vectoriales o "texto-como-imagen" que no
embeben una imagen (requeriría un renderizador/CGO); OCR local (Tesseract); más de
5 páginas; cifrado de PDFs protegidos.

**Limitación reconocida:** funciona para el caso común (sacaste foto / escaneaste y
guardaste como PDF → el PDF embebe un JPEG por página). PDFs con scans en formatos
exóticos (CCITT G4 fax, JBIG2) o sin imagen embebida **no** se procesan; se mantiene
el mensaje "súbelo como foto".

## 2. Arquitectura (backend, paquete `ai`)

### Nuevo: `pdfImages` (Go puro, `pdfcpu`)
`func pdfImages(data []byte, maxPages int) ([]pdfPageImage, error)` donde
`pdfPageImage = { bytes []byte; mime string }`.
- Recorre las primeras `maxPages` páginas del PDF y, por cada una, extrae la
  **imagen embebida más grande** (por tamaño en bytes).
- Devuelve solo las imágenes **JPEG o PNG** (las que `ExtractVision` acepta);
  cualquier otro formato (TIFF/CCITT, JBIG2, etc.) se descarta.
- Recupera de panics de la librería (como `pdfText`); ante PDF inválido devuelve
  error. Si no hay ninguna imagen usable, devuelve una lista vacía (sin error).

### `extract.go` — rama PDF
Hoy: `pdfText` → si vacío, error "súbelo como foto"; si hay texto, `ExtractText`.
Nuevo:
1. `pdfText(data)` → si hay texto (no vacío tras trim) → `ExtractText` (igual).
2. Si **vacío** (escaneado) → `imgs := pdfImages(data, maxScannedPages)`:
   - Si `len(imgs) == 0` → error "el PDF parece escaneado o ilegible; súbelo como
     foto" (fallback actual).
   - Si hay imágenes → por cada una, `ExtractVision(extractSystemPrompt,
     base64(img.bytes), img.mime)`; se **acumulan** los movimientos de todas.
3. Un fallo de red/HTTP de Groq en cualquier llamada → `ErrUnavailable` (503).

### Refactor para acumular movimientos
`extract()` hoy asume **una sola** respuesta del modelo (`raw` → `parsed.Movimientos`).
Se refactoriza para acumular `[]json.RawMessage` de **una o varias** llamadas, y
recién después correr el bucle de validación existente (`parseMovimientoLenient`,
`dropped`, "0 movimientos → error"). Las demás ramas (imagen, CSV, PDF-texto)
producen una sola llamada → una sola lista; la rama PDF-escaneado, varias.

- Constante nueva `maxScannedPages = 5`.
- El resto (validación, `dropped`, `truncated`, "0 movimientos → no pude leer…")
  no cambia.

## 3. Dependencia

`github.com/pdfcpu/pdfcpu` — Go puro (sin CGO, sin binarios del sistema). Se agrega
a `go.mod`/`go.sum`. La imagen de Docker (`golang:1.23-alpine` build,
`CGO_ENABLED=0`) no cambia.

## 4. Manejo de errores / bordes

- PDF con capa de texto → camino de texto (sin cambios).
- PDF escaneado con imagen embebida JPEG/PNG → visión (nuevo); junta hasta 5
  páginas.
- PDF escaneado sin imagen usable (formato raro, cifrado, sin imagen) → fallback
  "súbelo como foto".
- Fallo de Groq (cualquiera de las llamadas de visión) → `ErrUnavailable` → 503.
- Cero movimientos tras procesar todas las páginas → "no pude leer movimientos en
  el archivo" (422), igual que hoy.
- El tope de 8 MB del upload (handler) sigue vigente; un PDF de 5 páginas con fotos
  puede acercarse — el límite ya rechaza >8 MB con 413.
- `pdfImages` recupera de panics de la librería; un PDF corrupto → error de negocio
  con mensaje (no 500).

## 5. Testing

- **`pdfImages` (unit):** con un PDF que embebe un JPEG (fixture en `testdata/`)
  → devuelve 1 imagen con mime `image/jpeg` y bytes no vacíos; con un PDF de solo
  texto (o sin imagen) → lista vacía; con bytes que no son PDF → error.
- **`extract()` rama escaneado (con `extractClient` fake):** un PDF escaneado de 1
  página (pdfText vacío, pdfImages con imagen) → el fake `ExtractVision` recibe el
  base64 y devuelve movimientos → produce acciones; **multipágina** → se juntan los
  movimientos de las páginas; PDF escaneado **sin imagen usable** → error "súbelo
  como foto" (sin llamar a visión).
- **No-regresión:** los tests actuales de imagen, CSV y PDF-con-texto siguen verde.
- **Fixtures:** un PDF chico con un JPEG embebido y un PDF de solo texto en
  `internal/ai/testdata/`. Se pueden generar con `pdfcpu` en un helper de test o
  commitear como archivos.

## 6. Criterios de aceptación

- Subir un PDF escaneado (con imagen embebida) al importador extrae movimientos vía
  visión, juntando hasta 5 páginas; antes fallaba.
- Un PDF con texto sigue funcionando por el camino de texto.
- Un PDF escaneado sin imagen usable mantiene el mensaje claro "súbelo como foto".
- Sin CGO ni cambios en la imagen de Docker; suites en verde; smoke de producción
  OK.
