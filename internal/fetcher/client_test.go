package fetcher

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		UserAgent:      "test-bot/1.0",
		MaxBodyBytes:   1 << 20,
		MaxRedirects:   5,
		DNSTimeout:     2 * time.Second,
		ConnectTimeout: 2 * time.Second,
		TLSTimeout:     2 * time.Second,
		RequestTimeout: 2 * time.Second,
		MaxAttempts:    3,
		BaseBackoff:    5 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		JitterFactor:   0,
	}
}

func TestFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	c := New(testConfig())
	res, err := c.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", res.StatusCode)
	}
	if string(res.Body) != "hello world" {
		t.Errorf("Body = %q, want %q", res.Body, "hello world")
	}
}

func TestFetchRetriesOn5xxThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(testConfig())
	res, err := c.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Body) != "ok" {
		t.Errorf("Body = %q, want ok", res.Body)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("hits = %d, want 3", hits)
	}
}

func TestFetchGivesUpAfterMaxAttempts(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.MaxAttempts = 3
	c := New(cfg)
	_, err := c.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatalf("expected error after exhausting retries")
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("hits = %d, want 3 (MaxAttempts)", hits)
	}
}

func TestFetchDoesNotRetry4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(testConfig())
	_, err := c.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatalf("expected error for 404")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("hits = %d, want 1 (no retry on 404)", hits)
	}
}

func TestFetchRespectsMaxRedirects(t *testing.T) {
	var mux http.ServeMux
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hop1", http.StatusFound)
	})
	mux.HandleFunc("/hop1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hop2", http.StatusFound)
	})
	mux.HandleFunc("/hop2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hop3", http.StatusFound)
	})
	mux.HandleFunc("/hop3", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("too far"))
	})

	cfg := testConfig()
	cfg.MaxRedirects = 2
	cfg.MaxAttempts = 1
	c := New(cfg)
	_, err := c.Fetch(context.Background(), srv.URL+"/start")
	if err == nil {
		t.Fatalf("expected too-many-redirects error")
	}
	if !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("error = %v, want redirect-limit error", err)
	}
}

func TestFetchDetectsRedirectLoop(t *testing.T) {
	var mux http.ServeMux
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/a", http.StatusFound)
	})

	cfg := testConfig()
	cfg.MaxRedirects = 10
	cfg.MaxAttempts = 1
	c := New(cfg)
	_, err := c.Fetch(context.Background(), srv.URL+"/a")
	if err == nil {
		t.Fatalf("expected redirect loop error")
	}
	if !strings.Contains(err.Error(), "redirect loop") {
		t.Errorf("error = %v, want redirect loop error", err)
	}
}

func TestFetchEnforcesMaxBodySize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), 1000))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.MaxBodyBytes = 100
	cfg.MaxAttempts = 1
	c := New(cfg)
	_, err := c.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatalf("expected body-too-large error")
	}
	if !strings.Contains(err.Error(), "exceeds max size") {
		t.Errorf("error = %v, want body-too-large error", err)
	}
}

func TestFetchDecompressesGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		gw.Write([]byte("decompressed content"))
		gw.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	c := New(testConfig())
	res, err := c.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Body) != "decompressed content" {
		t.Errorf("Body = %q, want decompressed content", res.Body)
	}
}

func TestFetchHandlesUncompressedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain text"))
	}))
	defer srv.Close()

	c := New(testConfig())
	res, err := c.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Body) != "plain text" {
		t.Errorf("Body = %q, want plain text", res.Body)
	}
}

func TestFetchTranscodesNonUTF8Charset(t *testing.T) {
	// "café" in ISO-8859-1 (Latin-1): 'é' encodes as single byte 0xE9.
	latin1Body := []byte("caf\xe9")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
		w.Write(latin1Body)
	}))
	defer srv.Close()

	c := New(testConfig())
	res, err := c.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Body) != "café" {
		t.Errorf("Body = %q, want café (transcoded to UTF-8)", res.Body)
	}
}

func TestFetchSendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.UserAgent = "my-crawler/2.0"
	c := New(cfg)
	if _, err := c.Fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUA != "my-crawler/2.0" {
		t.Errorf("User-Agent = %q, want my-crawler/2.0", gotUA)
	}
}

func TestFetchRespectsContextCancellation(t *testing.T) {
	blockCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh
	}))
	defer srv.Close()
	defer close(blockCh)

	cfg := testConfig()
	cfg.RequestTimeout = 10 * time.Second
	cfg.MaxAttempts = 1
	c := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := c.Fetch(ctx, srv.URL)
	if err == nil {
		t.Fatalf("expected error from context cancellation")
	}
}

// erroringBreaker always blocks, for testing that Fetch honors it.
type blockingBreaker struct{}

func (blockingBreaker) Allow(string) bool    { return false }
func (blockingBreaker) RecordSuccess(string) {}
func (blockingBreaker) RecordFailure(string) {}

func TestFetchHonorsCircuitBreaker(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.Breaker = blockingBreaker{}
	c := New(cfg)

	_, err := c.Fetch(context.Background(), srv.URL)
	if err != ErrCircuitOpen {
		t.Fatalf("error = %v, want ErrCircuitOpen", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("hits = %d, want 0 (breaker should have blocked the request)", hits)
	}
}

// recordingBreaker tracks Allow/Success/Failure calls per domain.
type recordingBreaker struct {
	successes []string
	failures  []string
}

func (b *recordingBreaker) Allow(string) bool { return true }
func (b *recordingBreaker) RecordSuccess(domain string) {
	b.successes = append(b.successes, domain)
}
func (b *recordingBreaker) RecordFailure(domain string) {
	b.failures = append(b.failures, domain)
}

func TestFetchRecordsBreakerSuccessAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	breaker := &recordingBreaker{}
	cfg := testConfig()
	cfg.Breaker = breaker
	c := New(cfg)

	if _, err := c.Fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(breaker.successes) != 1 {
		t.Errorf("expected 1 success record, got %v", breaker.successes)
	}
	if len(breaker.failures) != 0 {
		t.Errorf("expected 0 failure records, got %v", breaker.failures)
	}
}

func TestFetch4xxDoesNotRecordBreakerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	breaker := &recordingBreaker{}
	cfg := testConfig()
	cfg.Breaker = breaker
	cfg.MaxAttempts = 1
	c := New(cfg)

	if _, err := c.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatalf("expected error for 404")
	}
	if len(breaker.failures) != 0 {
		t.Errorf("expected 404 to not count as a breaker failure, got %v", breaker.failures)
	}
	if len(breaker.successes) != 1 {
		t.Errorf("expected 404 (server reachable) to count as a breaker success, got %v", breaker.successes)
	}
}
