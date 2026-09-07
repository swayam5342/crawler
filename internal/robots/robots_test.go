package robots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T, robotsBody string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(robotsBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAllowedRespectsDisallow(t *testing.T) {
	srv := newTestServer(t, "User-agent: *\nDisallow: /private/\n", http.StatusOK)
	c := New(srv.Client(), "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	allowed, err := c.Allowed(ctx, srv.URL+"/public/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected /public/page to be allowed")
	}

	allowed, err = c.Allowed(ctx, srv.URL+"/private/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("expected /private/page to be disallowed")
	}
}

func TestAllowedUsesSpecificUserAgentGroup(t *testing.T) {
	body := "User-agent: test-bot\nDisallow: /secret/\n\nUser-agent: *\nDisallow: /\n"
	srv := newTestServer(t, body, http.StatusOK)
	c := New(srv.Client(), "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	// test-bot's group only disallows /secret/, not everything, unlike the
	// wildcard group.
	allowed, err := c.Allowed(ctx, srv.URL+"/public/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected test-bot-specific group to allow /public/page")
	}
}

func TestCrawlDelay(t *testing.T) {
	srv := newTestServer(t, "User-agent: *\nCrawl-delay: 2\n", http.StatusOK)
	c := New(srv.Client(), "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	delay, ok, err := c.CrawlDelay(ctx, srv.URL+"/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected crawl-delay to be present")
	}
	if delay != 2*time.Second {
		t.Errorf("CrawlDelay = %v, want 2s", delay)
	}
}

func TestNoCrawlDelayWhenAbsent(t *testing.T) {
	srv := newTestServer(t, "User-agent: *\nDisallow:\n", http.StatusOK)
	c := New(srv.Client(), "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	_, ok, err := c.CrawlDelay(ctx, srv.URL+"/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected no crawl-delay to be reported")
	}
}

func TestMissingRobotsTxtFailsOpen(t *testing.T) {
	srv := newTestServer(t, "not found", http.StatusNotFound)
	c := New(srv.Client(), "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	allowed, err := c.Allowed(ctx, srv.URL+"/anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected fail-open (allowed) when robots.txt is 404")
	}
}

func TestFetchErrorFailsOpen(t *testing.T) {
	c := New(&erroringClient{}, "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	allowed, err := c.Allowed(ctx, "https://unreachable.invalid/page")
	if err != nil {
		t.Fatalf("Allowed should not surface fetch errors: %v", err)
	}
	if !allowed {
		t.Errorf("expected fail-open (allowed) on fetch error")
	}
}

type erroringClient struct{}

func (erroringClient) Do(_ *http.Request) (*http.Response, error) {
	return nil, context.DeadlineExceeded
}

func TestCachingAvoidsRefetchWithinTTL(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("User-agent: *\nDisallow:\n"))
	}))
	t.Cleanup(srv.Close)

	c := New(srv.Client(), "test-bot", time.Hour, 5*time.Second)
	ctx := context.Background()

	for range 5 {
		if _, err := c.Allowed(ctx, srv.URL+"/page"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if hits != 1 {
		t.Errorf("expected robots.txt to be fetched once within TTL, got %d fetches", hits)
	}
}

func TestCacheRefreshesAfterTTLExpires(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("User-agent: *\nDisallow:\n"))
	}))
	t.Cleanup(srv.Close)

	c := New(srv.Client(), "test-bot", 10*time.Millisecond, 5*time.Second)
	ctx := context.Background()

	if _, err := c.Allowed(ctx, srv.URL+"/page"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := c.Allowed(ctx, srv.URL+"/page"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 2 {
		t.Errorf("expected robots.txt to be refetched after TTL expiry, got %d fetches", hits)
	}
}
