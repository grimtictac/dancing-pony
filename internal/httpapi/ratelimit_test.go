package httpapi

import (
	"net/http"
	"testing"
)

func TestPerCustomerRateLimit(t *testing.T) {
	// A burst of 3 and a refill too slow to matter during the test.
	a := newAPI(t, WithUserRateLimit(0.001, 3))
	const menu = "/restaurants/orc-shack/dishes"

	for i := range 3 {
		if rec := doAs(t, a, "frodo", http.MethodGet, menu, ""); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i+1, rec.Code)
		}
	}
	rec := doAs(t, a, "frodo", http.MethodGet, menu, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
	// The limit is per customer: Sam isn't slowed by Frodo.
	if rec := doAs(t, a, "sam", http.MethodGet, menu, ""); rec.Code != http.StatusOK {
		t.Fatalf("other customer: status %d, want 200", rec.Code)
	}
	// Anonymous requests are refused as unauthenticated before any limit.
	if rec := doAs(t, a, "", http.MethodGet, menu, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: status %d, want 401", rec.Code)
	}
}
