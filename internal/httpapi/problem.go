package httpapi

import (
	"net/http"
)

// problem is an RFC 9457 problem details object.
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// writeProblem writes an application/problem+json error response.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	writeBody(w, status, "application/problem+json", problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}

// router wraps a ServeMux so that its built-in 404 and 405 responses, which
// are plain text, are sent as problem+json instead.
type router struct {
	mux *http.ServeMux
}

func (rt router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, pattern := rt.mux.Handler(r)
	if pattern != "" {
		rt.mux.ServeHTTP(w, r)
		return
	}

	// No route matched. Let the mux decide between 404 and 405 (and the
	// Allow header), but discard its plain-text body.
	cw := &captureWriter{header: http.Header{}, status: http.StatusOK}
	h.ServeHTTP(cw, r)
	if allow := cw.header.Get("Allow"); allow != "" {
		w.Header().Set("Allow", allow)
	}
	writeProblem(w, cw.status, "")
}

// captureWriter records the status and headers a handler sets and discards
// its body.
type captureWriter struct {
	header http.Header
	status int
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) { return len(b), nil }
func (c *captureWriter) WriteHeader(status int)      { c.status = status }
