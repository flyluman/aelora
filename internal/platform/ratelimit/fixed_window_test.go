package ratelimit

import (
	"testing"
	"time"
)

func TestFixedWindowLimiter(t *testing.T) {
	limiter := NewFixedWindowLimiter(2, 100*time.Millisecond, 1*time.Second)
	key := "u1:r1"
	if !limiter.Allow(key) || !limiter.Allow(key) {
		t.Fatal("expected first two requests allowed")
	}
	if limiter.Allow(key) {
		t.Fatal("expected third request denied in same window")
	}
	time.Sleep(120 * time.Millisecond)
	if !limiter.Allow(key) {
		t.Fatal("expected request allowed after window reset")
	}
}
