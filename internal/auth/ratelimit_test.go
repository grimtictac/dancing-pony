package auth

import (
	"testing"
	"time"
)

func TestRateLimiterBurstThenRate(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	l := NewRateLimiter(10, 3)
	l.now = func() time.Time { return now }

	for i := range 3 {
		if ok, _ := l.Allow("frodo"); !ok {
			t.Fatalf("request %d of the burst refused", i+1)
		}
	}
	ok, wait := l.Allow("frodo")
	if ok {
		t.Fatal("request past the burst allowed")
	}
	if wait != 100*time.Millisecond {
		t.Fatalf("wait = %v, want 100ms (one token at 10/s)", wait)
	}
	if ok, _ := l.Allow("sam"); !ok {
		t.Fatal("another user was limited by frodo's bucket")
	}

	now = now.Add(100 * time.Millisecond)
	if ok, _ := l.Allow("frodo"); !ok {
		t.Fatal("refilled token refused")
	}
	if ok, _ := l.Allow("frodo"); ok {
		t.Fatal("second request after one refill allowed")
	}

	now = now.Add(time.Hour)
	for i := range 3 {
		if ok, _ := l.Allow("frodo"); !ok {
			t.Fatalf("after a long pause, request %d refused (bucket should be full, not more)", i+1)
		}
	}
	if ok, _ := l.Allow("frodo"); ok {
		t.Fatal("bucket refilled past its burst")
	}
}
