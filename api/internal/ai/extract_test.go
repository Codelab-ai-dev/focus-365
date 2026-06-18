package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readTestdata lee testdata/<name> y falla el test si no existe.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("readTestdata(%s): %v", name, err)
	}
	return data
}

// fakeExtractClient implementa la interfaz que usa el extractor para llamar a Groq.
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

func TestExtractCSVMovements(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[
		{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-06-10"},
		{"type":"income","amount_centavos":500000,"category":"sueldo"}]}`}
	ex := newExtractor(gc)
	res, err := ex.extract(context.Background(), []byte("fecha,monto,desc\n..."), "text/csv", "x.csv")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(res.actions) != 2 || res.dropped != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestExtractDropsInvalid(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[
		{"type":"expense","amount_centavos":25000,"category":"comida"},
		{"type":"transfer","amount_centavos":1,"category":"x"},
		{"type":"expense","amount_centavos":0,"category":"y"}]}`}
	ex := newExtractor(gc)
	res, err := ex.extract(context.Background(), []byte("..."), "text/csv", "x.csv")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(res.actions) != 1 || res.dropped != 2 {
		t.Errorf("res = %+v (esperaba 1 válido, 2 descartados)", res)
	}
}

func TestExtractZeroValidIsError(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[{"type":"transfer","amount_centavos":1,"category":"x"}]}`}
	ex := newExtractor(gc)
	if _, err := ex.extract(context.Background(), []byte("..."), "text/csv", "x.csv"); err == nil {
		t.Error("cero válidos debe ser error")
	}
}

func TestExtractImageUsesVision(t *testing.T) {
	gc := &fakeExtractClient{out: `{"movimientos":[{"type":"expense","amount_centavos":25000,"category":"comida"}]}`}
	ex := newExtractor(gc)
	if _, err := ex.extract(context.Background(), []byte{0x89, 0x50}, "image/png", "r.png"); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !gc.gotImage {
		t.Error("imagen debe ir por ExtractVision")
	}
}

func TestExtractUnsupportedType(t *testing.T) {
	ex := newExtractor(&fakeExtractClient{})
	if _, err := ex.extract(context.Background(), []byte("x"), "application/zip", "x.zip"); err == nil {
		t.Error("tipo no soportado debe fallar")
	}
}

func TestPdfTextFromSample(t *testing.T) {
	// sample.txt.pdf contiene texto extraíble; verificar que pdfText lo lee.
	data := readTestdata(t, "sample.txt.pdf")
	txt, err := pdfText(data)
	if err != nil {
		t.Fatalf("pdfText: %v", err)
	}
	if strings.TrimSpace(txt) == "" {
		t.Error("pdfText devolvió vacío para un PDF con texto")
	}
}

func TestPdfEmptyIsScannedError(t *testing.T) {
	// Un PDF sin texto debe dar el error de "escaneado" en extract.
	ex := newExtractor(&fakeExtractClient{out: `{"movimientos":[]}`})
	_, err := ex.extract(context.Background(), []byte("%PDF-1.4 sin texto real"), "application/pdf", "s.pdf")
	if err == nil {
		t.Error("PDF ilegible/escaneado debe fallar con mensaje claro")
	}
}

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
