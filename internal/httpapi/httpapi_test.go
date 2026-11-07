package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

// testAPI is the API under test plus a session token for each demo user
// (store.DemoUsers, by name). Requests without an Authorization header
// are sent as orc-owner; use h.raw to send one with none.
type testAPI struct {
	raw    http.Handler
	tokens map[string]string
}

func (a *testAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		r.Header.Set("Authorization", "Bearer "+a.tokens["orc-owner"])
	}
	a.raw.ServeHTTP(w, r)
}

// newAPI returns the API on a fresh database seeded with the demo users
// and restaurants: orc-shack (Seed Cake, Mushroom Pie) and green-dragon.
func newAPI(t *testing.T) *testAPI {
	t.Helper()
	ctx := context.Background()
	db := storetest.Migrated(t)
	if err := store.SeedUsers(ctx, db.OwnerURL, store.DemoUsers, store.DemoPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Seed(ctx, db.OwnerURL, store.DemoRestaurants); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMemberships(ctx, db.OwnerURL, store.DemoUsers); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(ctx, db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := &testAPI{raw: New(s, slog.New(slog.DiscardHandler), nil, nil), tokens: map[string]string{}}
	for _, u := range store.DemoUsers {
		a.tokens[u.Name] = login(t, a.raw, u.Email, store.DemoPassword)
	}
	return a
}

// login logs in and returns the session token.
func login(t *testing.T, h http.Handler, email, password string) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/auth/login", `{"email":"`+email+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: status %d, body %s", email, rec.Code, rec.Body)
	}
	return decode[map[string]any](t, rec)["token"].(string)
}

// doAs sends a request as the named demo user ("" for no user at all).
func doAs(t *testing.T, h *testAPI, user, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+h.tokens[user])
	}
	rec := httptest.NewRecorder()
	h.raw.ServeHTTP(rec, req)
	return rec
}

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

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// dishNamed returns the restaurant's dish with the given name.
func dishNamed(t *testing.T, h http.Handler, slug, name string) dish.Dish {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/restaurants/"+slug+"/dishes", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list %s dishes: status %d", slug, rec.Code)
	}
	for _, d := range decode[[]dish.Dish](t, rec) {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("%s has no dish named %q", slug, name)
	return dish.Dish{}
}

const (
	shackDishes = "/restaurants/orc-shack/dishes"
	// A well-formed ID that no dish has.
	unknownDishID = "0000000000000000000001"
)

func TestHealth(t *testing.T) {
	rec := do(t, newAPI(t), http.MethodGet, "/health", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := decode[map[string]string](t, rec); got["status"] != "ok" {
		t.Errorf("body = %v, want status ok", got)
	}
}

func TestListDishes(t *testing.T) {
	rec := do(t, newAPI(t), http.MethodGet, shackDishes, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	got := decode[[]dish.Dish](t, rec)
	if len(got) != 2 || got[0].Name != "Seed Cake" || got[1].Name != "Mushroom Pie" {
		t.Errorf("dishes = %+v, want Seed Cake then Mushroom Pie", got)
	}
}

func TestSeedDishPriceIsInMinorUnits(t *testing.T) {
	h := newAPI(t)
	cake := dishNamed(t, h, "orc-shack", "Seed Cake")
	rec := do(t, h, http.MethodGet, shackDishes+"/"+cake.ID, "")

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
	rec := do(t, newAPI(t), http.MethodPost, shackDishes, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	got := decode[dish.Dish](t, rec)
	if want := (dish.Money{Amount: 12500, Currency: "ZAR"}); got.Price != want {
		t.Errorf("price = %+v, want %+v", got.Price, want)
	}
	if len(got.ID) != 22 {
		t.Errorf("id = %q, want a 22-character base62 ID", got.ID)
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
		{"not the restaurant's currency", `{"amount":8500,"currency":"GBP"}`},
	}
	h := newAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"name":"Stew","description":"Rabbit stew.","price":` + tt.price + `,"image":"https://example.com/stew.jpg"}`
			rec := do(t, h, http.MethodPost, shackDishes, body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestUpdateThenDeleteDish(t *testing.T) {
	h := newAPI(t)
	cake := dishNamed(t, h, "orc-shack", "Seed Cake")
	path := shackDishes + "/" + cake.ID
	body := `{"name":"Seed Cake","description":"Now with extra seeds.","price":{"amount":9000,"currency":"ZAR"},"image":"https://example.com/seed-cake.jpg"}`

	rec := do(t, h, http.MethodPut, path, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if got := decode[dish.Dish](t, rec); got.ID != cake.ID || got.Price.Amount != 9000 {
		t.Errorf("updated dish = %+v, want ID %s and amount 9000", got, cake.ID)
	}

	if rec := do(t, h, http.MethodDelete, path, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec := do(t, h, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUnknownDishIsNotFound(t *testing.T) {
	h := newAPI(t)
	for _, dishID := range []string{unknownDishID, "1", "not-a-valid-id-at-all!"} {
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			t.Run(method+" "+dishID, func(t *testing.T) {
				if rec := do(t, h, method, shackDishes+"/"+dishID, ""); rec.Code != http.StatusNotFound {
					t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
				}
			})
		}
	}
}

func TestUnknownRestaurantIsNotFound(t *testing.T) {
	rec := do(t, newAPI(t), http.MethodGet, "/restaurants/prancing-pony/dishes", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := decode[problem](t, rec).Detail; got != "restaurant not found" {
		t.Errorf("detail = %q, want restaurant not found", got)
	}
}

func TestRoutes(t *testing.T) {
	h := newAPI(t)
	cake := shackDishes + "/" + dishNamed(t, h, "orc-shack", "Seed Cake").ID
	tests := []struct {
		method, path string
		wantStatus   int
		wantAllow    string // checked only for 405 responses
	}{
		{http.MethodGet, "/", http.StatusOK, ""},
		{http.MethodGet, "/health", http.StatusOK, ""},
		{http.MethodGet, shackDishes, http.StatusOK, ""},
		{http.MethodGet, cake, http.StatusOK, ""},
		{http.MethodGet, "/nope", http.StatusNotFound, ""},
		{http.MethodGet, "/dishes", http.StatusNotFound, ""},
		{http.MethodGet, "/restaurants/orc-shack", http.StatusNotFound, ""},
		{http.MethodGet, cake + "/extra", http.StatusNotFound, ""},
		{http.MethodPost, "/", http.StatusMethodNotAllowed, "GET, HEAD"},
		{http.MethodPost, "/health", http.StatusMethodNotAllowed, "GET, HEAD"},
		{http.MethodDelete, shackDishes, http.StatusMethodNotAllowed, "GET, HEAD, POST"},
		{http.MethodPost, cake, http.StatusMethodNotAllowed, "DELETE, GET, HEAD, PUT"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, "")

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
