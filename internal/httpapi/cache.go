package httpapi

import (
	"context"
	"time"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/cache"
	"the-dancing-pony-v2/internal/tenant"
)

// The time to live bounds how stale an entry can be when the data changes
// outside this process: pony admin granting admin, pony seed, or (one day)
// another instance. Logout in this process evicts at once.
const (
	sessionCacheTTL    = 30 * time.Second
	restaurantCacheTTL = time.Minute
	sessionCacheMax    = 10_000
	restaurantCacheMax = 1_000
)

// WithCache wraps s so that the session and restaurant lookups that
// nearly every request makes are served from memory (ADR-002). Only found
// rows are cached: caching "no such session" would let random tokens fill
// the cache. m may be nil.
func WithCache(s Store, m *Metrics) Store {
	return &cachedStore{
		Store:       s,
		metrics:     m,
		sessions:    cache.New[string, auth.User](sessionCacheTTL, sessionCacheMax),
		restaurants: cache.New[string, tenant.Restaurant](restaurantCacheTTL, restaurantCacheMax),
	}
}

// cachedStore overrides three Store methods; the embedded Store serves
// every other one unchanged.
type cachedStore struct {
	Store
	metrics     *Metrics
	sessions    *cache.TTL[string, auth.User] // by token hash
	restaurants *cache.TTL[string, tenant.Restaurant]
}

func (c *cachedStore) UserBySession(ctx context.Context, tokenHash []byte) (auth.User, error) {
	key := string(tokenHash)
	if u, ok := c.sessions.Get(key); ok {
		c.metrics.cacheLookup("session", true)
		return u, nil
	}
	c.metrics.cacheLookup("session", false)
	v := c.sessions.Version()
	u, err := c.Store.UserBySession(ctx, tokenHash)
	if err == nil {
		c.sessions.Set(key, u, v)
	}
	return u, err
}

// DeleteSession evicts before and after the delete, so a lookup racing
// with logout can't put the ended session back (see cache.TTL.Set).
func (c *cachedStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	key := string(tokenHash)
	c.sessions.Delete(key)
	err := c.Store.DeleteSession(ctx, tokenHash)
	c.sessions.Delete(key)
	return err
}

func (c *cachedStore) RestaurantBySlug(ctx context.Context, slug string) (tenant.Restaurant, error) {
	if r, ok := c.restaurants.Get(slug); ok {
		c.metrics.cacheLookup("restaurant", true)
		return r, nil
	}
	c.metrics.cacheLookup("restaurant", false)
	v := c.restaurants.Version()
	r, err := c.Store.RestaurantBySlug(ctx, slug)
	if err == nil {
		c.restaurants.Set(slug, r, v)
	}
	return r, err
}
