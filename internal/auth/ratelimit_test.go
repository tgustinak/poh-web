package auth

import (
	"testing"
	"time"
)

func TestLoginRateLimiterBlocksAfterThreshold(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute, time.Minute)

	if l.Blocked("1.2.3.4") {
		t.Fatal("should not be blocked initially")
	}

	l.RecordFailure("1.2.3.4")
	l.RecordFailure("1.2.3.4")

	if l.Blocked("1.2.3.4") {
		t.Fatal("should not be blocked before threshold")
	}

	if !l.RecordFailure("1.2.3.4") {
		t.Fatal("third failure should trigger lockout")
	}

	if !l.Blocked("1.2.3.4") {
		t.Fatal("should be blocked after threshold")
	}

	l.Reset("1.2.3.4")

	if l.Blocked("1.2.3.4") {
		t.Fatal("reset should clear block")
	}
}

func TestLoginRateLimiterLockoutExpires(t *testing.T) {
	l := NewLoginRateLimiter(1, time.Minute, 20*time.Millisecond)
	l.RecordFailure("5.6.7.8")

	if !l.Blocked("5.6.7.8") {
		t.Fatal("should be blocked")
	}

	time.Sleep(30 * time.Millisecond)

	if l.Blocked("5.6.7.8") {
		t.Fatal("lockout should expire")
	}
}
