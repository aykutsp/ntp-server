package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucketAllow(t *testing.T) {
	b := NewTokenBucket(2, 2)
	now := time.Now()

	if !b.Allow(now) {
		t.Fatalf("first request should pass")
	}
	if !b.Allow(now) {
		t.Fatalf("second request should pass")
	}
	if b.Allow(now) {
		t.Fatalf("third request should be blocked")
	}
	if !b.Allow(now.Add(600 * time.Millisecond)) {
		t.Fatalf("request should pass after refill")
	}
}

func TestKeyedLimiterPerClientIsolation(t *testing.T) {
	l := NewKeyedLimiter(1, 1, time.Minute, 4)
	now := time.Now()

	if !l.Allow("10.0.0.1", now) {
		t.Fatalf("first request from client A should pass")
	}
	if l.Allow("10.0.0.1", now) {
		t.Fatalf("second immediate request from client A should fail")
	}

	if !l.Allow("10.0.0.2", now) {
		t.Fatalf("client B should not be affected by client A")
	}
}
