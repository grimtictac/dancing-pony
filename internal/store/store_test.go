package store_test

import (
	"context"
	"errors"
	"testing"

	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

func TestRestaurantBySlug(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)
	owner := openDB(t, db.OwnerURL)
	if _, err := owner.ExecContext(ctx,
		`INSERT INTO restaurants (slug, name) VALUES ('orc-shack', 'The Orc Shack')`); err != nil {
		t.Fatal(err)
	}
	s := openStore(t, db.AppURL)

	r, err := s.RestaurantBySlug(ctx, "orc-shack")
	if err != nil {
		t.Fatal(err)
	}
	if r.Slug != "orc-shack" || r.Name != "The Orc Shack" || r.Currency != "ZAR" {
		t.Errorf("got %+v, want orc-shack / The Orc Shack / ZAR", r)
	}
	if r.ID.Version() != 7 {
		t.Errorf("id %s is UUID version %d, want 7", r.ID, r.ID.Version())
	}

	if _, err := s.RestaurantBySlug(ctx, "green-dragon"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown slug: err = %v, want ErrNotFound", err)
	}
}

func TestAppRoleCannotChangeRestaurants(t *testing.T) {
	app := openDB(t, storetest.Migrated(t).AppURL)
	_, err := app.ExecContext(context.Background(),
		`INSERT INTO restaurants (slug, name) VALUES ('rogue', 'Rogue')`)
	if err == nil {
		t.Fatal("pony_app inserted a restaurant, want permission denied")
	}
}

func TestRestaurantSlugMustBeURLSafe(t *testing.T) {
	owner := openDB(t, storetest.Migrated(t).OwnerURL)
	for _, slug := range []string{"Orc-Shack", "orc shack", "-orc", "orc-", "orc--shack", "", "orc/shack"} {
		_, err := owner.ExecContext(context.Background(),
			`INSERT INTO restaurants (slug, name) VALUES ($1, 'x')`, slug)
		if err == nil {
			t.Errorf("slug %q was accepted, want a check violation", slug)
		}
	}
}

func openStore(t *testing.T, dsn string) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}
