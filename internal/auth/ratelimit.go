package auth

import (
	"sync"
	"time"
)

// RateLimiter is a token bucket per key (a user ID): each key may make
// Burst requests at once, refilled at Rate per second. Unlike Throttle,
// which counts failures, it counts every request. It never sleeps: callers
// reply 429 with Retry-After.
type RateLimiter struct {
	Rate  float64 // tokens added per second
	Burst float64 // bucket size

	now func() time.Time // replaceable in tests

	mu sync.Mutex
	m  map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxRateKeys bounds memory: past it, buckets that have refilled are swept
// (a full bucket is the same as no bucket).
const maxRateKeys = 100_000

// NewRateLimiter returns a limiter allowing burst requests at once and
// rate per second after that.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{Rate: rate, Burst: float64(burst), now: time.Now, m: map[string]*bucket{}}
}

// Allow takes a token for key. If none is left it returns false and how
// long until one is.
func (l *RateLimiter) Allow(key string) (ok bool, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, found := l.m[key]
	if !found {
		if len(l.m) >= maxRateKeys {
			l.sweep(now)
		}
		b = &bucket{tokens: l.Burst, last: now}
		l.m[key] = b
	}
	b.tokens = min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.Rate * float64(time.Second))
}

func (l *RateLimiter) sweep(now time.Time) {
	for k, b := range l.m {
		if b.tokens+now.Sub(b.last).Seconds()*l.Rate >= l.Burst {
			delete(l.m, k)
		}
	}
}
