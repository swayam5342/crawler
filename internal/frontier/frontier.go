// Package frontier manages the queue of URLs to crawl and deduplication of
// already-seen URLs, with checkpointing support so a crashed or restarted
// crawl can resume without starting over.
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
	inFlight map[string]Item // dequeued (via Next) but not yet Done
	visited  map[string]struct{}
	notify   chan struct{}
	closeCh  chan struct{}
	closeOne sync.Once

	closed  bool
	pending int // items queued or in-flight, not yet marked Done
}

// New creates an empty Frontier. filter may be nil to allow everything.
func New(filter Filter) *Frontier {
	return &Frontier{
		filter:   filter,
		inFlight: make(map[string]Item),
		visited:  make(map[string]struct{}),
		notify:   make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
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

// Done marks a previously-dequeued item (from Next, identified by its
// normalized URL) as fully processed, including any follow-up Add calls for
// links it discovered. Once the number of pending items (queued or
// in-flight) reaches zero, the frontier auto-closes: it will never receive
// more work, so further Next calls return ok=false once the queue drains.
func (f *Frontier) Done(url string) {
	f.mu.Lock()
	delete(f.inFlight, url)
	f.pending--
	pending := f.pending
	f.mu.Unlock()

	if pending <= 0 {
		f.Close()
	}
}

// Next blocks until an item is available, the context is canceled, or the
// frontier is closed and drained. ok is false when the frontier is
// permanently empty (closed with nothing left) or ctx is done. Callers must
// call Done with the returned item's URL once processing (including any
// follow-up Add calls) completes.
func (f *Frontier) Next(ctx context.Context) (item Item, ok bool) {
	for {
		f.mu.Lock()
		if len(f.queue) > 0 {
			item = f.queue[0]
			f.queue = f.queue[1:]
			f.inFlight[item.URL] = item
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

// Checkpoint is a persistable snapshot of frontier state: enough to resume
// a crawl without re-visiting completed URLs or losing queued/in-flight
// work.
type Checkpoint struct {
	// Queue holds not-yet-dequeued items followed by any items that were
	// in-flight (dequeued but not yet Done) at snapshot time, so no work in
	// progress is silently lost if the process crashes.
	Queue   []Item   `json:"queue"`
	Visited []string `json:"visited"`
}

// Snapshot captures the frontier's current state for persistence. It does
// not stop the frontier; items may be added or dequeued concurrently, so
// the snapshot is a best-effort point-in-time view.
func (f *Frontier) Snapshot() Checkpoint {
	f.mu.Lock()
	defer f.mu.Unlock()

	queue := make([]Item, 0, len(f.inFlight)+len(f.queue))
	for _, item := range f.inFlight {
		queue = append(queue, item)
	}
	queue = append(queue, f.queue...)

	visited := make([]string, 0, len(f.visited))
	for u := range f.visited {
		visited = append(visited, u)
	}

	return Checkpoint{Queue: queue, Visited: visited}
}

// Restore loads a Checkpoint into a freshly-created, empty Frontier. It is
// an error to call Restore on a Frontier that has already had Add or Next
// called on it.
func (f *Frontier) Restore(cp Checkpoint) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range cp.Visited {
		f.visited[u] = struct{}{}
	}
	for _, item := range cp.Queue {
		f.visited[item.URL] = struct{}{} // Restore may run before all queue URLs were separately marked visited
		f.queue = append(f.queue, item)
	}
	f.pending += len(cp.Queue)
	if f.pending > 0 {
		f.signal()
	}
}
