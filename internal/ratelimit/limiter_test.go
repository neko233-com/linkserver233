package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterAllowsBurstThenBlocks(t *testing.T) {
	current := time.Unix(0, 0)
	clock := func() time.Time { return current }

	// 1 token/sec, burst of 3.
	limiter := New(1, 3, clock)

	for i := 0; i < 3; i++ {
		if !limiter.Allow("client") {
			t.Fatalf("burst request %d should be allowed", i)
		}
	}
	if limiter.Allow("client") {
		t.Fatal("fourth request should be rate limited")
	}

	// After 1 second one token is refilled.
	current = current.Add(time.Second)
	if !limiter.Allow("client") {
		t.Fatal("request after refill should be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("request without further refill should be blocked")
	}
}

func TestLimiterIsolatesKeys(t *testing.T) {
	current := time.Unix(0, 0)
	limiter := New(1, 1, func() time.Time { return current })

	if !limiter.Allow("a") {
		t.Fatal("first key should be allowed")
	}
	if !limiter.Allow("b") {
		t.Fatal("second key should have its own bucket")
	}
	if limiter.Allow("a") {
		t.Fatal("first key should now be limited")
	}
}

func TestLimiterDisabled(t *testing.T) {
	limiter := New(0, 0, nil)
	if limiter.Enabled() {
		t.Fatal("limiter with zero rate should be disabled")
	}
	for i := 0; i < 100; i++ {
		if !limiter.Allow("client") {
			t.Fatal("disabled limiter should always allow")
		}
	}
}
