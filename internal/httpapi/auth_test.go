package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterLoginLogout(t *testing.T) {
	h := newAPI(t)

	rec := do(t, h.raw, http.MethodPost, "/auth/register", `{"email":"  Pippin@Shire.example ","password":"second breakfast"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status %d, body %s", rec.Code, rec.Body)
	}
	if got := decode[userResponse](t, rec); got.Email != "pippin@shire.example" {
		t.Errorf("email = %q, want it trimmed and lower-cased", got.Email)
	}
	if rec := do(t, h.raw, http.MethodPost, "/auth/register", `{"email":"pippin@shire.example","password":"another password"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate register: status %d, want 409", rec.Code)
	}

	token := login(t, h.raw, "PIPPIN@shire.example", "second breakfast")
	bearer := func(method, path string) int {
		req, _ := http.NewRequestWithContext(t.Context(), method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.raw.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := bearer(http.MethodGet, "/auth/me"); code != http.StatusOK {
		t.Errorf("me: status %d, want 200", code)
	}
	if code := bearer(http.MethodPost, "/auth/logout"); code != http.StatusNoContent {
		t.Errorf("logout: status %d, want 204", code)
	}
	if code := bearer(http.MethodGet, "/auth/me"); code != http.StatusUnauthorized {
		t.Errorf("me after logout: status %d, want 401", code)
	}
}

func TestRegisterValidation(t *testing.T) {
	h := newAPI(t)
	for name, body := range map[string]string{ //nolint:gosec // G101: test inputs, not credentials
		"short password":    `{"email":"a@b.example","password":"short"}`,
		"long password":     `{"email":"a@b.example","password":"` + strings.Repeat("x", 300) + `"}`,
		"not an email":      `{"email":"not-an-email","password":"long enough password"}`,
		"display-name form": `{"email":"Frodo <f@shire.example>","password":"long enough password"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if rec := do(t, h.raw, http.MethodPost, "/auth/register", body); rec.Code != http.StatusBadRequest {
				t.Errorf("status %d, want 400; body %s", rec.Code, rec.Body)
			}
		})
	}
}

// A wrong password and an unknown email must be indistinguishable.
func TestLoginFailuresLookAlike(t *testing.T) {
	h := newAPI(t)
	wrong := do(t, h.raw, http.MethodPost, "/auth/login", `{"email":"frodo@shire.example","password":"not the password"}`)
	unknown := do(t, h.raw, http.MethodPost, "/auth/login", `{"email":"gollum@misty.example","password":"not the password"}`)
	if wrong.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("statuses %d and %d, want 401 for both", wrong.Code, unknown.Code)
	}
	if wrong.Body.String() != unknown.Body.String() {
		t.Errorf("bodies differ:\n%s\n%s", wrong.Body, unknown.Body)
	}
}

func TestLoginRequired(t *testing.T) {
	h := newAPI(t)
	for name, header := range map[string]string{ //nolint:gosec // G101: test inputs, not credentials
		"no header":     "",
		"wrong scheme":  "Basic " + h.tokens["frodo"],
		"unknown token": "Bearer not-a-real-token",
	} {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, shackDishes, nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			h.raw.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
				t.Errorf("status %d, WWW-Authenticate %q; want 401 with the header", rec.Code, rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
	// Public endpoints stay public.
	for _, path := range []string{"/", "/livez", "/readyz", "/health"} {
		if rec := do(t, h.raw, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s anonymously: status %d, want 200", path, rec.Code)
		}
	}
}
