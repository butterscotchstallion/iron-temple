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

// ---- Consume ----

// Consume spends budget on SUCCESS, which is the opposite of the Allow/Fail
// pair above and the reason it is a separate method. A limit on posting must
// count the posts; a limit on guessing must count only the wrong guesses.
func TestConsumeSpendsOnEveryCall(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)

	for i := range 3 {
		if !l.Consume("ada") {
			t.Fatalf("call %d refused before the limit was reached", i+1)
		}
	}
	if l.Consume("ada") {
		t.Error("a fourth call was allowed past a limit of 3")
	}
}

// The budget is per key, so one noisy account cannot silence another.
func TestConsumeIsPerKey(t *testing.T) {
	l, _ := newTestLimiter(1, time.Minute)

	if !l.Consume("ada") {
		t.Fatal("ada's first call was refused")
	}
	if l.Consume("ada") {
		t.Error("ada's second call was allowed past a limit of 1")
	}
	if !l.Consume("grace") {
		t.Error("grace was refused because ada had spent her own budget")
	}
}

// The window is fixed rather than sliding: once it passes, the count starts
// again from nothing.
func TestConsumeRecoversAfterTheWindow(t *testing.T) {
	l, advance := newTestLimiter(2, time.Minute)

	l.Consume("ada")
	l.Consume("ada")
	if l.Consume("ada") {
		t.Fatal("a third call inside the window was allowed past a limit of 2")
	}

	advance(time.Minute + time.Second)
	if !l.Consume("ada") {
		t.Error("the window passed and the budget did not come back")
	}
}

// Consume and Fail share one counter, which is what makes a single limiter
// usable for either policy — and what a future caller mixing them would need to
// know.
func TestConsumeSharesItsCounterWithFail(t *testing.T) {
	l, _ := newTestLimiter(2, time.Minute)

	l.Fail("ada")
	if !l.Consume("ada") {
		t.Fatal("the second unit of budget was refused")
	}
	if l.Consume("ada") {
		t.Error("a third unit was allowed past a limit of 2")
	}
}

// Sweep drops an expired window, which is what keeps the map from growing once
// per distinct key forever. The sweeper runs it for both limiters.
func TestConsumeWindowIsSwept(t *testing.T) {
	l, advance := newTestLimiter(1, time.Minute)

	l.Consume("ada")
	l.Sweep()
	if len(l.attempts) != 1 {
		t.Fatalf("a live window was swept: %d entries left", len(l.attempts))
	}

	advance(time.Minute + time.Second)
	l.Sweep()
	if len(l.attempts) != 0 {
		t.Errorf("an expired window survived the sweep: %d entries left", len(l.attempts))
	}
}
