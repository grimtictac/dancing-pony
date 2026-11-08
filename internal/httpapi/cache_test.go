package httpapi

import (
	"context"
	"errors"
	"testing"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// countingStore answers the cached lookups and counts the calls that reach
// it. The embedded nil Store panics if anything else is called.
type countingStore struct {
	Store
	sessionCalls, restaurantCalls int
	loggedOut                     bool
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
