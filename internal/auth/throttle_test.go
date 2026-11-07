package auth

import (
	"testing"
	"time"
)

func TestThrottle(t *testing.T) {
	clock := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	th := NewThrottle(3, time.Second, 10*time.Second, time.Hour)
	th.now = func() time.Time { return clock }

	for i := range 3 {
		if n, blocked := th.Fail("k"); n != i+1 || blocked != 0 {
			t.Fatalf("free failure %d: Fail = (%d, %v), want (%d, 0)", i+1, n, blocked, i+1)
		}
	}
	if w := th.Wait("k"); w != 0 {
		t.Fatalf("after 3 free failures: wait %v, want 0", w)
	}
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second} {
		if n, blocked := th.Fail("k"); n != 4+i || blocked != want {
			t.Errorf("failure %d over the limit: Fail = (%d, %v), want (%d, %v)", i+1, n, blocked, 4+i, want)
		}
		if w := th.Wait("k"); w != want {
			t.Errorf("failure %d over the limit: wait %v, want %v", i+1, w, want)
		}
	}
	if w := th.Wait("other"); w != 0 {
		t.Errorf("other key: wait %v, want 0", w)
	}

	clock = clock.Add(11 * time.Second)
	if w := th.Wait("k"); w != 0 {
		t.Errorf("after the delay: wait %v, want 0", w)
	}

	clock = clock.Add(2 * time.Hour)
	th.Fail("k")
	if w := th.Wait("k"); w != 0 {
		t.Errorf("after being forgotten, one failure: wait %v, want 0", w)
	}

	for range 10 {
		th.Fail("k")
	}
	th.Reset("k")
	if w := th.Wait("k"); w != 0 {
		t.Errorf("after Reset: wait %v, want 0", w)
	}
}
