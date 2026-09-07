// Package crawler orchestrates the worker pool that drains the frontier,
// fetches pages, and feeds discovered links back in.
package crawler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/swayamsb50/web_crawler/internal/frontier"
	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// PageFetcher fetches a URL and returns the links discovered on the page.
// The concrete implementation (HTTP client with retries, timeouts, etc.)
// lives in the fetcher package and is added in a later phase.
type PageFetcher interface {
	Fetch(ctx context.Context, rawURL string) (links []string, err error)
}

// RobotsChecker answers robots.txt allow/disallow and Crawl-delay questions
// for a URL. The concrete implementation lives in the robots package.
type RobotsChecker interface {
	Allowed(ctx context.Context, rawURL string) (bool, error)
	CrawlDelay(ctx context.Context, rawURL string) (delay time.Duration, ok bool, err error)
}

// RateLimiter paces requests per domain. The concrete implementation lives
// in the ratelimit package.
type RateLimiter interface {
	Wait(ctx context.Context, domain string) error
	SetRate(domain string, requestsPerSecond float64, burst int)
}

// DeadLetter records URLs that permanently failed to fetch, so nothing is
// silently dropped. The concrete implementation lives in the deadletter
// package.
type DeadLetter interface {
	Record(url, domain string, depth, attempts int, cause error) error
}

// attemptCounter is implemented by fetch errors that know how many attempts
// were made (e.g. *fetcher.FetchError), detected structurally so this
// package doesn't need to import fetcher.
type attemptCounter interface {
	AttemptCount() int
}

// Pool drives a fixed number of worker goroutines that pull URLs from a
// Frontier, fetch them (subject to a per-domain concurrency cap), and push
// newly discovered links back into the Frontier.
type Pool struct {
	frontier       *frontier.Frontier
	fetcher        PageFetcher
	robots         RobotsChecker // optional
	limiter        RateLimiter   // optional
	deadLetter     DeadLetter    // optional
	globalWorkers  int
	perDomainLimit int
	maxDepth       int
	logger         *slog.Logger

	domainSemMu sync.Mutex
	domainSem   map[string]chan struct{}
}

// Config holds the tunables needed to construct a Pool.
type Config struct {
	GlobalWorkers  int
	PerDomainLimit int
	MaxDepth       int
	Logger         *slog.Logger

	// Robots, when set, is consulted before every fetch: disallowed URLs
	// are skipped, and a Crawl-delay directive overrides Limiter's rate
	// for that domain.
	Robots RobotsChecker
	// Limiter, when set, paces requests per domain before each fetch.
	Limiter RateLimiter
	// DeadLetter, when set, records URLs that permanently fail to fetch.
	DeadLetter DeadLetter
}

// New builds a Pool. f and fetcher must be non-nil.
func New(f *frontier.Frontier, fetcher PageFetcher, cfg Config) *Pool {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.GlobalWorkers <= 0 {
		cfg.GlobalWorkers = 1
	}
	if cfg.PerDomainLimit <= 0 {
		cfg.PerDomainLimit = 1
	}
	return &Pool{
		frontier:       f,
		fetcher:        fetcher,
		robots:         cfg.Robots,
		limiter:        cfg.Limiter,
		deadLetter:     cfg.DeadLetter,
		globalWorkers:  cfg.GlobalWorkers,
		perDomainLimit: cfg.PerDomainLimit,
		maxDepth:       cfg.MaxDepth,
		logger:         logger,
		domainSem:      make(map[string]chan struct{}),
	}
}

// Run starts globalWorkers goroutines that drain the frontier until it is
// closed and empty, or ctx is canceled. It blocks until all workers exit.
func (p *Pool) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range p.globalWorkers {
		wg.Go(func() {
			p.workerLoop(ctx)
		})
	}
	wg.Wait()
}

func (p *Pool) workerLoop(ctx context.Context) {
	for {
		item, ok := p.frontier.Next(ctx)
		if !ok {
			return
		}
		p.process(ctx, item)
	}
}

func (p *Pool) process(ctx context.Context, item frontier.Item) {
	defer p.frontier.Done(item.URL)

	domain, err := urlutil.Domain(item.URL)
	if err != nil {
		p.logger.Warn("could not derive domain, skipping", "url", item.URL, "error", err)
		return
	}

	if p.robots != nil {
		allowed, err := p.robots.Allowed(ctx, item.URL)
		if err != nil {
			p.logger.Warn("robots.txt check failed; allowing by default", "url", item.URL, "error", err)
		} else if !allowed {
			p.logger.Debug("blocked by robots.txt", "url", item.URL)
			return
		}
	}

	sem := p.domainSemaphore(domain)
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-sem }()

	if p.robots != nil && p.limiter != nil {
		if delay, ok, err := p.robots.CrawlDelay(ctx, item.URL); err == nil && ok {
			p.limiter.SetRate(domain, 1/delay.Seconds(), 1)
		}
	}
	if p.limiter != nil {
		if err := p.limiter.Wait(ctx, domain); err != nil {
			return
		}
	}

	links, err := p.fetcher.Fetch(ctx, item.URL)
	if err != nil {
		p.logger.Warn("fetch failed", "url", item.URL, "domain", domain, "error", err)
		if p.deadLetter != nil {
			attempts := 1
			if ac, ok := err.(attemptCounter); ok {
				attempts = ac.AttemptCount()
			}
			if derr := p.deadLetter.Record(item.URL, domain, item.Depth, attempts, err); derr != nil {
				p.logger.Warn("failed to record dead-letter entry", "url", item.URL, "error", derr)
			}
		}
		return
	}

	p.logger.Debug("fetched page", "url", item.URL, "domain", domain, "depth", item.Depth, "links_found", len(links))

	if item.Depth >= p.maxDepth {
		return
	}
	for _, link := range links {
		if _, err := p.frontier.Add(link, item.Depth+1); err != nil {
			p.logger.Debug("skipping invalid link", "url", link, "error", err)
		}
	}
}

func (p *Pool) domainSemaphore(domain string) chan struct{} {
	p.domainSemMu.Lock()
	defer p.domainSemMu.Unlock()
	sem, ok := p.domainSem[domain]
	if !ok {
		sem = make(chan struct{}, p.perDomainLimit)
		p.domainSem[domain] = sem
	}
	return sem
}
