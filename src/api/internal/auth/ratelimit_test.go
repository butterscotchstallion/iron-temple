package auth

import (
	"sync"
	"testing"
	"time"
)

// newTestLimiter returns a limiter whose clock the test drives, so the window
// can be crossed without sleeping through it.
func newTestLimiter(limit int, period time.Duration) (*RateLimiter, func(time.Duration)) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	l := NewRateLimiter(limit, period)
	l.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	return l, func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(d)
	}
}

func TestAllowUntilLimitThenBlocks(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)

	for i := range 3 {
		if !l.Allow("ada") {
			t.Fatalf("attempt %d blocked before the limit was reached", i+1)
		}
		l.Fail("ada")
	}
	if l.Allow("ada") {
		t.Error("a fourth attempt was allowed past a limit of 3")
	}
}

// A limiter that consumed budget on every attempt would let an attacker lock
// out the account they are guessing at. Only failures count.
func TestSuccessDoesNotConsumeBudget(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)

	for range 10 {
		if !l.Allow("ada") {
			t.Fatal("Allow blocked without any recorded failure")
		}
	}
}

func TestResetClearsFailures(t *testing.T) {
	l, _ := newTestLimiter(2, time.Minute)
	l.Fail("ada")
	l.Fail("ada")
	if l.Allow("ada") {
		t.Fatal("precondition: the key should be blocked")
	}

	l.Reset("ada")
	if !l.Allow("ada") {
		t.Error("Reset did not clear the failure count")
	}
}

func TestWindowExpires(t *testing.T) {
	l, advance := newTestLimiter(2, time.Minute)
	l.Fail("ada")
	l.Fail("ada")
	if l.Allow("ada") {
		t.Fatal("precondition: the key should be blocked")
	}

	advance(time.Minute)
	if !l.Allow("ada") {
		t.Error("the key is still blocked after its window elapsed")
	}
}

// One user exhausting their budget must not affect anyone else — the key
// includes the username precisely so that lockout stays scoped.
func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTestLimiter(1, time.Minute)
	l.Fail("ada")
	if l.Allow("ada") {
		t.Fatal("precondition: ada should be blocked")
	}
	if !l.Allow("grace") {
		t.Error("blocking one key also blocked another")
	}
}

// The map is keyed by attacker-supplied values, so it must not grow forever.
func TestSweepDropsExpiredWindowsOnly(t *testing.T) {
	l, advance := newTestLimiter(5, time.Minute)
	l.Fail("old")
	advance(2 * time.Minute)
	l.Fail("fresh")

	l.Sweep()

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.attempts["old"]; ok {
		t.Error("Sweep kept an expired window")
	}
	if _, ok := l.attempts["fresh"]; !ok {
		t.Error("Sweep dropped a live window")
	}
}

func TestConcurrentUseIsRaceFree(t *testing.T) {
	l := NewRateLimiter(DefaultAttempts, DefaultWindow)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := string(rune('a' + i%5))
			l.Allow(key)
			l.Fail(key)
			l.Sweep()
			l.Reset(key)
		}()
	}
	wg.Wait()
}
