package httpapi

import (
	"net/http"
	"slices"
	"testing"

	"the-dancing-pony-v2/internal/dish"
)

// TestTenantIsolation checks, over HTTP, that green-dragon can't reach
// orc-shack's dishes through any tenant route, and can't even tell that
// they exist: a dish of another restaurant gets exactly the response of a
// dish that doesn't exist at all.
func TestTenantIsolation(t *testing.T) {
	const (
		shack  = "/restaurants/orc-shack/dishes"
		dragon = "/restaurants/green-dragon/dishes"
	)
	h := newAPI(t)
	cake := dishNamed(t, h, "orc-shack", "Seed Cake")
	shackBefore := listDishes(t, h, shack)

	covered := map[string]bool{}

	// First, while the dish certainly exists.
	covered["GET /restaurants/{slug}/dishes"] = true
	t.Run("GET /restaurants/{slug}/dishes", func(t *testing.T) {
		for _, d := range listDishes(t, h, dragon) {
			if d.ID == cake.ID {
				t.Errorf("green-dragon's menu includes orc-shack's %s", cake.Name)
			}
		}
	})

	// Routes that address one dish: another restaurant's dish must look
	// exactly like a missing one.
	for _, tt := range []struct {
		pattern, method, suffix, body string
		// The attacker: green-dragon's owner for dish writes (they may
		// manage dishes, just not orc-shack's), a customer for the rest.
		user string
	}{
		{"GET /restaurants/{slug}/dishes/{id}", http.MethodGet, "", "", "frodo"},
		{"PUT /restaurants/{slug}/dishes/{id}", http.MethodPut, "", validDishJSON, "dragon-owner"},
		{"DELETE /restaurants/{slug}/dishes/{id}", http.MethodDelete, "", "", "dragon-owner"},
		{"GET /restaurants/{slug}/dishes/{id}/reviews", http.MethodGet, "/reviews", "", "frodo"},
		{"PUT /restaurants/{slug}/dishes/{id}/reviews/me", http.MethodPut, "/reviews/me", validReviewJSON, "frodo"},
		{"DELETE /restaurants/{slug}/dishes/{id}/reviews/me", http.MethodDelete, "/reviews/me", "", "frodo"},
	} {
		covered[tt.pattern] = true
		t.Run(tt.pattern, func(t *testing.T) {
			missing := doAs(t, h, tt.user, tt.method, dragon+"/"+unknownDishID+tt.suffix, tt.body)
			got := doAs(t, h, tt.user, tt.method, dragon+"/"+cake.ID+tt.suffix, tt.body)

			if got.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", got.Code, http.StatusNotFound)
			}
			if got.Body.String() != missing.Body.String() {
				t.Errorf("body = %s, want the same as for a missing dish: %s", got.Body, missing.Body)
			}
		})
	}

	covered["POST /restaurants/{slug}/dishes"] = true
	t.Run("POST /restaurants/{slug}/dishes", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, dragon, validDishJSON)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		created := decode[dish.Dish](t, rec)
		if rec := do(t, h, http.MethodGet, shack+"/"+created.ID, ""); rec.Code != http.StatusNotFound {
			t.Errorf("green-dragon's new dish is visible at orc-shack: status %d", rec.Code)
		}
	})

	t.Run("orc-shack is unchanged", func(t *testing.T) {
		if after := listDishes(t, h, shack); !slices.Equal(after, shackBefore) {
			t.Errorf("orc-shack's dishes = %+v, want unchanged %+v", after, shackBefore)
		}
	})

	t.Run("every tenant route is covered", func(t *testing.T) {
		for _, rt := range (&api{}).tenantRoutes() {
			if !covered[rt.pattern] {
				t.Errorf("route %q has no tenant-isolation test", rt.pattern)
			}
		}
	})
}

func listDishes(t *testing.T, h http.Handler, path string) []dish.Dish {
	t.Helper()
	rec := do(t, h, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rec.Code)
	}
	return decode[[]dish.Dish](t, rec)
}
