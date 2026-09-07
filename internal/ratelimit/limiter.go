// Package ratelimit provides a per-domain token bucket rate limiter, with
// support for overriding a domain's rate (e.g. from a robots.txt
// Crawl-delay directive) without discarding its accumulated token state.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter rate-limits requests independently per domain using a token
// bucket per domain. All methods are safe for concurrent use.
type Limiter struct {
	defaultRPS   float64
	defaultBurst int

	mu      sync.Mutex
	buckets map[string]*bucket
}

// New creates a Limiter. Domains are lazily assigned a bucket with the given
// default requests-per-second and burst on first use.
func New(defaultRPS float64, defaultBurst int) *Limiter {
	if defaultBurst < 1 {
		defaultBurst = 1
	}
	return &Limiter{
		defaultRPS:   defaultRPS,
		defaultBurst: defaultBurst,
		buckets:      make(map[string]*bucket),
	}
}

// Wait blocks until a token is available for domain, or ctx is canceled.
func (l *Limiter) Wait(ctx context.Context, domain string) error {
	return l.bucketFor(domain).wait(ctx)
}

// SetRate overrides the rate for a domain (e.g. from robots.txt
// Crawl-delay). It is idempotent for identical (rps, burst) pairs and does
// not reset accumulated tokens beyond clamping to the new capacity.
func (l *Limiter) SetRate(domain string, rps float64, burst int) {
	if burst < 1 {
		burst = 1
	}
	l.bucketFor(domain).setRate(rps, burst)
}

func (l *Limiter) bucketFor(domain string) *bucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[domain]
	if !ok {
		b = newBucket(l.defaultRPS, l.defaultBurst)
		l.buckets[domain] = b
	}
	return b
}

// bucket is a single domain's token bucket.
type bucket struct {
	mu           sync.Mutex
	tokens       float64
	capacity     float64
	refillPerSec float64
	last         time.Time
}

func newBucket(rps float64, burst int) *bucket {
	return &bucket{
		tokens:       float64(burst),
		capacity:     float64(burst),
		refillPerSec: rps,
		last:         time.Now(),
	}
}

func (b *bucket) setRate(rps float64, burst int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	capacity := float64(burst)
	if b.refillPerSec == rps && b.capacity == capacity {
		return // no-op: avoid disturbing accumulated tokens for identical settings
	}
	b.refill()
	b.refillPerSec = rps
	b.capacity = capacity
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
}

// wait blocks until a token can be consumed or ctx is done.
func (b *bucket) wait(ctx context.Context) error {
	for {
		b.mu.Lock()
		b.refill()
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		deficit := 1 - b.tokens
		rps := b.refillPerSec
		b.mu.Unlock()

		if rps <= 0 {
			// No refill configured; nothing to do but wait for cancellation
			// or a future SetRate. Poll rather than block forever.
			rps = 1
		}
		waitFor := time.Duration(deficit / rps * float64(time.Second))
		if waitFor <= 0 {
			waitFor = time.Millisecond
		}

		timer := time.NewTimer(waitFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// refill must be called with b.mu held.
func (b *bucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	b.last = now
	if b.refillPerSec <= 0 {
		return
	}
	b.tokens += elapsed.Seconds() * b.refillPerSec
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
}
