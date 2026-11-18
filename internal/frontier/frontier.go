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
	URL   string `json:"url"` // normalized, absolute URL
	Depth int    `json:"depth"`
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

	closed    bool
	pending   int  // items queued or in-flight, not yet marked Done
	keepAlive bool // if true, never auto-close when pending reaches zero
}

// Option configures a Frontier.
type Option func(*Frontier)

// WithKeepAlive keeps the frontier open even when all pending work has
// been processed, so new URLs can be added later (e.g. from the control
// dashboard). Without it, the frontier auto-closes once drained. A
// keep-alive frontier only closes via an explicit Close.
func WithKeepAlive() Option {
	return func(f *Frontier) { f.keepAlive = true }
}

// New creates an empty Frontier. filter may be nil to allow everything.
func New(filter Filter, opts ...Option) *Frontier {
	f := &Frontier{
		filter:   filter,
		inFlight: make(map[string]Item),
		visited:  make(map[string]struct{}),
		notify:   make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
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
	if _, ok := f.inFlight[url]; !ok {
		// Not in flight: either released back into the queue or dropped
		// by Reset. Either way, pending was already accounted for.
		f.mu.Unlock()
		return
	}
	delete(f.inFlight, url)
	f.pending--
	pending := f.pending
	keepAlive := f.keepAlive
	f.mu.Unlock()

	if pending <= 0 && !keepAlive {
		f.Close()
	}
}

// Release returns a dequeued-but-unprocessed item to the front of the
// queue (e.g. because the crawl was paused right after Next handed it
// out). Pending accounting is unchanged; a subsequent Done for the same
// URL is a no-op until the item is handed out again.
func (f *Frontier) Release(item Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.inFlight[item.URL]; !ok {
		return
	}
	delete(f.inFlight, item.URL)
	f.queue = append([]Item{item}, f.queue...)
	f.signal()
}

// Requeue enqueues rawURL even if it was already visited (e.g. to retry
// a URL from the dead-letter log). It is a no-op if the URL is currently
// queued or in flight, or rejected by the filter. Returns true if the URL
// was enqueued.
func (f *Frontier) Requeue(rawURL string, depth int) (bool, error) {
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
	if _, ok := f.inFlight[normalized]; ok {
		return false, nil
	}
	for _, it := range f.queue {
		if it.URL == normalized {
			return false, nil
		}
	}
	f.visited[normalized] = struct{}{}
	f.queue = append(f.queue, Item{URL: normalized, Depth: depth})
	f.pending++
	f.signal()
	return true, nil
}

// RemoveQueued drops a not-yet-dequeued URL from the queue. The URL stays
// marked as visited, so it will not be re-discovered. Returns true if it
// was found in the queue.
func (f *Frontier) RemoveQueued(rawURL string) bool {
	normalized, err := urlutil.Normalize(rawURL)
	if err != nil {
		normalized = rawURL
	}

	f.mu.Lock()
	removed := false
	for i, it := range f.queue {
		if it.URL == normalized {
			f.queue = append(f.queue[:i], f.queue[i+1:]...)
			f.pending--
			removed = true
			break
		}
	}
	pending := f.pending
	keepAlive := f.keepAlive
	f.mu.Unlock()

	if removed && pending <= 0 && !keepAlive {
		f.Close()
	}
	return removed
}

// Reset forgets every queued item and the visited set, so previously
// crawled URLs can be crawled again. Items currently in flight are
// forgotten too: their eventual Done calls become no-ops, and links they
// discover are still added normally.
func (f *Frontier) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = nil
	f.inFlight = make(map[string]Item)
	f.visited = make(map[string]struct{})
	f.pending = 0
}

// Stats is a consistent point-in-time view of frontier sizes.
type Stats struct {
	Queued   int  `json:"queued"`
	InFlight int  `json:"in_flight"`
	Visited  int  `json:"visited"`
	Closed   bool `json:"closed"`
}

// Stats returns current queue, in-flight and visited counts.
func (f *Frontier) Stats() Stats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Stats{
		Queued:   len(f.queue),
		InFlight: len(f.inFlight),
		Visited:  len(f.visited),
		Closed:   f.closed,
	}
}

// Peek returns up to n queued items (in dequeue order) without removing
// them, plus everything currently in flight.
func (f *Frontier) Peek(n int) (queued []Item, inFlight []Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n < 0 || n > len(f.queue) {
		n = len(f.queue)
	}
	queued = append([]Item(nil), f.queue[:n]...)
	inFlight = make([]Item, 0, len(f.inFlight))
	for _, it := range f.inFlight {
		inFlight = append(inFlight, it)
	}
	return queued, inFlight
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
