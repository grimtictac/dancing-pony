package store_test

import (
	"context"
	"testing"

	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)

	first, err := store.Seed(ctx, db.OwnerURL, store.DemoRestaurants)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range first {
		if want := len(store.DemoRestaurants[i].Dishes); r.DishesAdded != want {
			t.Errorf("first seed added %d dishes to %s, want %d", r.DishesAdded, r.Slug, want)
		}
	}

	second, err := store.Seed(ctx, db.OwnerURL, store.DemoRestaurants)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range second {
		if r.DishesAdded != 0 {
			t.Errorf("second seed added %d dishes to %s, want 0", r.DishesAdded, r.Slug)
		}
	}

	s := openStore(t, db.AppURL)
	for _, sr := range store.DemoRestaurants {
		r, err := s.RestaurantBySlug(ctx, sr.Slug)
		if err != nil {
			t.Fatalf("%s: %v", sr.Slug, err)
		}
		dishes, err := s.ListDishes(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(dishes) != len(sr.Dishes) {
			t.Errorf("%s has %d dishes, want %d", sr.Slug, len(dishes), len(sr.Dishes))
		}
	}
}
