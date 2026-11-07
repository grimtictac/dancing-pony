package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

func TestDishCRUD(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)
	shack := addRestaurant(t, openDB(t, db.OwnerURL), "orc-shack", "ZAR")
	s := openStore(t, db.AppURL)

	created, err := s.CreateDish(ctx, shack, lembas(4550))
	if err != nil {
		t.Fatal(err)
	}
	u, err := id.Decode(created.ID)
	if err != nil {
		t.Fatalf("created ID %q is not a base62 UUID: %v", created.ID, err)
	}
	if u.Version() != 7 {
		t.Errorf("dish UUID version = %d, want 7", u.Version())
	}
	if created.Name != "Lembas" || created.Price != (dish.Money{Amount: 4550, Currency: "ZAR"}) {
		t.Errorf("created = %+v", created)
	}

	if got, err := s.GetDish(ctx, shack, u); err != nil || got != created {
		t.Errorf("GetDish = %+v, %v; want %+v", got, err, created)
	}

	updated, err := s.UpdateDish(ctx, shack, u, lembas(4999))
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Price.Amount != 4999 {
		t.Errorf("updated = %+v, want same ID and amount 4999", updated)
	}

	if list, err := s.ListDishes(ctx, shack); err != nil || len(list) != 1 || list[0] != updated {
		t.Errorf("ListDishes = %+v, %v; want [%+v]", list, err, updated)
	}

	if err := s.DeleteDish(ctx, shack, u); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDish(ctx, shack, u); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetDish after delete: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteDish(ctx, shack, u); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second DeleteDish: err = %v, want ErrNotFound", err)
	}
}

func TestListDishesIsEmptyNotNil(t *testing.T) {
	db := storetest.Migrated(t)
	shack := addRestaurant(t, openDB(t, db.OwnerURL), "orc-shack", "ZAR")

	list, err := openStore(t, db.AppURL).ListDishes(context.Background(), shack)
	if err != nil || list == nil || len(list) != 0 {
		t.Errorf("ListDishes = %#v, %v; want an empty, non-nil slice", list, err)
	}
}

func TestDishesAreScopedToTheirRestaurant(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)
	owner := openDB(t, db.OwnerURL)
	shack := addRestaurant(t, owner, "orc-shack", "ZAR")
	dragon := addRestaurant(t, owner, "green-dragon", "ZAR")
	s := openStore(t, db.AppURL)

	d, err := s.CreateDish(ctx, shack, lembas(4550))
	if err != nil {
		t.Fatal(err)
	}
	dishID, _ := id.Decode(d.ID)

	if list, _ := s.ListDishes(ctx, dragon); len(list) != 0 {
		t.Errorf("green-dragon lists %d dishes, want 0", len(list))
	}
	if _, err := s.GetDish(ctx, dragon, dishID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("green-dragon GetDish: err = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateDish(ctx, dragon, dishID, lembas(1)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("green-dragon UpdateDish: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteDish(ctx, dragon, dishID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("green-dragon DeleteDish: err = %v, want ErrNotFound", err)
	}
	if got, err := s.GetDish(ctx, shack, dishID); err != nil || got != d {
		t.Errorf("orc-shack dish after green-dragon's attempts = %+v, %v; want unchanged %+v", got, err, d)
	}
}

func TestDishPriceMustBeInRestaurantCurrency(t *testing.T) {
	db := storetest.Migrated(t)
	shack := addRestaurant(t, openDB(t, db.OwnerURL), "orc-shack", "ZAR")

	d := lembas(4550)
	d.Price.Currency = "GBP"
	if _, err := openStore(t, db.AppURL).CreateDish(context.Background(), shack, d); err == nil {
		t.Error("created a GBP dish at a ZAR restaurant, want a foreign key violation")
	}
}

func TestRestaurantCurrencyCannotChangeUnderExistingPrices(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)
	owner := openDB(t, db.OwnerURL)
	shack := addRestaurant(t, owner, "orc-shack", "ZAR")
	if _, err := openStore(t, db.AppURL).CreateDish(ctx, shack, lembas(4550)); err != nil {
		t.Fatal(err)
	}

	if _, err := owner.ExecContext(ctx, `UPDATE restaurants SET currency = 'GBP' WHERE id = $1`, shack); err == nil {
		t.Error("changed the currency of a restaurant with ZAR prices, want a foreign key violation")
	}
}

func addRestaurant(t *testing.T, owner *sql.DB, slug, currency string) uuid.UUID {
	t.Helper()
	var u uuid.UUID
	err := owner.QueryRowContext(context.Background(),
		`INSERT INTO restaurants (slug, name, currency) VALUES ($1, $1, $2) RETURNING id`,
		slug, currency).Scan(&u)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func lembas(amount int64) dish.Dish {
	return dish.Dish{
		Name:        "Lembas",
		Description: "Elvish waybread.",
		Price:       dish.Money{Amount: amount, Currency: "ZAR"},
		Image:       "https://example.com/lembas.jpg",
	}
}
