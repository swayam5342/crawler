// Package fetcher provides a resilient HTTP client for crawling: timeouts
// at every network layer, retry with exponential backoff and jitter,
// redirect loop detection, response size caps, transparent gzip/br
// decompression, non-UTF8 charset normalization, HTTP/2-to-1.1 fallback,
// and optional circuit breaker integration.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// Breaker gates and records outcomes of requests to a domain. The
// implementation lives in the breaker package.
type Breaker interface {
	Allow(domain string) bool
	RecordSuccess(domain string)
	RecordFailure(domain string)
}

// Config controls Client behavior.
type Config struct {
	UserAgent    string
	MaxBodyBytes int64
	MaxRedirects int

	DNSTimeout     time.Duration
	ConnectTimeout time.Duration
	TLSTimeout     time.Duration
	RequestTimeout time.Duration

	MaxAttempts  int
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	JitterFactor float64

	// Breaker is optional; when nil, no circuit breaking is applied.
	Breaker Breaker
}

// Result is a successfully fetched and decoded page.
type Result struct {
	FinalURL   string
	StatusCode int
	Header     http.Header
	// Body is the response body, decompressed and transcoded to UTF-8.
	Body []byte
}

// Client fetches URLs with retries, timeouts, and other resilience measures
// applied. It is safe for concurrent use.
type Client struct {
	cfg Config

	client *http.Client

	h1Mu   sync.Mutex
	h1Only map[string]bool // hosts that HTTP/2 has failed against
	h2RT   http.RoundTripper
	h1RT   http.RoundTripper
}

// New builds a Client from cfg, applying safe minimums for any zero-valued
// tunables.
func New(cfg Config) *Client {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 10 << 20
	}
	if cfg.MaxRedirects < 0 {
		cfg.MaxRedirects = 0
	}
	if cfg.DNSTimeout <= 0 {
		cfg.DNSTimeout = 5 * time.Second
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 5 * time.Second
	}
	if cfg.TLSTimeout <= 0 {
		cfg.TLSTimeout = 5 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	h2RT, h1RT := buildTransports(cfg.DNSTimeout, cfg.ConnectTimeout, cfg.TLSTimeout)

	c := &Client{
		cfg:    cfg,
		h1Only: make(map[string]bool),
		h2RT:   h2RT,
		h1RT:   h1RT,
	}

	c.client = &http.Client{
		Transport:     roundTripperFunc(c.roundTrip),
		CheckRedirect: c.checkRedirect(cfg.MaxRedirects),
	}

	return c
}

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// roundTrip picks the HTTP/2-capable transport unless this host has already
// been marked HTTP/1.1-only after a prior HTTP/2 failure.
func (c *Client) roundTrip(req *http.Request) (*http.Response, error) {
	c.h1Mu.Lock()
	forceH1 := c.h1Only[req.URL.Host]
	c.h1Mu.Unlock()

	rt := c.h2RT
	if forceH1 {
		rt = c.h1RT
	}

	resp, err := rt.RoundTrip(req)
	if err == nil || forceH1 {
		return resp, err
	}
	if looksLikeHTTP2Failure(err) {
		c.h1Mu.Lock()
		c.h1Only[req.URL.Host] = true
		c.h1Mu.Unlock()
		return c.h1RT.RoundTrip(req)
	}
	return resp, err
}

// looksLikeHTTP2Failure heuristically detects HTTP/2 protocol errors from
// the error text. Go's net/http vendors HTTP/2 support internally without
// exporting typed errors we could match with errors.As, so a substring
// check on the (short, library-generated) error message is the practical
// option here.
func looksLikeHTTP2Failure(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "http2")
}

// checkRedirect enforces a max-hop limit and detects redirect loops (a URL
// revisited within the same chain).
func (c *Client) checkRedirect(maxRedirects int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return ErrTooManyRedirects
		}
		target := req.URL.String()
		for _, prev := range via {
			if prev.URL.String() == target {
				return ErrRedirectLoop
			}
		}
		return nil
	}
}

// Fetch retrieves rawURL, retrying transient failures with exponential
// backoff and jitter, subject to an optional circuit breaker.
func (c *Client) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	domain, err := urlutil.Domain(rawURL)
	if err != nil {
		return nil, fmt.Errorf("determining domain: %w", err)
	}

	var lastErr error
	var attemptsMade int
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		attemptsMade = attempt
		if c.cfg.Breaker != nil && !c.cfg.Breaker.Allow(domain) {
			return nil, ErrCircuitOpen
		}

		result, status, retryAfter, err := c.attempt(ctx, rawURL)

		if c.cfg.Breaker != nil && isBreakerFailure(status, err) {
			c.cfg.Breaker.RecordFailure(domain)
		} else if c.cfg.Breaker != nil {
			c.cfg.Breaker.RecordSuccess(domain)
		}

		if err == nil && !isRetryableStatus(status) {
			if status < 200 || status >= 300 {
				return nil, &HTTPStatusError{StatusCode: status, Status: http.StatusText(status)}
			}
			return result, nil
		}

		lastErr = err
		if err == nil {
			lastErr = &HTTPStatusError{StatusCode: status, Status: http.StatusText(status)}
		}

		retryable := (err != nil && isRetryableError(err)) || (err == nil && isRetryableStatus(status))
		if !retryable || attempt == c.cfg.MaxAttempts {
			break
		}

		wait := backoffDuration(attempt, c.cfg.BaseBackoff, c.cfg.MaxBackoff, c.cfg.JitterFactor)
		if retryAfter > 0 {
			wait = retryAfter
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, &FetchError{URL: rawURL, Attempts: attemptsMade, Err: lastErr}
}

// attempt performs a single fetch attempt: request, redirect-following,
// decompression, size capping, and charset normalization. It returns the
// HTTP status code even on error paths where one was obtained, so the
// caller can classify retryability and drive the circuit breaker.
func (c *Client) attempt(ctx context.Context, rawURL string) (result *Result, status int, retryAfter time.Duration, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("building request: %w", err)
	}
	if c.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", c.cfg.UserAgent)
	}
	req.Header.Set("Accept-Encoding", "gzip, br")

	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrTooManyRedirects) || errors.Is(err, ErrRedirectLoop) {
			return nil, 0, 0, err
		}
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	status = resp.StatusCode
	if isRetryableStatus(status) {
		retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	if status < 200 || status >= 300 {
		// Drain a bounded amount so the connection can be reused, but don't
		// bother decoding a body we're going to discard.
		io.CopyN(io.Discard, resp.Body, 4096) //nolint:errcheck
		return nil, status, retryAfter, nil
	}

	decoded, err := decompress(resp.Body, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, status, 0, err
	}

	limited := io.LimitReader(decoded, c.cfg.MaxBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, status, 0, fmt.Errorf("reading body: %w", err)
	}
	if int64(len(raw)) > c.cfg.MaxBodyBytes {
		return nil, status, 0, ErrBodyTooLarge
	}

	utf8Body, err := toUTF8(raw, resp.Header.Get("Content-Type"))
	if err != nil {
		// Malformed/undeclared charset: fall back to the raw bytes rather
		// than failing the whole fetch.
		utf8Body = raw
	}

	return &Result{
		FinalURL:   resp.Request.URL.String(),
		StatusCode: status,
		Header:     resp.Header,
		Body:       utf8Body,
	}, status, 0, nil
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		return time.Until(when)
	}
	return 0
}
