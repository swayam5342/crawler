package dedup

import (
	"fmt"
	"sync"
	"testing"
)

func TestSeenReturnsFalseThenTrue(t *testing.T) {
	b := NewBloomFilter(1000, 0.01)

	if b.Seen("https://example.com/a") {
		t.Errorf("expected first Seen() to return false (not seen before)")
	}
	if !b.Seen("https://example.com/a") {
		t.Errorf("expected second Seen() for the same URL to return true")
	}
}

func TestSeenDistinguishesDifferentURLs(t *testing.T) {
	b := NewBloomFilter(1000, 0.01)

	if b.Seen("https://example.com/a") {
		t.Errorf("expected Seen(a) to be false initially")
	}
	if b.Seen("https://example.com/b") {
		t.Errorf("expected Seen(b) to be false initially (distinct from a)")
	}
}

func TestFalsePositiveRateStaysWithinReasonableBound(t *testing.T) {
	const n = 10000
	const targetFP = 0.01
	b := NewBloomFilter(n, targetFP)

	for i := range n {
		b.Seen(fmt.Sprintf("https://example.com/page-%d", i))
	}

	falsePositives := 0
	const trials = 10000
	for i := range trials {
		// Query URLs guaranteed not to have been added.
		if b.Seen(fmt.Sprintf("https://example.com/never-added-%d", i)) {
			falsePositives++
		}
	}

	rate := float64(falsePositives) / float64(trials)
	// Generous upper bound: this isn't re-validating the library's FP math,
	// just guarding against a gross misuse (e.g. an undersized filter or a
	// broken hash) that would blow the rate up by an order of magnitude.
	if rate > 0.15 {
		t.Errorf("observed false positive rate %.4f is far above target %.4f", rate, targetFP)
	}
}

func TestSeenIsSafeForConcurrentUse(t *testing.T) {
	b := NewBloomFilter(10000, 0.01)

	const n = 200
	var wg sync.WaitGroup
	results := make([]bool, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = b.Seen("https://example.com/shared")
		}(i)
	}
	wg.Wait()

	falseCount := 0
	for _, r := range results {
		if !r {
			falseCount++
		}
	}
	if falseCount != 1 {
		t.Errorf("expected exactly 1 caller to observe Seen()=false for a shared URL added concurrently, got %d", falseCount)
	}
}

func TestEstimatedFalsePositiveRateIncreasesWithLoad(t *testing.T) {
	b := NewBloomFilter(100, 0.01)
	before := b.EstimatedFalsePositiveRate()

	for i := range 1000 { // load well past the sized-for capacity
		b.Seen(fmt.Sprintf("https://example.com/page-%d", i))
	}
	after := b.EstimatedFalsePositiveRate()

	if after <= before {
		t.Errorf("expected estimated false positive rate to increase after overloading the filter: before=%v after=%v", before, after)
	}
}
