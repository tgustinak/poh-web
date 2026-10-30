package auth

import (
	"sync"
	"time"
)

// LoginRateLimiter is a small in-memory sliding-window limiter for the
// admin login endpoint. It is intentionally simple: single instance,
// per-IP counters, no persistence. Behind a proxy set BEHIND_PROXY=true
// and trust X-Forwarded-For set by the proxy (first entry = client).
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	blocked  map[string]time.Time

	maxAttempts int
	window      time.Duration
	lockout     time.Duration
}

// NewLoginRateLimiter returns a limiter allowing maxAttempts failures
// per window, locking out the IP for lockout duration.
func NewLoginRateLimiter(maxAttempts int, window, lockout time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		attempts:    make(map[string][]time.Time),
		blocked:     make(map[string]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
		lockout:     lockout,
	}
}

// DefaultLoginRateLimiter allows 10 failures per 10 minutes,
// then locks out for 10 minutes.
func DefaultLoginRateLimiter() *LoginRateLimiter {
	return NewLoginRateLimiter(10, 10*time.Minute, 10*time.Minute)
}

// Blocked reports whether ip is currently locked out.
func (l *LoginRateLimiter) Blocked(ip string) bool {
	if ip == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	until, ok := l.blocked[ip]
	if !ok {
		return false
	}

	if time.Now().Before(until) {
		return true
	}

	delete(l.blocked, ip)

	return false
}

// RecordFailure records a failed login and locks out when the threshold
// is reached. Returns true when this failure triggered a lockout.
func (l *LoginRateLimiter) RecordFailure(ip string) bool {
	if ip == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	kept := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	l.attempts[ip] = kept

	if len(kept) >= l.maxAttempts {
		l.blocked[ip] = now.Add(l.lockout)
		delete(l.attempts, ip)

		return true
	}

	return false
}

// Reset clears failures and lockout for ip (called on successful login).
func (l *LoginRateLimiter) Reset(ip string) {
	if ip == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, ip)
	delete(l.blocked, ip)
}
