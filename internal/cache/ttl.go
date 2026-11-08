// Package cache is a small in-memory cache whose entries expire, for
// lookups that almost every request makes (sessions, restaurants).
package cache

import (
	"sync"
	"time"
)

// TTL maps keys to values for a fixed time to live, holding at most max
// entries. It is safe for concurrent use.
//
// Go note: K and V are type parameters; "comparable" constrains K to types
// that work with == (and so as map keys).
type TTL[K comparable, V any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	now     func() time.Time // replaced in tests
	items   map[K]entry[V]
	version uint64 // bumped by every Delete; see Set
}

type entry[V any] struct {
	value   V
	expires time.Time
}

// New returns an empty cache.
func New[K comparable, V any](ttl time.Duration, max int) *TTL[K, V] {
	return &TTL[K, V]{ttl: ttl, max: max, now: time.Now, items: make(map[K]entry[V])}
}

// Get returns the value for k, unless it is missing or expired.
func (c *TTL[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[k]
	if ok && c.now().Before(e.expires) {
		return e.value, true
	}
	if ok {
		delete(c.items, k)
	}
	var zero V
	return zero, false
}

// Version returns a token for Set. Take it before reading the value from
// its source.
func (c *TTL[K, V]) Version() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.version
}

// Set stores v for k, unless a Delete happened since version was taken:
// then v may have been read before that delete and be stale (a session
// read just before its logout), so it is dropped. When the cache is full,
// expired entries go first, then an arbitrary one.
func (c *TTL[K, V]) Set(k K, v V, version uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if version != c.version {
		return
	}
	if _, exists := c.items[k]; !exists && len(c.items) >= c.max {
		c.evict()
	}
	c.items[k] = entry[V]{value: v, expires: c.now().Add(c.ttl)}
}

// Delete removes k, and makes any Set with an earlier version a no-op.
func (c *TTL[K, V]) Delete(k K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, k)
	c.version++
}

// Len returns the number of entries, including expired ones not yet removed.
func (c *TTL[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// evict makes room for one entry. The caller holds c.mu.
func (c *TTL[K, V]) evict() {
	now := c.now()
	for k, e := range c.items {
		if !now.Before(e.expires) {
			delete(c.items, k)
		}
	}
	if len(c.items) < c.max {
		return
	}
	for k := range c.items { // map order is random: drops an arbitrary entry
		delete(c.items, k)
		return
	}
}
