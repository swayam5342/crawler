package fetcher

import (
	"testing"
	"time"
)

func TestBackoffDurationGrowsExponentially(t *testing.T) {
	base := 100 * time.Millisecond
	max := 10 * time.Second

	d1 := backoffDuration(1, base, max, 0)
	d2 := backoffDuration(2, base, max, 0)
	d3 := backoffDuration(3, base, max, 0)

	if d1 != base {
		t.Errorf("attempt 1 = %v, want %v", d1, base)
	}
	if d2 != 2*base {
		t.Errorf("attempt 2 = %v, want %v", d2, 2*base)
	}
	if d3 != 4*base {
		t.Errorf("attempt 3 = %v, want %v", d3, 4*base)
	}
}

func TestBackoffDurationCapsAtMax(t *testing.T) {
	base := 1 * time.Second
	max := 5 * time.Second

	d := backoffDuration(10, base, max, 0)
	if d != max {
		t.Errorf("backoffDuration(10) = %v, want capped at %v", d, max)
	}
}

func TestBackoffDurationJitterStaysInBounds(t *testing.T) {
	base := 1 * time.Second
	max := 10 * time.Second
	jitter := 0.3

	for range 100 {
		d := backoffDuration(2, base, max, jitter)
		want := 2 * base
		lower := time.Duration(float64(want) * (1 - jitter))
		upper := time.Duration(float64(want) * (1 + jitter))
		if d < lower || d > upper {
			t.Fatalf("backoffDuration = %v, want within [%v, %v]", d, lower, upper)
		}
	}
}

func TestBackoffDurationNeverNegative(t *testing.T) {
	for range 100 {
		d := backoffDuration(1, time.Millisecond, time.Second, 5.0) // huge jitter factor
		if d < 0 {
			t.Fatalf("backoffDuration returned negative duration: %v", d)
		}
	}
}
