package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestErrorsUseProblemJSON(t *testing.T) {
	const unknownDish = shackDishes + "/" + unknownDishID
	tests := []struct {
		name         string
		method, path string
		body         string
		wantStatus   int
		wantDetail   string // substring; empty means no detail is required
		wantAllow    string
	}{
		{"unknown route", http.MethodGet, "/nope", "", http.StatusNotFound, "", ""},
		{"unknown restaurant", http.MethodGet, "/restaurants/nope/dishes", "", http.StatusNotFound, "restaurant not found", ""},
		{"unknown dish", http.MethodGet, unknownDish, "", http.StatusNotFound, "dish not found", ""},
		{"update unknown dish", http.MethodPut, unknownDish, validDishJSON, http.StatusNotFound, "dish not found", ""},
		{"delete unknown dish", http.MethodDelete, unknownDish, "", http.StatusNotFound, "dish not found", ""},
		{"method not allowed", http.MethodDelete, shackDishes, "", http.StatusMethodNotAllowed, "", "GET, HEAD, POST"},
		{"malformed JSON", http.MethodPost, shackDishes, `{"name":`, http.StatusBadRequest, "invalid JSON", ""},
		{"validation failure", http.MethodPost, shackDishes, `{"description":"x","price":{"amount":1,"currency":"ZAR"},"image":"x"}`, http.StatusBadRequest, "name is required", ""},
	}
	h := newAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", ct)
			}
			if tt.wantAllow != "" {
				if got := rec.Header().Get("Allow"); got != tt.wantAllow {
					t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
				}
			}

			p := decode[problem](t, rec)
			want := problem{Type: "about:blank", Title: http.StatusText(tt.wantStatus), Status: tt.wantStatus}
			if p.Type != want.Type || p.Title != want.Title || p.Status != want.Status {
				t.Errorf("problem = %+v, want type/title/status %+v", p, want)
			}
			if !strings.Contains(p.Detail, tt.wantDetail) {
				t.Errorf("detail = %q, want it to contain %q", p.Detail, tt.wantDetail)
			}
		})
	}
}

const validDishJSON = `{"name":"Stew","description":"Rabbit stew.","price":{"amount":9500,"currency":"ZAR"},"image":"https://example.com/stew.jpg"}`
