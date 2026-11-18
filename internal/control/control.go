// Package control is the runtime control plane for a crawl: it lets an
// operator (via the dashboard API) add URLs, pause and resume, retry
// failed URLs, change domain scoping, reset state, and read live stats,
// without restarting the process.
package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swayamsb50/web_crawler/internal/crawler"
	"github.com/swayamsb50/web_crawler/internal/deadletter"
	"github.com/swayamsb50/web_crawler/internal/frontier"
	"github.com/swayamsb50/web_crawler/internal/storage"
	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// ErrUnsupported is returned when the configured storage backend does not
// support an operation (e.g. browsing pages).
var ErrUnsupported = errors.New("not supported by the configured storage backend")

// State is the crawl's high-level run state.
type State string

const (
	StateRunning  State = "running"  // workers are fetching
	StateIdle     State = "idle"     // running, but nothing queued or in flight
	StatePaused   State = "paused"   // operator paused; queue is preserved
	StateStopping State = "stopping" // shutting down; draining in-flight work
	StateFinished State = "finished" // batch crawl drained its frontier
)

// DeadLetterLog is the dead-letter store as seen by the controller.
// *deadletter.FileSink implements it.
type DeadLetterLog interface {
	Entries() ([]deadletter.Entry, error)
	Remove(drop func(deadletter.Entry) bool) (int, error)
}

// Options wires a Controller to the running crawl's components. Frontier,
// Filter and Gate are required; everything else is optional.
type Options struct {
	Frontier   *frontier.Frontier
	Filter     *frontier.DomainFilter
	Gate       *crawler.PauseGate
	DeadLetter DeadLetterLog
	Checkpoint frontier.CheckpointStore
	Pages      storage.Browser
	Purger     storage.Purger
	// BreakerState reports a domain's circuit breaker state, e.g. "open".
	BreakerState func(domain string) string
	// KeepAlive reports whether the crawl stays up when the queue drains.
	KeepAlive bool
	Logger    *slog.Logger
	// Now overrides the clock (tests).
	Now func() time.Time
}

// Controller is safe for concurrent use.
type Controller struct {
	opts      Options
	logger    *slog.Logger
	stats     *stats
	startedAt time.Time

	stopping atomic.Bool
	finished atomic.Bool

	dlMu    sync.Mutex
	dlCount int // cached dead-letter entry count
}

