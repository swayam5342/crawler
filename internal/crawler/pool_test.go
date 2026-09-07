package crawler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/frontier"
)

// fakeFetcher returns a fixed link graph and records fetched URLs.
type fakeFetcher struct {
	graph map[string][]string

	mu      sync.Mutex
	fetched []string
}

func (f *fakeFetcher) Fetch(_ context.Context, rawURL string) ([]string, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, rawURL)
	f.mu.Unlock()
	return f.graph[rawURL], nil
}

func (f *fakeFetcher) fetchedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.fetched))
	copy(out, f.fetched)
	return out
}

func TestPoolCrawlsReachableGraph(t *testing.T) {
	fetcher := &fakeFetcher{
		graph: map[string][]string{
			"https://example.com/":  {"https://example.com/a", "https://example.com/b"},
			"https://example.com/a": {"https://example.com/c"},
			"https://example.com/b": {"https://example.com/c"}, // shared child: must be fetched once
			"https://example.com/c": nil,
		},
	}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 4, PerDomainLimit: 2, MaxDepth: 10})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	got := fetcher.fetchedURLs()
	want := map[string]int{
		"https://example.com/":  1,
		"https://example.com/a": 1,
		"https://example.com/b": 1,
		"https://example.com/c": 1,
	}
	counts := map[string]int{}
	for _, u := range got {
		counts[u]++
	}
	for url, wantCount := range want {
		if counts[url] != wantCount {
			t.Errorf("fetch count for %s = %d, want %d (fetched: %v)", url, counts[url], wantCount, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("total fetches = %d, want %d (fetched: %v)", len(got), len(want), got)
	}
}

func TestPoolRespectsMaxDepth(t *testing.T) {
	fetcher := &fakeFetcher{
		graph: map[string][]string{
			"https://example.com/":  {"https://example.com/a"},
			"https://example.com/a": {"https://example.com/b"},
			"https://example.com/b": {"https://example.com/c"},
		},
	}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 2, PerDomainLimit: 1, MaxDepth: 1})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	got := fetcher.fetchedURLs()
	// depth 0: "/", depth 1: "a" is fetched but its links aren't followed
	// (depth 1 >= MaxDepth), so "b" and "c" must never be fetched.
	for _, u := range got {
		if u == "https://example.com/b" || u == "https://example.com/c" {
			t.Errorf("fetched %s beyond max depth; fetched list: %v", u, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("total fetches = %d, want 2 (fetched: %v)", len(got), got)
	}
}

func TestPoolStopsOnContextCancel(t *testing.T) {
	blockCh := make(chan struct{})
	fetcher := &blockingFetcher{blockCh: blockCh}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5})

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	go func() {
		pool.Run(ctx)
		close(doneCh)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Pool.Run did not return after context cancellation")
	}
	close(blockCh)
}

type blockingFetcher struct {
	blockCh chan struct{}
}

func (b *blockingFetcher) Fetch(ctx context.Context, _ string) ([]string, error) {
	select {
	case <-b.blockCh:
	case <-ctx.Done():
	}
	return nil, ctx.Err()
}
