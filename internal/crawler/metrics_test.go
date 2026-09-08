package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/frontier"
)

type recordingMetrics struct {
	mu      sync.Mutex
	records []metricsRecord
}

type metricsRecord struct {
	domain  string
	success bool
}

func (m *recordingMetrics) RecordFetch(domain string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, metricsRecord{domain, success})
}

func TestPoolRecordsMetricsOnSuccess(t *testing.T) {
	metrics := &recordingMetrics{}
	fetcher := &fakeFetcher{graph: map[string][]string{"https://example.com/": nil}}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, Metrics: metrics})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.records) != 1 || !metrics.records[0].success || metrics.records[0].domain != "example.com" {
		t.Fatalf("expected 1 successful record for example.com, got %+v", metrics.records)
	}
}

func TestPoolRecordsMetricsOnFailure(t *testing.T) {
	metrics := &recordingMetrics{}
	fetcher := &failingFetcher{err: errors.New("boom")}

	f := frontier.New(nil)
	if _, err := f.Add("https://example.com/", 0); err != nil {
		t.Fatalf("seed add failed: %v", err)
	}

	pool := New(f, fetcher, Config{GlobalWorkers: 1, PerDomainLimit: 1, MaxDepth: 5, Metrics: metrics})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Run(ctx)

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.records) != 1 || metrics.records[0].success {
		t.Fatalf("expected 1 failed record, got %+v", metrics.records)
	}
}
