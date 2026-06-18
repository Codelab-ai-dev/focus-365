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
