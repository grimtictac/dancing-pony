package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

func TestListDishesOptions(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)
	shack := addRestaurant(t, openDB(t, db.OwnerURL), "orc-shack", "ZAR")
	s := openStore(t, db.AppURL)
	for _, d := range []struct {
		name, desc string
		price      int64
	}{
		{"Seed Cake", "A honey cake with seeds.", 8500},
		{"Mushroom Pie", "A savoury pie with woodland mushrooms.", 12000},
		{"Rabbit Stew", "Slow-cooked coney with taters.", 9500},
	} {
		if _, err := s.CreateDish(ctx, shack, dish.Dish{
			Name: d.name, Description: d.desc, Image: "https://example.com/x.jpg",
			Price: dish.Money{Amount: d.price, Currency: "ZAR"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	names := func(opts store.ListOptions) []string {
		t.Helper()
		list, _, err := s.ListDishes(ctx, shack, opts)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range list {
			out = append(out, d.Name)
		}
		return out
	}

	tests := []struct {
		name string
		opts store.ListOptions
		want []string
	}{
		{"default: oldest first", store.ListOptions{}, []string{"Seed Cake", "Mushroom Pie", "Rabbit Stew"}},
		{"full-text, stemmed", store.ListOptions{Search: "mushrooms"}, []string{"Mushroom Pie"}},
		{"full-text matches descriptions", store.ListOptions{Search: "honey"}, []string{"Seed Cake"}},
		{"typo in the name", store.ListOptions{Search: "mushrom"}, []string{"Mushroom Pie"}},
		{"price range", store.ListOptions{MinPrice: 9000, MaxPrice: 10000}, []string{"Rabbit Stew"}},
		{"sort by price, descending", store.ListOptions{Sort: "-price"}, []string{"Mushroom Pie", "Rabbit Stew", "Seed Cake"}},
		{"sort by name", store.ListOptions{Sort: "name"}, []string{"Mushroom Pie", "Rabbit Stew", "Seed Cake"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := names(tt.opts); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("pages follow the cursor to the end", func(t *testing.T) {
		var got []string
		opts := store.ListOptions{Sort: "price", Limit: 2}
		for page := 0; ; page++ {
			list, next, err := s.ListDishes(ctx, shack, opts)
			if err != nil || page > 3 {
				t.Fatalf("page %d: %v", page, err)
			}
			for _, d := range list {
				got = append(got, d.Name)
			}
			if next == "" {
				break
			}
			opts.Cursor = next
		}
		if want := []string{"Seed Cake", "Rabbit Stew", "Mushroom Pie"}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	for name, opts := range map[string]store.ListOptions{
		"unknown sort":          {Sort: "rating; DROP TABLE dishes"},
		"limit too large":       {Limit: store.MaxLimit + 1},
		"garbage cursor":        {Cursor: "not-a-cursor"},
		"negative price filter": {MinPrice: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := s.ListDishes(ctx, shack, opts); !errors.Is(err, store.ErrInvalidListOptions) {
				t.Errorf("err = %v, want ErrInvalidListOptions", err)
			}
		})
	}
}
