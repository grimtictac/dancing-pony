package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// The Evil Orc guesses Frodo's password, sending a different spoofed
// X-Forwarded-For each time. No proxy is trusted, so every attempt counts
// against his real address, and he's throttled: even the right password
// then gets 429 until the delay passes.
func TestLoginThrottlingResistsSpoofedIPs(t *testing.T) {
	h := newAPI(t)
	attempt := func(i int, password string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/login",
			strings.NewReader(`{"email":"frodo@shire.example","password":"`+password+`"}`))
		req.RemoteAddr = "203.0.113.66:5000"
		req.Header.Set("X-Forwarded-For", "10.0.0."+strconv.Itoa(i))
		rec := httptest.NewRecorder()
		h.raw.ServeHTTP(rec, req)
		return rec
	}

	for i := range 6 { // 5 free failures for this email+IP, then a delay
		if rec := attempt(i, "wrong guess number "+strconv.Itoa(i)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: status %d, want 401", i, rec.Code)
		}
	}
	rec := attempt(99, "pony-demo-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after 6 failures: status %d, want 429", rec.Code)
	}
	if ra, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || ra < 1 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", rec.Header().Get("Retry-After"))
	}

	// Frodo, from his own address, is unaffected by the per-pair limit.
	if rec := do(t, h.raw, http.MethodPost, "/auth/login", `{"email":"frodo@shire.example","password":"pony-demo-password"}`); rec.Code != http.StatusOK {
		t.Errorf("Frodo from another IP: status %d, want 200", rec.Code)
	}
}
