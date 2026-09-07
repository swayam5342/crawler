package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/frontier"
)

// failingFetcher always fails; if withAttempts is set, its error reports an
// attempt count via AttemptCount(), mimicking *fetcher.FetchError.
type failingFetcher struct {
	err error
}

func (f *failingFetcher) Fetch(_ context.Context, _ string) ([]string, error) {
	return nil, f.err
}

type fakeFetchError struct {
	attempts int
}

func (e *fakeFetchError) Error() string     { return "fake fetch failure" }
func (e *fakeFetchError) AttemptCount() int { return e.attempts }

type recordingDeadLetter struct {
	mu      sync.Mutex
	records []deadLetterRecord
}

type deadLetterRecord struct {
	url      string
	domain   string
	depth    int
	attempts int
	cause    error
}

func (d *recordingDeadLetter) Record(url, domain string, depth, attempts int, cause error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records = append(d.records, deadLetterRecord{url, domain, depth, attempts, cause})
	return nil
}

func TestPoolRecordsDeadLetterOnFetchFailure(t *testing.T) {
	dl := &recordingDeadLetter{}
	fetcher := &failingFetcher{err: &fakeFetchError{attempts: 4}}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, DeadLetter: dl})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	dl.mu.Lock()
	defer dl.mu.Unlock()
	if len(dl.records) != 1 {
		t.Fatalf("expected 1 dead-letter record, got %d", len(dl.records))
	}
	rec := dl.records[0]
	if rec.url != "https://example.com/" {
		t.Errorf("url = %q, want https://example.com/", rec.url)
	}
	if rec.domain != "example.com" {
		t.Errorf("domain = %q, want example.com", rec.domain)
	}
	if rec.attempts != 4 {
		t.Errorf("attempts = %d, want 4 (from AttemptCount())", rec.attempts)
	}
}

func TestPoolDeadLetterDefaultsAttemptsToOneWithoutAttemptCounter(t *testing.T) {
	dl := &recordingDeadLetter{}
	fetcher := &failingFetcher{err: errors.New("plain error, no AttemptCount")}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, DeadLetter: dl})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	dl.mu.Lock()
	defer dl.mu.Unlock()
	if len(dl.records) != 1 || dl.records[0].attempts != 1 {
		t.Fatalf("expected 1 dead-letter record with attempts=1, got %+v", dl.records)
	}
}

func TestPoolDoesNotRecordDeadLetterOnSuccess(t *testing.T) {
	dl := &recordingDeadLetter{}
	fetcher := &fakeFetcher{graph: map[string][]string{"https://example.com/": nil}}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, DeadLetter: dl})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	dl.mu.Lock()
	defer dl.mu.Unlock()
	if len(dl.records) != 0 {
		t.Errorf("expected no dead-letter records on success, got %v", dl.records)
	}
}
