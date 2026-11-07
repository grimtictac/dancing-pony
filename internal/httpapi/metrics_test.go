package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsUseRoutePatterns(t *testing.T) {
	m := NewMetrics(prometheus.NewRegistry())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /things/{id}", func(http.ResponseWriter, *http.Request) {})
	h := accessLog(slog.New(slog.DiscardHandler), m, router{mux: mux})

	for _, path := range []string{"/things/1", "/things/2", "/nope", "/also-nope"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	}

	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "GET /things/{id}", "200")); got != 2 {
		t.Errorf("requests for the pattern = %v, want 2 (both paths share one series)", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "unmatched", "404")); got != 2 {
		t.Errorf("unmatched 404s = %v, want 2", got)
	}
}
