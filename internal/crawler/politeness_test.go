package crawler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/frontier"
)

// fakeRobots blocks a fixed set of URLs and reports a fixed crawl-delay for
// every domain.
type fakeRobots struct {
	disallowed map[string]bool
	crawlDelay time.Duration
}

func (r *fakeRobots) Allowed(_ context.Context, rawURL string) (bool, error) {
	return !r.disallowed[rawURL], nil
}

func (r *fakeRobots) CrawlDelay(_ context.Context, _ string) (time.Duration, bool, error) {
	if r.crawlDelay <= 0 {
		return 0, false, nil
	}
	return r.crawlDelay, true, nil
}

// recordingLimiter records SetRate calls and Wait calls without actually
// blocking, so tests can assert on what the pool told it to do.
type recordingLimiter struct {
	mu       sync.Mutex
	waits    []string
	setRates []setRateCall
}

type setRateCall struct {
	domain string
	rps    float64
	burst  int
}

func (l *recordingLimiter) Wait(_ context.Context, domain string) error {
	l.mu.Lock()
	l.waits = append(l.waits, domain)
	l.mu.Unlock()
	return nil
}

func (l *recordingLimiter) SetRate(domain string, rps float64, burst int) {
	l.mu.Lock()
	l.setRates = append(l.setRates, setRateCall{domain, rps, burst})
	l.mu.Unlock()
}

func TestPoolSkipsURLsDisallowedByRobots(t *testing.T) {
	fetcher := &fakeFetcher{
		graph: map[string][]string{
			"https://example.com/":        {"https://example.com/allowed", "https://example.com/blocked"},
			"https://example.com/allowed": nil,
			"https://example.com/blocked": nil,
		},
	}
	robots := &fakeRobots{disallowed: map[string]bool{"https://example.com/blocked": true}}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 2, PerDomainLimit: 2, MaxDepth: 5, Robots: robots})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	got := fetcher.fetchedURLs()
	for _, u := range got {
		if u == "https://example.com/blocked" {
			t.Fatalf("robots-disallowed URL was fetched; fetched: %v", got)
		}
	}
	found := false
	for _, u := range got {
		if u == "https://example.com/allowed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected allowed URL to be fetched; fetched: %v", got)
	}
}

func TestPoolAppliesCrawlDelayToLimiter(t *testing.T) {
	fetcher := &fakeFetcher{
		graph: map[string][]string{
			"https://example.com/": nil,
		},
	}
	robots := &fakeRobots{crawlDelay: 2 * time.Second}
	limiter := &recordingLimiter{}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, Robots: robots, Limiter: limiter})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.setRates) != 1 {
		t.Fatalf("expected exactly one SetRate call, got %d: %v", len(limiter.setRates), limiter.setRates)
	}
	call := limiter.setRates[0]
	if call.domain != "example.com" {
		t.Errorf("SetRate domain = %q, want example.com", call.domain)
	}
	wantRPS := 1.0 / 2.0 // 1 request per 2s crawl-delay
	if call.rps != wantRPS {
		t.Errorf("SetRate rps = %v, want %v", call.rps, wantRPS)
	}
	if len(limiter.waits) != 1 || limiter.waits[0] != "example.com" {
		t.Errorf("expected Wait to be called once for example.com, got %v", limiter.waits)
	}
}

func TestPoolDoesNotApplyCrawlDelayWhenAbsent(t *testing.T) {
	fetcher := &fakeFetcher{graph: map[string][]string{"https://example.com/": nil}}
	robots := &fakeRobots{} // no crawl-delay configured
	limiter := &recordingLimiter{}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, Robots: robots, Limiter: limiter})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.setRates) != 0 {
		t.Errorf("expected no SetRate calls when robots.txt has no crawl-delay, got %v", limiter.setRates)
	}
	if len(limiter.waits) != 1 {
		t.Errorf("expected Wait to still be called once, got %v", limiter.waits)
	}
}
