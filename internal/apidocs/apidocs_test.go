package apidocs

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestDocsPage(t *testing.T) {
	rec := get(t, "/docs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self'", "connect-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe") {
		t.Error("CSP allows inline or eval'd scripts")
	}
	body := rec.Body.String()
	for _, src := range []string{"/docs/" + scalarFile, "/docs/docs.js"} {
		if !strings.Contains(body, src) {
			t.Errorf("page doesn't load %s", src)
		}
	}
}

func TestSpecIsServed(t *testing.T) {
	rec := get(t, "/openapi.yaml", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "openapi: 3.1") {
		t.Fatalf("status %d, body starts %.20q", rec.Code, rec.Body.String())
	}
}

func TestScalarBundle(t *testing.T) {
	gz := get(t, "/docs/"+scalarFile, "gzip, br")
	if gz.Code != http.StatusOK || gz.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip: status %d, encoding %q", gz.Code, gz.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(gz.Body)
	if err != nil {
		t.Fatal(err)
	}
	unzipped, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}

	plain := get(t, "/docs/"+scalarFile, "")
	if plain.Header().Get("Content-Encoding") != "" || plain.Body.String() != string(unzipped) {
		t.Fatal("a client without gzip didn't get the same script, uncompressed")
	}
	if !strings.Contains(plain.Body.String(), "createApiReference") {
		t.Fatal("bundle isn't Scalar's standalone build")
	}
}

func TestUnknownAssetIsNotFound(t *testing.T) {
	if rec := get(t, "/docs/evil.js", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}
