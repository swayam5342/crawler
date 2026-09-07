// Package robots fetches, parses, and caches robots.txt files, exposing
// per-URL allow/disallow checks and Crawl-delay lookups.
package robots

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

// HTTPClient is the subset of *http.Client used to fetch robots.txt. It is
// an interface so tests can substitute a fake transport.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Checker fetches and caches robots.txt per host, and answers allow/disallow
// and crawl-delay questions against the cached rules.
type Checker struct {
	client    HTTPClient
	userAgent string
	ttl       time.Duration
	timeout   time.Duration

	mu    sync.Mutex
	cache map[string]*entry // key: scheme://host
}

type entry struct {
	mu        sync.Mutex
	data      *robotstxt.RobotsData
	fetchedAt time.Time
	group     *robotstxt.Group
}

// New creates a Checker. client may be nil to use http.DefaultClient.
func New(client HTTPClient, userAgent string, ttl, timeout time.Duration) *Checker {
	if client == nil {
		client = http.DefaultClient
	}
	return &Checker{
		client:    client,
		userAgent: userAgent,
		ttl:       ttl,
		timeout:   timeout,
		cache:     make(map[string]*entry),
	}
}

// Allowed reports whether rawURL may be fetched according to the target
// host's robots.txt. On fetch/parse failure, robots.txt is treated as
// absent and the URL is allowed (fail-open), per common crawler convention.
func (c *Checker) Allowed(ctx context.Context, rawURL string) (bool, error) {
	e, err := c.entryFor(ctx, rawURL)
	if err != nil {
		return true, err
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("parsing url: %w", err)
	}
	if e.group == nil {
		return true, nil
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return e.group.Test(path), nil
}

// CrawlDelay returns the Crawl-delay directive for rawURL's host, if any.
func (c *Checker) CrawlDelay(ctx context.Context, rawURL string) (delay time.Duration, ok bool, err error) {
	e, err := c.entryFor(ctx, rawURL)
	if err != nil {
		return 0, false, err
	}
	if e.group == nil || e.group.CrawlDelay <= 0 {
		return 0, false, nil
	}
	return e.group.CrawlDelay, true, nil
}

// entryFor returns the cached (or freshly fetched) robots.txt entry for
// rawURL's origin, refreshing it if the TTL has expired.
func (c *Checker) entryFor(ctx context.Context, rawURL string) (*entry, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	key := u.Scheme + "://" + u.Host

	c.mu.Lock()
	e, ok := c.cache[key]
	if !ok {
		e = &entry{}
		c.cache[key] = e
	}
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.data != nil && time.Since(e.fetchedAt) < c.ttl {
		return e, nil
	}

	data, ferr := c.fetch(ctx, u.Scheme, u.Host)
	e.fetchedAt = time.Now()
	if ferr != nil {
		// Keep serving stale data if we have any; otherwise e.data/e.group
		// are already nil, which Allowed() treats as fail-open.
		return e, nil
	}
	e.data = data
	if c.userAgent != "" {
		e.group = data.FindGroup(c.userAgent)
	} else {
		e.group = data.FindGroup("*")
	}
	return e, nil
}

func (c *Checker) fetch(ctx context.Context, scheme, host string) (*robotstxt.RobotsData, error) {
	reqCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	robotsURL := scheme + "://" + host + "/robots.txt"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building robots.txt request: %w", err)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching robots.txt: %w", err)
	}
	defer resp.Body.Close()

	// robotstxt.FromStatusAndBytes treats 4xx as "no restrictions" and 5xx
	// as "disallow all", matching common crawler conventions.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1 MiB
	if err != nil {
		return nil, fmt.Errorf("reading robots.txt body: %w", err)
	}
	data, err := robotstxt.FromStatusAndBytes(resp.StatusCode, body)
	if err != nil {
		return nil, fmt.Errorf("parsing robots.txt: %w", err)
	}
	return data, nil
}
