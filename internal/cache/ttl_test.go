package cache

import (
	"testing"
	"time"
)

func newTestCache(max int) (*TTL[string, int], *time.Time) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	c := New[string, int](time.Minute, max)
	c.now = func() time.Time { return now }
	return c, &now
}

func TestGetSetExpire(t *testing.T) {
	c, now := newTestCache(10)
	if _, ok := c.Get("a"); ok {
		t.Fatal("empty cache returned a value")
	}
	c.Set("a", 1, c.Version())
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get = %d, %v; want 1, true", v, ok)
	}
	*now = now.Add(time.Minute)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expired entry returned")
	}
	if c.Len() != 0 {
		t.Fatalf("Len = %d after expired Get, want 0", c.Len())
	}
}

func TestDeleteDropsStaleSet(t *testing.T) {
	c, _ := newTestCache(10)
	v := c.Version() // a reader starts...
	c.Delete("a")    // ...a logout happens...
	c.Set("a", 1, v) // ...the reader's value may predate it.
	if _, ok := c.Get("a"); ok {
		t.Fatal("Set with a version from before Delete was stored")
	}
	c.Set("a", 2, c.Version())
	if v, ok := c.Get("a"); !ok || v != 2 {
		t.Fatalf("Get = %d, %v; want 2, true", v, ok)
	}
}

func TestFullCacheEvicts(t *testing.T) {
	c, now := newTestCache(2)
	c.Set("old", 1, c.Version())
	*now = now.Add(2 * time.Minute)
	c.Set("a", 2, c.Version())
	c.Set("b", 3, c.Version()) // full: the expired "old" goes first
	if _, ok := c.Get("a"); !ok {
		t.Fatal("live entry evicted while an expired one was there")
	}
	c.Set("c", 4, c.Version()) // full of live entries: one goes
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("new entry not stored")
	}
}
