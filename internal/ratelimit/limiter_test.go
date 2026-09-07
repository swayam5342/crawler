package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestWaitAllowsBurstImmediately(t *testing.T) {
	l := New(10, 3) // 10 rps, burst of 3
	ctx := context.Background()

	start := time.Now()
	for i := range 3 {
		if err := l.Wait(ctx, "example.com"); err != nil {
			t.Fatalf("Wait() #%d unexpected error: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Errorf("burst of 3 took %v, want near-instant", elapsed)
	}
}

func TestWaitBlocksBeyondBurst(t *testing.T) {
	l := New(20, 1) // 20 rps => ~50ms per token, burst 1
	ctx := context.Background()

	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	start := time.Now()
	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 30*time.Millisecond {
		t.Errorf("second Wait() returned after %v, expected to block roughly 50ms", elapsed)
	}
}

func TestWaitRespectsContextCancellation(t *testing.T) {
	l := New(0.1, 1) // effectively very slow refill after burst consumed
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := l.Wait(context.Background(), "example.com"); err != nil {
		t.Fatalf("unexpected error consuming burst: %v", err)
	}

	err := l.Wait(ctx, "example.com")
	if err == nil {
		t.Fatalf("expected context deadline error, got nil")
	}
}

func TestDomainsAreIndependent(t *testing.T) {
	l := New(1000, 1) // fast default so domain "a" would refill quickly if shared
	l.SetRate("slow.example.com", 0.001, 1)

	ctx := context.Background()
	if err := l.Wait(ctx, "fast.example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Immediately available again since "fast" domain has a high rate.
	start := time.Now()
	if err := l.Wait(ctx, "fast.example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("fast domain should not be throttled by slow domain's rate")
	}
}

func TestSetRateAppliesNewRate(t *testing.T) {
	l := New(1000, 1) // fast default
	ctx := context.Background()

	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l.SetRate("example.com", 20, 1) // slow down to ~50ms/token, consuming the reset-to-capacity token

	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	start := time.Now()
	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 30*time.Millisecond {
		t.Errorf("expected new rate to throttle to ~50ms, took %v", elapsed)
	}
}

func TestSetRateIsIdempotentForSameSettings(t *testing.T) {
	l := New(20, 1)
	ctx := context.Background()

	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Calling SetRate with identical params must not reset tokens back to
	// capacity, or it would let a caller bypass the limiter by repeatedly
	// re-applying the same crawl-delay.
	l.SetRate("example.com", 20, 1)

	start := time.Now()
	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 30*time.Millisecond {
		t.Errorf("idempotent SetRate should not reset bucket; Wait() returned after %v", elapsed)
	}
}
