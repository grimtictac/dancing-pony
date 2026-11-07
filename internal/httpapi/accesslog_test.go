package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		info(r.Context()).Restaurant = "orc-shack"
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		internalError(w, r, errors.New("database on fire"))
	})
	h := accessLog(logger, router{mux: mux})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/things/42?secret=token", nil)
	req.Header.Set(requestIDHeader, "trace-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", nil))

	if got := rec.Header().Get(requestIDHeader); got != "trace-123" {
		t.Errorf("response X-Request-ID = %q, want the caller's", got)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2:\n%s", len(lines), buf.String())
	}
	var put, boom map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &put)
	_ = json.Unmarshal([]byte(lines[1]), &boom)

	want := map[string]any{
		"level": "INFO", "request_id": "trace-123", "method": "PUT", "route": "PUT /things/{id}",
		"path": "/things/42", "status": float64(201), "bytes": float64(2), "restaurant": "orc-shack",
	}
	for k, v := range want {
		if put[k] != v {
			t.Errorf("PUT line %s = %v, want %v", k, put[k], v)
		}
	}
	if strings.Contains(lines[0], "secret") {
		t.Error("query string was logged")
	}
	if boom["level"] != "ERROR" || boom["err"] != "database on fire" || boom["request_id"] == "" {
		t.Errorf("500 line = %v, want ERROR with err and a generated request_id", boom)
	}
}