// New builds a Controller.
func New(opts Options) (*Controller, error) {
	if opts.Frontier == nil || opts.Filter == nil || opts.Gate == nil {
		return nil, errors.New("control: Frontier, Filter and Gate are required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	c := &Controller{
		opts:      opts,
		logger:    opts.Logger.With("component", "control"),
		stats:     newStats(opts.Now),
		startedAt: opts.Now(),
	}
	c.recountDeadLetters()
	return c, nil
}

// OnFetch implements crawler.EventSink.
func (c *Controller) OnFetch(ev crawler.FetchEvent) {
	c.stats.record(ev)
	if ev.Outcome == crawler.OutcomeFailure && c.opts.DeadLetter != nil {
		c.dlMu.Lock()
		c.dlCount++
		c.dlMu.Unlock()
	}
}

// MarkStopping records that a shutdown is in progress.
func (c *Controller) MarkStopping() { c.stopping.Store(true) }

// MarkFinished records that a batch crawl drained its frontier.
func (c *Controller) MarkFinished() { c.finished.Store(true) }

// State reports the crawl's current run state.
func (c *Controller) State() State {
	switch {
	case c.stopping.Load():
		return StateStopping
	case c.finished.Load():
		return StateFinished
	case c.opts.Gate.Paused():
		return StatePaused
	}
	fs := c.opts.Frontier.Stats()
	if fs.Queued == 0 && fs.InFlight == 0 {
		return StateIdle
	}
	return StateRunning
}

// Status is the dashboard's main polling payload.
type Status struct {
	State          State          `json:"state"`
	KeepAlive      bool           `json:"keep_alive"`
	StartedAt      time.Time      `json:"started_at"`
	StatsSince     time.Time      `json:"stats_since"`
	UptimeSeconds  int64          `json:"uptime_seconds"`
	Frontier       frontier.Stats `json:"frontier"`
	Totals         Totals         `json:"totals"`
	PagesPerSecond float64        `json:"pages_per_second"`
	ErrorRate      float64        `json:"error_rate"` // failures / (successes + failures)
	DeadLetters    int            `json:"dead_letters"`
	Throughput     []Point        `json:"throughput"`
	Domains        []DomainStats  `json:"domains"`
	AllowedDomains []string       `json:"allowed_domains"`
}

// Status returns a snapshot of crawl state and statistics.
func (c *Controller) Status() Status {
	snap := c.stats.snapshot(100)
	if c.opts.BreakerState != nil {
		for i := range snap.Domains {
			snap.Domains[i].Breaker = c.opts.BreakerState(snap.Domains[i].Domain)
		}
	}
	var errRate float64
	if n := snap.Totals.Success + snap.Totals.Failure; n > 0 {
		errRate = float64(snap.Totals.Failure) / float64(n)
	}
	c.dlMu.Lock()
	dl := c.dlCount
	c.dlMu.Unlock()

	return Status{
		State:          c.State(),
		KeepAlive:      c.opts.KeepAlive,
		StartedAt:      c.startedAt,
		StatsSince:     snap.Since,
		UptimeSeconds:  int64(c.opts.Now().Sub(c.startedAt).Seconds()),
		Frontier:       c.opts.Frontier.Stats(),
		Totals:         snap.Totals,
		PagesPerSecond: snap.PagesPerS,
		ErrorRate:      errRate,
		DeadLetters:    dl,
		Throughput:     snap.Throughput,
		Domains:        snap.Domains,
		AllowedDomains: c.opts.Filter.Domains(),
	}
}

// Recent returns up to n recently processed URLs, newest first.
func (c *Controller) Recent(n int) []Event { return c.stats.recentEvents(n) }

// Pause stops workers from starting new fetches. In-flight requests finish.
func (c *Controller) Pause() bool {
	ok := c.opts.Gate.Pause()
	if ok {
		c.logger.Info("crawl paused by operator")
	}
	return ok
}

// Resume lets workers start new fetches again.
func (c *Controller) Resume() bool {
	ok := c.opts.Gate.Resume()
	if ok {
		c.logger.Info("crawl resumed by operator")
	}
	return ok
}

// AddResult reports what happened to one submitted URL.
type AddResult struct {
	URL    string `json:"url"`
	Status string `json:"status"` // queued, duplicate, out_of_scope, invalid, closed
	Error  string `json:"error,omitempty"`
}

// AddOptions controls AddURLs.
type AddOptions struct {
	// AllowDomain adds each URL's domain to the allow-list when the crawl
	// is scoped to specific domains, so the URL (and links to the same
	// site) are in scope.
	AllowDomain bool
	// Recrawl enqueues URLs even if they were already visited.
	Recrawl bool
}

// MaxURLsPerAdd bounds a single AddURLs call.
const MaxURLsPerAdd = 1000

// AddURLs enqueues URLs at depth 0 (as new seeds).
func (c *Controller) AddURLs(urls []string, opts AddOptions) ([]AddResult, error) {
	if len(urls) > MaxURLsPerAdd {
		return nil, fmt.Errorf("too many URLs in one request (%d > %d)", len(urls), MaxURLsPerAdd)
	}
	results := make([]AddResult, 0, len(urls))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		results = append(results, c.addOne(raw, opts))
	}
	queued := 0
	for _, r := range results {
		if r.Status == "queued" {
			queued++
		}
	}
	c.logger.Info("URLs submitted by operator", "submitted", len(results), "queued", queued)
	return results, nil
}

