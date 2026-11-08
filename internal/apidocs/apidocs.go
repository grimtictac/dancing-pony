// Package apidocs serves the API's OpenAPI description and interactive
// documentation (Scalar) at /openapi.yaml and /docs.
//
// Everything is embedded in the binary, so the docs deploy with the code and
// the page loads nothing from third parties. Scalar is vendored as a pinned,
// gzipped bundle (MIT licence in scalar-LICENSE); to update it, download
// dist/browser/standalone.js for the new version from the npm package
// @scalar/api-reference, gzip it under the new name and change scalarFile.
package apidocs

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"net/http"
	"strings"
)

// Spec is the OpenAPI document. Tests check it lists exactly the routes the
// API serves.
//
//go:embed openapi.yaml
var Spec []byte

//go:embed index.html docs.js scalar-1.77.0.js.gz
var files embed.FS

const scalarFile = "scalar-1.77.0.js"

// The docs page runs only scripts from this origin, talks only to this
// origin ("Try it" requests), and can't be framed. Scalar sets inline
// styles, hence 'unsafe-inline' for styles (not scripts).
const docsCSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Register adds the documentation routes to mux. They need no login.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.yaml", serveSpec)
	mux.HandleFunc("GET /docs", servePage)
	mux.HandleFunc("GET /docs/{file}", serveAsset)
}

func serveSpec(w http.ResponseWriter, _ *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "application/yaml; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=300")
	h.Set("Access-Control-Allow-Origin", "*") // so other tools can load it
	_, _ = w.Write(Spec)
}

func servePage(w http.ResponseWriter, _ *http.Request) {
	page, _ := files.ReadFile("index.html")
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", docsCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}

func serveAsset(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	switch r.PathValue("file") {
	case "docs.js":
		js, _ := files.ReadFile("docs.js")
		h.Set("Cache-Control", "no-cache")
		_, _ = w.Write(js)
	case scalarFile:
		// The name carries the version, so browsers may keep it for good.
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		h.Set("Vary", "Accept-Encoding")
		gz, _ := files.ReadFile(scalarFile + ".gz")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.Set("Content-Encoding", "gzip")
			_, _ = w.Write(gz)
			return
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			http.Error(w, "docs unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = io.Copy(w, zr) //nolint:gosec // G110: our own embedded file, not user input
	default:
		h.Del("Content-Type")
		http.NotFound(w, r)
	}
}
