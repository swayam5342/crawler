// Package frontier manages the queue of URLs to crawl and deduplication of
// already-seen URLs. This initial version is in-memory; persistence and
// checkpointing are added in a later phase.
package frontier

import (
	"context"
	"sync"

	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// Item is a unit of work in the frontier.
type Item struct {
	URL   string // normalized, absolute URL
	Depth int
}

// Filter decides whether a URL is in-scope for the crawl (e.g. domain
// allow-listing). Implementations must be safe for concurrent use.
type Filter interface {
	Allowed(rawURL string) bool
}

// Frontier is a thread-safe FIFO queue of crawl items with built-in
// deduplication by normalized URL.
type Frontier struct {
	filter Filter

	mu       sync.Mutex
	queue    []Item
	visited  map[string]struct{}
	notify   chan struct{}
	closeCh  chan struct{}
	closeOne sync.Once

	closed  bool
	pending int // items added but not yet marked Done
}

// New creates an empty Frontier. filter may be nil to allow everything.
func New(filter Filter) *Frontier {
	return &Frontier{
		filter:  filter,
		visited: make(map[string]struct{}),
		notify:  make(chan struct{}, 1),
		closeCh: make(chan struct{}),
	}
}

// Add normalizes and enqueues a URL if it passes the filter and has not
// already been seen. Returns true if the URL was newly enqueued.
func (f *Frontier) Add(rawURL string, depth int) (bool, error) {
	normalized, err := urlutil.Normalize(rawURL)
	if err != nil {
		return false, err
	}

	if f.filter != nil && !f.filter.Allowed(normalized) {
		return false, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return false, nil
	}
	if _, seen := f.visited[normalized]; seen {
		return false, nil
	}
	f.visited[normalized] = struct{}{}
	f.queue = append(f.queue, Item{URL: normalized, Depth: depth})
	f.pending++
	f.signal()
	return true, nil
}

// Done marks one previously-dequeued item (from Next) as fully processed,
// including any follow-up Add calls for links it discovered. Once the
// number of pending items (queued or in-flight) reaches zero, the frontier
// auto-closes: it will never receive more work, so further Next calls
// return ok=false once the queue drains.
func (f *Frontier) Done() {
	f.mu.Lock()
	f.pending--
	pending := f.pending
	f.mu.Unlock()

	if pending <= 0 {
		f.Close()
	}
}

// Next blocks until an item is available, the context is canceled, or the
// frontier is closed and drained. ok is false when the frontier is
// permanently empty (closed with nothing left) or ctx is done.
func (f *Frontier) Next(ctx context.Context) (item Item, ok bool) {
	for {
		f.mu.Lock()
		if len(f.queue) > 0 {
			item = f.queue[0]
			f.queue = f.queue[1:]
			f.mu.Unlock()
			return item, true
		}
		closed := f.closed
		f.mu.Unlock()

		if closed {
			return Item{}, false
		}

		select {
		case <-ctx.Done():
			return Item{}, false
		case <-f.closeCh:
		case <-f.notify:
		}
	}
}

// Len returns the number of items currently queued.
func (f *Frontier) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queue)
}

// Visited reports how many distinct normalized URLs have been seen.
func (f *Frontier) Visited() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.visited)
}

// Close marks the frontier as permanently closed: no more items will be
// accepted, and blocked/future Next calls return ok=false once the queue
// drains.
func (f *Frontier) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	f.closeOne.Do(func() { close(f.closeCh) })
}

// signal wakes any goroutine blocked in Next. Must be called with f.mu held
// or immediately after a state change that Next cares about.
func (f *Frontier) signal() {
	select {
	case f.notify <- struct{}{}:
	default:
	}
}
