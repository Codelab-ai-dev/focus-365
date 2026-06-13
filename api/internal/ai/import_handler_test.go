package ai_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
)

// postImport arma una petición multipart con un único campo "file" y la sirve.
func postImport(t *testing.T, h http.Handler, tok, filename, ctype string, body []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	hdr.Set("Content-Type", ctype)
	part, _ := mw.CreatePart(hdr)
	part.Write(body)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/ai/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func getPending(t *testing.T, h http.Handler, tok string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ai/import/pending", nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestImportCSVHappy(t *testing.T) {
	comp := &fakeCompleter{}
	e := newEnv(t, true, comp)
	e.extract.out = `{"movimientos":[
		{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-06-10"},
		{"type":"income","amount_centavos":500000,"category":"sueldo"}]}`
	_, tok := e.user(t, "imp@b.com")

	csv := []byte("fecha,monto,desc\n2026-06-10,250,almuerzo\n")
	rec, body := postImport(t, e.h, tok, "movs.csv", "text/csv", csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	created, _ := body["created"].([]any)
	if len(created) != 2 {
		t.Fatalf("created = %d, want 2 (body %v)", len(created), body)
	}
	if body["dropped"] != float64(0) {
		t.Errorf("dropped = %v, want 0", body["dropped"])
	}
	for _, c := range created {
		m := c.(map[string]any)
		if m["status"] != "proposed" || m["kind"] != "movimiento" {
			t.Errorf("acción creada inesperada: %v", m)
		}
	}

	// GET /ai/import/pending debe listar los dos movimientos recién creados.
	rec2, body2 := getPending(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("pending code = %d", rec2.Code)
	}
	acts, _ := body2["actions"].([]any)
	if len(acts) != 2 {
		t.Errorf("pending actions = %d, want 2", len(acts))
	}
}

func TestImportUnsupportedType(t *testing.T) {
	comp := &fakeCompleter{}
	e := newEnv(t, true, comp)
	e.extract.out = `{"movimientos":[]}`
	_, tok := e.user(t, "unsup@b.com")

	rec, body := postImport(t, e.h, tok, "x.zip", "application/zip", []byte("PK..."))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422 (body %v)", rec.Code, body)
	}
	if body["error"] == nil {
		t.Errorf("esperaba mensaje de error, got %v", body)
	}
}

func TestImportRequiresAuth(t *testing.T) {
	comp := &fakeCompleter{}
	e := newEnv(t, true, comp)
	e.extract.out = `{"movimientos":[{"type":"expense","amount_centavos":1,"category":"x"}]}`

	rec, _ := postImport(t, e.h, "", "movs.csv", "text/csv", []byte("a,b\n1,2\n"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestImportNoKeyUnavailable(t *testing.T) {
	comp := &fakeCompleter{}
	e := newEnv(t, false, comp)
	e.extract.out = `{"movimientos":[{"type":"expense","amount_centavos":1,"category":"x"}]}`
	_, tok := e.user(t, "nokeyimp@b.com")

	rec, body := postImport(t, e.h, tok, "movs.csv", "text/csv", []byte("a,b\n1,2\n"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503 (body %v)", rec.Code, body)
	}
}
