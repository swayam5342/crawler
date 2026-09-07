// Package crawler orchestrates the worker pool that drains the frontier,
// fetches pages, and feeds discovered links back in.
package crawler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/swayamsb50/web_crawler/internal/frontier"
	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// PageFetcher fetches a URL and returns the links discovered on the page.
// The concrete implementation (HTTP client with retries, timeouts, etc.)
// lives in the fetcher package and is added in a later phase.
type PageFetcher interface {
	Fetch(ctx context.Context, rawURL string) (links []string, err error)
}

// Pool drives a fixed number of worker goroutines that pull URLs from a
// Frontier, fetch them (subject to a per-domain concurrency cap), and push
// newly discovered links back into the Frontier.
type Pool struct {
	frontier       *frontier.Frontier
	fetcher        PageFetcher
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
	defer p.frontier.Done()

	domain, err := urlutil.Domain(item.URL)
	if err != nil {
		p.logger.Warn("could not derive domain, skipping", "url", item.URL, "error", err)
		return
	}

	sem := p.domainSemaphore(domain)
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-sem }()

	links, err := p.fetcher.Fetch(ctx, item.URL)
	if err != nil {
		p.logger.Warn("fetch failed", "url", item.URL, "domain", domain, "error", err)
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
