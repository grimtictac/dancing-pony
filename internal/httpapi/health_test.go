package httpapi

import (
	"net/http"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	h := newAPI(t)
	for _, path := range []string{"/livez", "/readyz", "/health"} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, rec.Code)
		}
	}
}
