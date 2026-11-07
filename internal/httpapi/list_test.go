package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/dish"
)

func TestListDishesQuery(t *testing.T) {
	h := newAPI(t)

	rec := do(t, h, http.MethodGet, shackDishes+"?q=mushrom", "")
	if got := decode[[]dish.Dish](t, rec); len(got) != 1 || got[0].Name != "Mushroom Pie" {
		t.Errorf("q=mushrom: got %+v, want Mushroom Pie", got)
	}

	rec = do(t, h, http.MethodGet, shackDishes+"?sort=-price&limit=1", "")
	if got := decode[[]dish.Dish](t, rec); len(got) != 1 || got[0].Name != "Mushroom Pie" {
		t.Errorf("first page: got %+v, want Mushroom Pie", got)
	}
	link := rec.Header().Get("Link")
	if !strings.HasPrefix(link, "</restaurants/orc-shack/dishes?") || !strings.HasSuffix(link, `>; rel="next"`) {
		t.Fatalf("Link = %q, want a rel=next link", link)
	}
	next := strings.TrimSuffix(strings.TrimPrefix(link, "<"), `>; rel="next"`)
	rec = do(t, h, http.MethodGet, next, "")
	if got := decode[[]dish.Dish](t, rec); len(got) != 1 || got[0].Name != "Seed Cake" {
		t.Errorf("second page: got %+v, want Seed Cake", got)
	}
	if rec.Header().Get("Link") != "" {
		t.Errorf("last page has a Link header")
	}

	for _, query := range []string{"?sort=bogus", "?limit=abc", "?min_price=-5", "?cursor=xyz"} {
		if rec := do(t, h, http.MethodGet, shackDishes+query, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", query, rec.Code)
		}
	}
}