func (c *Controller) addOne(raw string, opts AddOptions) AddResult {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	res := AddResult{URL: raw}

	normalized, err := urlutil.Normalize(raw)
	if err == nil && !strings.HasPrefix(normalized, "http://") && !strings.HasPrefix(normalized, "https://") {
		err = errors.New("only http and https URLs are supported")
	}
	if err != nil {
		res.Status, res.Error = "invalid", err.Error()
		return res
	}
	res.URL = normalized

	if opts.AllowDomain && len(c.opts.Filter.Domains()) > 0 {
		// Only when already scoped: adding to an empty allow-list would
		// turn "allow everything" into "allow only this domain".
		if domain, err := urlutil.Domain(normalized); err == nil && c.opts.Filter.Add(domain) {
			c.logger.Info("domain added to allow-list", "domain", domain)
		}
	}

	if !c.opts.Filter.Allowed(normalized) {
		res.Status = "out_of_scope"
		return res
	}
	if c.opts.Frontier.Stats().Closed {
		res.Status = "closed"
		return res
	}

	var added bool
	if opts.Recrawl {
		added, err = c.opts.Frontier.Requeue(normalized, 0)
	} else {
		added, err = c.opts.Frontier.Add(normalized, 0)
	}
	switch {
	case err != nil:
		res.Status, res.Error = "invalid", err.Error()
	case added:
		res.Status = "queued"
	default:
		res.Status = "duplicate"
	}
	return res
}

// QueueView lists what's waiting and what's being fetched right now.
type QueueView struct {
	Queued      []frontier.Item `json:"queued"`
	InFlight    []frontier.Item `json:"in_flight"`
	TotalQueued int             `json:"total_queued"`
}

// Queue returns up to limit queued items plus everything in flight.
func (c *Controller) Queue(limit int) QueueView {
	queued, inFlight := c.opts.Frontier.Peek(limit)
	return QueueView{Queued: queued, InFlight: inFlight, TotalQueued: c.opts.Frontier.Stats().Queued}
}

// RemoveQueued drops a URL from the queue.
func (c *Controller) RemoveQueued(url string) bool { return c.opts.Frontier.RemoveQueued(url) }

// AllowDomain adds a domain to the crawl's allow-list.
func (c *Controller) AllowDomain(domain string) bool { return c.opts.Filter.Add(domain) }

// DisallowDomain removes a domain from the crawl's allow-list.
func (c *Controller) DisallowDomain(domain string) bool { return c.opts.Filter.Remove(domain) }

