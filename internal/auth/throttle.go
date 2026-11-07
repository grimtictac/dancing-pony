package auth

import (
	"sync"
	"time"
)

// Throttle slows down repeated failures for a key (an IP address, an
// email, or both). After Free failures, each further failure blocks the
// key for an exponentially growing delay, capped at Max. It never locks an
// account outright and never sleeps: callers check Wait and reply 429
// with Retry-After at once, so an attacker can't tie up server resources.
type Throttle struct {
	Free   int           // failures allowed before any delay
	Base   time.Duration // first delay
	Max    time.Duration // longest delay
	Forget time.Duration // a key quiet for this long starts afresh

	now func() time.Time // replaceable in tests

	// Go note: handlers run concurrently, one goroutine per request, and Go
	// maps aren't safe for concurrent use. sync.Mutex lets one goroutine at
	// a time in; "mu.Lock(); defer mu.Unlock()" holds it until the method
	// returns. Its zero value is ready to use, so it needs no constructor.
	mu sync.Mutex
	m  map[string]*throttleEntry
}

type throttleEntry struct {
	failures     int
	last         time.Time
	blockedUntil time.Time
}

// maxThrottleKeys bounds memory: past it, forgotten keys are swept.
const maxThrottleKeys = 100_000

// NewThrottle returns a Throttle with the given policy.
func NewThrottle(free int, base, maxDelay, forget time.Duration) *Throttle {
	return &Throttle{Free: free, Base: base, Max: maxDelay, Forget: forget, now: time.Now, m: map[string]*throttleEntry{}}
}

// Wait returns how long key must wait before its next attempt (0: go ahead).
func (t *Throttle) Wait(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m[key]
	if !ok {
		return 0
	}
	now := t.now()
	if now.Sub(e.last) > t.Forget {
		delete(t.m, key)
		return 0
	}
	return max(0, e.blockedUntil.Sub(now))
}

// Fail records a failed attempt for key.
func (t *Throttle) Fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if len(t.m) >= maxThrottleKeys {
		t.sweep(now)
	}
	e, ok := t.m[key]
	if !ok || now.Sub(e.last) > t.Forget {
		e = &throttleEntry{}
		t.m[key] = e
	}
	e.failures++
	e.last = now
	if over := e.failures - t.Free; over > 0 {
		delay := t.Max
		if over < 32 && t.Base<<(over-1) < t.Max {
			delay = t.Base << (over - 1) // 1x, 2x, 4x, ... the base delay
		}
		e.blockedUntil = now.Add(delay)
	}
}

// Reset forgets key, e.g. after a successful login.
func (t *Throttle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, key)
}

func (t *Throttle) sweep(now time.Time) {
	for k, e := range t.m {
		if now.Sub(e.last) > t.Forget {
			delete(t.m, k)
		}
	}
}
