package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestRequestBodyChecks(t *testing.T) {
	h := newAPI(t)
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantDetail string
	}{
		{"unknown field", strings.Replace(validDishJSON, `"name"`, `"colour":"red","name"`, 1), http.StatusBadRequest, `unknown field "colour"`},
		{"trailing data", validDishJSON + `{}`, http.StatusBadRequest, "unexpected data"},
		{"too large", `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`, http.StatusRequestEntityTooLarge, "at most 64 KiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, shackDishes, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if p := decode[problem](t, rec); !strings.Contains(p.Detail, tt.wantDetail) {
				t.Errorf("detail = %q, want it to contain %q", p.Detail, tt.wantDetail)
			}
		})
	}
}
