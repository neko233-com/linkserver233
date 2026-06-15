package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a per-key token-bucket rate limiter that is safe for concurrent
// use. A zero or negative rate disables limiting.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64 // tokens added per second
	capacity float64 // maximum burst
	ttl      time.Duration
	now      func() time.Time
	lastGC   time.Time
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// New creates a limiter allowing ratePerSecond sustained requests with the
// given burst capacity. now may be nil to use the wall clock.
func New(ratePerSecond, burst float64, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		buckets:  make(map[string]*bucket),
		rate:     ratePerSecond,
		capacity: burst,
		ttl:      10 * time.Minute,
		now:      now,
		lastGC:   now(),
	}
}

// Enabled reports whether the limiter actively rejects traffic.
func (l *Limiter) Enabled() bool {
	return l != nil && l.rate > 0
}

// Allow consumes a token for key and reports whether the request may proceed.
func (l *Limiter) Allow(key string) bool {
	if !l.Enabled() {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.gcLocked(now)

	b, exists := l.buckets[key]
	if !exists {
		b = &bucket{tokens: l.capacity, lastSeen: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastSeen).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * l.rate
			if b.tokens > l.capacity {
				b.tokens = l.capacity
			}
		}
		b.lastSeen = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *Limiter) gcLocked(now time.Time) {
	if now.Sub(l.lastGC) < l.ttl {
		return
	}
	l.lastGC = now
	for key, b := range l.buckets {
		if now.Sub(b.lastSeen) > l.ttl {
			delete(l.buckets, key)
		}
	}
}
