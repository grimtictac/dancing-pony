package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/cache"
	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/photo"
	"the-dancing-pony-v2/internal/review"
	"the-dancing-pony-v2/internal/sentiment"
	"the-dancing-pony-v2/internal/store"
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

	// The menu and single-dish reads are the bulk of the traffic. Caching
	// them per restaurant removes the four Row-Level-Security round trips
	// (BEGIN, set tenant, query, COMMIT) that each would otherwise make. A
	// one-minute TTL bounds staleness; writes invalidate at once (see
	// cachedStore.invalidate), so a caller always sees its own writes.
	menuCacheTTL = time.Minute
	menuCacheMax = 1_000
	dishCacheTTL = time.Minute
	dishCacheMax = 10_000
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
		menu:        cache.New[menuKey, menuEntry](menuCacheTTL, menuCacheMax),
		dishes:      cache.New[dishKey, dish.Dish](dishCacheTTL, dishCacheMax),
		gen:         make(map[uuid.UUID]uint64),
	}
}

// cachedStore overrides the read and write methods that touch sessions,
// restaurants, menus and dishes; the embedded Store serves every other one
// unchanged.
type cachedStore struct {
	Store
	metrics     *Metrics
	sessions    *cache.TTL[string, auth.User] // by token hash
	restaurants *cache.TTL[string, tenant.Restaurant]
	menu        *cache.TTL[menuKey, menuEntry]
	dishes      *cache.TTL[dishKey, dish.Dish]

	// gen is a per-restaurant cache generation, part of every menu and dish
	// key. A write bumps it, orphaning every page and dish cached for that
	// tenant in one step (they then expire by TTL); this gives
	// read-your-writes without tracking which pages a change affects.
	genMu sync.Mutex
	gen   map[uuid.UUID]uint64
}

// menuKey identifies one dish-list query within a restaurant, at a
// generation. store.ListOptions is comparable (only strings and integers),
// so it works as part of a map key.
type menuKey struct {
	restaurant uuid.UUID
	generation uint64
	opts       store.ListOptions
}

// menuEntry is a cached page: the dishes and the next-page cursor. The slice
// is shared with every caller that hits this entry; the handlers only read
// it (they JSON-encode it), so they do not race.
type menuEntry struct {
	dishes []dish.Dish
	next   string
}

// dishKey identifies one dish within a restaurant, at a generation.
type dishKey struct {
	restaurant uuid.UUID
	generation uint64
	dish       uuid.UUID
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

// generation returns the restaurant's current cache generation.
func (c *cachedStore) generation(r uuid.UUID) uint64 {
	c.genMu.Lock()
	defer c.genMu.Unlock()
	return c.gen[r]
}

// invalidate bumps the restaurant's generation, so every menu page and dish
// cached for it becomes unreachable (and expires by TTL). Called after any
// write that can change what a read returns, which is what gives
// read-your-writes.
func (c *cachedStore) invalidate(r uuid.UUID) {
	c.genMu.Lock()
	c.gen[r]++
	c.genMu.Unlock()
}

// ListDishes serves a dish-list page from memory. Only the query's inputs
// and the restaurant's generation form the key, so the database's ordering,
// collation and full-text search results are cached exactly as produced;
// nothing is re-derived in this process.
func (c *cachedStore) ListDishes(ctx context.Context, restaurantID uuid.UUID, opts store.ListOptions) ([]dish.Dish, string, error) {
	key := menuKey{restaurant: restaurantID, generation: c.generation(restaurantID), opts: opts}
	if e, ok := c.menu.Get(key); ok {
		c.metrics.cacheLookup("menu", true)
		return e.dishes, e.next, nil
	}
	c.metrics.cacheLookup("menu", false)
	v := c.menu.Version()
	dishes, next, err := c.Store.ListDishes(ctx, restaurantID, opts)
	if err == nil {
		c.menu.Set(key, menuEntry{dishes: dishes, next: next}, v)
	}
	return dishes, next, err
}

// GetDish serves a single dish from memory, keyed like the menu.
func (c *cachedStore) GetDish(ctx context.Context, restaurantID, dishID uuid.UUID) (dish.Dish, error) {
	key := dishKey{restaurant: restaurantID, generation: c.generation(restaurantID), dish: dishID}
	if d, ok := c.dishes.Get(key); ok {
		c.metrics.cacheLookup("dish", true)
		return d, nil
	}
	c.metrics.cacheLookup("dish", false)
	v := c.dishes.Version()
	d, err := c.Store.GetDish(ctx, restaurantID, dishID)
	if err == nil {
		c.dishes.Set(key, d, v)
	}
	return d, err
}

// The writes below invalidate the restaurant's menu and dish caches on
// success. A dish's rating is part of both the list and the single-dish
// read, so reviews invalidate too, not just dish edits.

func (c *cachedStore) CreateDish(ctx context.Context, restaurantID uuid.UUID, d dish.Dish) (dish.Dish, error) {
	created, err := c.Store.CreateDish(ctx, restaurantID, d)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return created, err
}

func (c *cachedStore) UpdateDish(ctx context.Context, restaurantID, dishID uuid.UUID, d dish.Dish) (dish.Dish, error) {
	updated, err := c.Store.UpdateDish(ctx, restaurantID, dishID, d)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return updated, err
}

func (c *cachedStore) DeleteDish(ctx context.Context, restaurantID, dishID uuid.UUID) (string, error) {
	key, err := c.Store.DeleteDish(ctx, restaurantID, dishID)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return key, err
}

func (c *cachedStore) SetDishPhoto(ctx context.Context, restaurantID, dishID uuid.UUID, key string, p photo.Processed) (string, error) {
	oldKey, err := c.Store.SetDishPhoto(ctx, restaurantID, dishID, key, p)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return oldKey, err
}

func (c *cachedStore) DeleteDishPhoto(ctx context.Context, restaurantID, dishID uuid.UUID) (string, error) {
	key, err := c.Store.DeleteDishPhoto(ctx, restaurantID, dishID)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return key, err
}

func (c *cachedStore) PutReview(ctx context.Context, restaurantID, dishID, userID uuid.UUID, in review.Input, s *sentiment.Sentiment) (review.Review, bool, error) {
	r, created, err := c.Store.PutReview(ctx, restaurantID, dishID, userID, in, s)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return r, created, err
}

func (c *cachedStore) DeleteReview(ctx context.Context, restaurantID, dishID, userID uuid.UUID) error {
	err := c.Store.DeleteReview(ctx, restaurantID, dishID, userID)
	if err == nil {
		c.invalidate(restaurantID)
	}
	return err
}
