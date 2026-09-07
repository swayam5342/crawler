// Package dedup provides a space-efficient probabilistic set for
// large-scale duplicate URL detection, as an alternative to an exact
// in-memory set when the number of distinct URLs a crawl will see is too
// large to hold precisely in memory.
package dedup

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

// BloomFilter is a thread-safe probabilistic set: Seen never has false
// negatives (if it says "not seen", the item truly wasn't added before),
// but may have false positives (it may occasionally claim an item was seen
// when it wasn't), at a rate controlled by the filter's size and hash
// count. It never shrinks and items cannot be removed.
type BloomFilter struct {
	mu     sync.Mutex
	filter *bloom.BloomFilter
}

// NewBloomFilter creates a filter sized for expectedItems elements at the
// given falsePositiveRate (e.g. 0.001 for 0.1%).
func NewBloomFilter(expectedItems uint, falsePositiveRate float64) *BloomFilter {
	return &BloomFilter{filter: bloom.NewWithEstimates(expectedItems, falsePositiveRate)}
}

// Seen reports whether url was already added, and adds it if not. It is
// the atomic "test and set" callers need for deduplication: only one
// concurrent caller for a given never-before-seen URL gets false back.
func (b *BloomFilter) Seen(url string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.filter.TestAndAddString(url)
}

// EstimatedFalsePositiveRate returns the filter's current estimated false
// positive rate given how many items have been added so far.
func (b *BloomFilter) EstimatedFalsePositiveRate() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := uint(b.filter.ApproximatedSize())
	return bloom.EstimateFalsePositiveRate(b.filter.Cap(), b.filter.K(), n)
}
