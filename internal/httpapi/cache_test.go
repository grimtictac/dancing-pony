package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// countingStore answers the cached lookups and counts the calls that reach
// it. The embedded nil Store panics if anything else is called.
type countingStore struct {
	Store
	sessionCalls, restaurantCalls int
	loggedOut                     bool

	listCalls, getCalls int
	dishName            string // what ListDishes/GetDish report; UpdateDish changes it
}

func (s *countingStore) UserBySession(context.Context, []byte) (auth.User, error) {
	s.sessionCalls++
	if s.loggedOut {
		return auth.User{}, store.ErrNotFound
	}
	return auth.User{Email: "frodo@shire.example"}, nil
}

func (s *countingStore) DeleteSession(context.Context, []byte) error {
	s.loggedOut = true
	return nil
}

func (s *countingStore) RestaurantBySlug(_ context.Context, slug string) (tenant.Restaurant, error) {
	s.restaurantCalls++
	if slug != "orc-shack" {
		return tenant.Restaurant{}, store.ErrNotFound
	}
	return tenant.Restaurant{Slug: slug}, nil
}

func (s *countingStore) ListDishes(context.Context, uuid.UUID, store.ListOptions) ([]dish.Dish, string, error) {
	s.listCalls++
	return []dish.Dish{{Name: s.dishName}}, "", nil
}

func (s *countingStore) GetDish(context.Context, uuid.UUID, uuid.UUID) (dish.Dish, error) {
	s.getCalls++
	return dish.Dish{Name: s.dishName}, nil
}

func (s *countingStore) UpdateDish(_ context.Context, _, _ uuid.UUID, d dish.Dish) (dish.Dish, error) {
	s.dishName = d.Name
	return d, nil
}

func TestCacheServesRepeatLookups(t *testing.T) {
	ctx := context.Background()
	inner := &countingStore{}
	s := WithCache(inner, nil)

	for range 3 {
		if _, err := s.UserBySession(ctx, []byte("hash")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.RestaurantBySlug(ctx, "orc-shack"); err != nil {
			t.Fatal(err)
		}
	}
	if inner.sessionCalls != 1 || inner.restaurantCalls != 1 {
		t.Fatalf("store calls: sessions %d, restaurants %d; want 1 each", inner.sessionCalls, inner.restaurantCalls)
	}
}

func TestCacheLogoutIsImmediate(t *testing.T) {
	ctx := context.Background()
	s := WithCache(&countingStore{}, nil)
	if _, err := s.UserBySession(ctx, []byte("hash")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, []byte("hash")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, []byte("hash")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after logout: err = %v, want ErrNotFound", err)
	}
}

func TestCacheSkipsNotFound(t *testing.T) {
	ctx := context.Background()
	inner := &countingStore{}
	s := WithCache(inner, nil)
	for range 2 {
		if _, err := s.RestaurantBySlug(ctx, "nowhere"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if inner.restaurantCalls != 2 {
		t.Fatalf("restaurant calls = %d, want 2 (not-found is not cached)", inner.restaurantCalls)
	}
}

func TestCacheServesMenuAndDish(t *testing.T) {
	ctx := context.Background()
	inner := &countingStore{dishName: "Seed Cake"}
	s := WithCache(inner, nil)
	rest, dishID := uuid.New(), uuid.New()

	for range 3 {
		if _, _, err := s.ListDishes(ctx, rest, store.ListOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetDish(ctx, rest, dishID); err != nil {
			t.Fatal(err)
		}
	}
	if inner.listCalls != 1 || inner.getCalls != 1 {
		t.Fatalf("store calls: list %d, get %d; want 1 each", inner.listCalls, inner.getCalls)
	}
}

func TestCacheWriteInvalidatesMenuAndDish(t *testing.T) {
	ctx := context.Background()
	inner := &countingStore{dishName: "Seed Cake"}
	s := WithCache(inner, nil)
	rest, dishID := uuid.New(), uuid.New()

	// Warm both caches, then change the dish through the cached store.
	if _, _, err := s.ListDishes(ctx, rest, store.ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDish(ctx, rest, dishID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDish(ctx, rest, dishID, dish.Dish{Name: "Lembas Bread"}); err != nil {
		t.Fatal(err)
	}

	list, _, err := s.ListDishes(ctx, rest, store.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDish(ctx, rest, dishID)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Name != "Lembas Bread" || got.Name != "Lembas Bread" {
		t.Fatalf("after update: list %q, dish %q; want both Lembas Bread (read-your-writes)", list[0].Name, got.Name)
	}
	if inner.listCalls != 2 || inner.getCalls != 2 {
		t.Fatalf("store calls: list %d, get %d; want 2 each (one before, one after the write)", inner.listCalls, inner.getCalls)
	}
}
