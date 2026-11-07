package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/dish"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, path, nil)
	} else {
		req = httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newSeededAPI() http.Handler {
	return New(dish.NewMemoryStore(dish.Seed()...))
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestHealth(t *testing.T) {
	rec := do(t, newSeededAPI(), http.MethodGet, "/health", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := decode[map[string]string](t, rec); got["status"] != "ok" {
		t.Errorf("body = %v, want status ok", got)
	}
}

func TestSeedDishPriceIsInMinorUnits(t *testing.T) {
	rec := do(t, newSeededAPI(), http.MethodGet, "/dishes/1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := dish.Money{Amount: 8500, Currency: "ZAR"}
	if got := decode[dish.Dish](t, rec).Price; got != want {
		t.Errorf("price = %+v, want %+v", got, want)
	}
}

func TestCreateDishAcceptsMinorUnitPrice(t *testing.T) {
	body := `{"name":"Lembas","description":"Elven waybread.","price":{"amount":12500,"currency":"ZAR"},"image":"https://example.com/lembas.jpg"}`
	rec := do(t, newSeededAPI(), http.MethodPost, "/dishes", body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	want := dish.Money{Amount: 12500, Currency: "ZAR"}
	if got := decode[dish.Dish](t, rec).Price; got != want {
		t.Errorf("price = %+v, want %+v", got, want)
	}
}

func TestCreateDishRejectsInvalidPrice(t *testing.T) {
	tests := []struct {
		name  string
		price string
	}{
		{"float price", `8.5`},
		{"fractional amount", `{"amount":8.5,"currency":"ZAR"}`},
		{"zero amount", `{"amount":0,"currency":"ZAR"}`},
		{"negative amount", `{"amount":-100,"currency":"ZAR"}`},
		{"missing currency", `{"amount":8500}`},
		{"unsupported currency", `{"amount":8500,"currency":"GBP"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"name":"Stew","description":"Rabbit stew.","price":` + tt.price + `,"image":"https://example.com/stew.jpg"}`
			rec := do(t, newSeededAPI(), http.MethodPost, "/dishes", body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestUpdateThenDeleteDish(t *testing.T) {
	h := newSeededAPI()
	body := `{"name":"Seed Cake","description":"Now with extra seeds.","price":{"amount":9000,"currency":"ZAR"},"image":"https://example.com/seed-cake.jpg"}`

	rec := do(t, h, http.MethodPut, "/dishes/1", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if got := decode[dish.Dish](t, rec); got.ID != "1" || got.Price.Amount != 9000 {
		t.Errorf("updated dish = %+v, want ID 1 and amount 9000", got)
	}

	if rec := do(t, h, http.MethodDelete, "/dishes/1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec := do(t, h, http.MethodGet, "/dishes/1", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUnknownDishIsNotFound(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			if rec := do(t, newSeededAPI(), method, "/dishes/999", ""); rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		method, path string
		wantStatus   int
		wantAllow    string // checked only for 405 responses
	}{
		{http.MethodGet, "/", http.StatusOK, ""},
		{http.MethodGet, "/health", http.StatusOK, ""},
		{http.MethodGet, "/dishes", http.StatusOK, ""},
		{http.MethodGet, "/dishes/1", http.StatusOK, ""},
		{http.MethodGet, "/nope", http.StatusNotFound, ""},
		{http.MethodGet, "/dishes/1/extra", http.StatusNotFound, ""},
		{http.MethodPost, "/", http.StatusMethodNotAllowed, "GET, HEAD"},
		{http.MethodPost, "/health", http.StatusMethodNotAllowed, "GET, HEAD"},
		{http.MethodDelete, "/dishes", http.StatusMethodNotAllowed, "GET, HEAD, POST"},
		{http.MethodPost, "/dishes/1", http.StatusMethodNotAllowed, "DELETE, GET, HEAD, PUT"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := do(t, newSeededAPI(), tt.method, tt.path, "")

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusMethodNotAllowed {
				if got := rec.Header().Get("Allow"); got != tt.wantAllow {
					t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
				}
			}
		})
	}
}
