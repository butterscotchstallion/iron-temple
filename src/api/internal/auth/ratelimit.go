package auth

import (
	"sync"
	"time"
)

// Login rate-limit defaults. The API is reachable from the internet, and a
// password endpoint with no limiter is an offline-speed guessing oracle. These
// are deliberately generous enough that a person fat-fingering their password
// never notices.
const (
	// DefaultAttempts is how many failed logins a key may make per window.
	DefaultAttempts = 10
	// DefaultWindow is the period over which attempts are counted.
	DefaultWindow = 15 * time.Minute
)

// RateLimiter counts failed attempts per key over a fixed window.
//
// State is in memory, so it is per-process and lost on restart. That is the
// right trade here: the deployment is a single replica, and the alternative —
// a database write on every login attempt — hands an attacker a cheap way to
// generate write load. It is a brute-force brake, not an audit log.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*window
	limit    int
	period   time.Duration
	// now is injectable so tests can advance the clock instead of sleeping.
	now func() time.Time
}

type window struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter returns a limiter allowing limit failures per period.
func NewRateLimiter(limit int, period time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string]*window),
		limit:    limit,
		period:   period,
		now:      time.Now,
	}
}

// Allow reports whether key may make another attempt. It does not consume
// budget — only a failure does, via Fail — so a user with the right password is
// never locked out by someone else guessing at their account.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.attempts[key]
	if !ok || !l.now().Before(w.resetAt) {
		return true
	}
	return w.count < l.limit
}

// Fail records a failed attempt against key.
func (l *RateLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	w, ok := l.attempts[key]
	if !ok || !now.Before(w.resetAt) {
		l.attempts[key] = &window{count: 1, resetAt: now.Add(l.period)}
		return
	}
	w.count++
}

// Reset clears the record for key, called on a successful login so a run of
// typos doesn't count against the session that follows it.
func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// Sweep drops windows that have expired. Without it the map grows once per
// distinct attacker-chosen key and never shrinks — a slow memory leak with an
// external trigger. Callers run it periodically; it is safe to call at any time.
func (l *RateLimiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for k, w := range l.attempts {
		if !now.Before(w.resetAt) {
			delete(l.attempts, k)
		}
	}
}
