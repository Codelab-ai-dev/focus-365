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

// fakeExtractor implementa la interfaz que usa el extractor para llamar a Groq.
type fakeExtractClient struct {
	out      string
	err      error
	gotImage bool
}

func (f *fakeExtractClient) ExtractText(ctx context.Context, system, user string) (string, error) {
	return f.out, f.err
}
func (f *fakeExtractClient) ExtractVision(ctx context.Context, system, b64, mime string) (string, error) {
	f.gotImage = true
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