// DeadLetters returns every permanently-failed URL, newest first.
func (c *Controller) DeadLetters() ([]deadletter.Entry, error) {
	if c.opts.DeadLetter == nil {
		return nil, nil
	}
	entries, err := c.opts.DeadLetter.Entries()
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

// RetryDeadLetters re-queues failed URLs (all when urls is empty) and
// removes the re-queued ones from the dead-letter log. URLs that are out
// of scope or already queued stay in the log.
func (c *Controller) RetryDeadLetters(urls []string) (int, error) {
	if c.opts.DeadLetter == nil {
		return 0, nil
	}
	entries, err := c.opts.DeadLetter.Entries()
	if err != nil {
		return 0, err
	}
	wanted := toSet(urls)
	retried := make(map[string]struct{})
	for _, e := range entries {
		if wanted != nil {
			if _, ok := wanted[e.URL]; !ok {
				continue
			}
		}
		if _, done := retried[e.URL]; done {
			continue
		}
		ok, err := c.opts.Frontier.Requeue(e.URL, e.Depth)
		if err != nil || !ok {
			continue
		}
		retried[e.URL] = struct{}{}
	}
	if len(retried) > 0 {
		if _, err := c.opts.DeadLetter.Remove(func(e deadletter.Entry) bool {
			_, ok := retried[e.URL]
			return ok
		}); err != nil {
			return len(retried), fmt.Errorf("URLs re-queued but dead-letter log not updated: %w", err)
		}
	}
	c.recountDeadLetters()
	c.logger.Info("dead-letter URLs re-queued by operator", "count", len(retried))
	return len(retried), nil
}

// DiscardDeadLetters deletes entries (all when urls is empty) from the log.
func (c *Controller) DiscardDeadLetters(urls []string) (int, error) {
	if c.opts.DeadLetter == nil {
		return 0, nil
	}
	wanted := toSet(urls)
	n, err := c.opts.DeadLetter.Remove(func(e deadletter.Entry) bool {
		if wanted == nil {
			return true
		}
		_, ok := wanted[e.URL]
		return ok
	})
	c.recountDeadLetters()
	return n, err
}

func (c *Controller) recountDeadLetters() {
	if c.opts.DeadLetter == nil {
		return
	}
	entries, err := c.opts.DeadLetter.Entries()
	if err != nil {
		c.logger.Warn("could not read dead-letter log", "error", err)
		return
	}
	c.dlMu.Lock()
	c.dlCount = len(entries)
	c.dlMu.Unlock()
}

// Pages lists stored pages.
func (c *Controller) Pages(ctx context.Context, q storage.PageQuery) (storage.PageList, error) {
	if c.opts.Pages == nil {
		return storage.PageList{}, ErrUnsupported
	}
	return c.opts.Pages.ListPages(ctx, q)
}

// PageDomains lists stored page counts per domain.
func (c *Controller) PageDomains(ctx context.Context) ([]storage.DomainCount, error) {
	if c.opts.Pages == nil {
		return nil, ErrUnsupported
	}
	return c.opts.Pages.DomainCounts(ctx)
}

// ResetOptions picks which pieces of state Reset clears.
type ResetOptions struct {
	Queue       bool `json:"queue"`        // queued URLs + visited set + checkpoint
	DeadLetters bool `json:"dead_letters"` // the failure log
	Pages       bool `json:"pages"`        // stored page metadata
	Stats       bool `json:"stats"`        // in-memory counters and activity feed
}

// ResetResult reports what Reset did.
type ResetResult struct {
	QueueCleared       bool  `json:"queue_cleared"`
	DeadLettersRemoved int   `json:"dead_letters_removed"`
	PagesDeleted       int64 `json:"pages_deleted"`
	StatsCleared       bool  `json:"stats_cleared"`
}

// Reset clears the selected state. Clearing the queue also forgets the
// visited set, so previously-crawled URLs may be crawled again.
func (c *Controller) Reset(ctx context.Context, o ResetOptions) (ResetResult, error) {
	var res ResetResult
	var errs []error

	if o.Queue {
		c.opts.Frontier.Reset()
		if c.opts.Checkpoint != nil {
			if err := c.opts.Checkpoint.Save(c.opts.Frontier.Snapshot()); err != nil {
				errs = append(errs, fmt.Errorf("saving empty checkpoint: %w", err))
			}
		}
		res.QueueCleared = true
	}
	if o.DeadLetters {
		n, err := c.DiscardDeadLetters(nil)
		if err != nil {
			errs = append(errs, fmt.Errorf("clearing dead letters: %w", err))
		}
		res.DeadLettersRemoved = n
	}
	if o.Pages {
		if c.opts.Purger == nil {
			errs = append(errs, fmt.Errorf("deleting pages: %w", ErrUnsupported))
		} else {
			n, err := c.opts.Purger.DeleteAllPages(ctx)
			if err != nil {
				errs = append(errs, err)
			}
			res.PagesDeleted = n
		}
	}
	if o.Stats {
		c.stats.Reset()
		res.StatsCleared = true
	}

	c.logger.Info("state reset by operator",
		"queue", o.Queue, "dead_letters", o.DeadLetters, "pages", o.Pages, "stats", o.Stats)
	return res, errors.Join(errs...)
}

func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(items))
	for _, s := range items {
		m[s] = struct{}{}
	}
	return m
}
