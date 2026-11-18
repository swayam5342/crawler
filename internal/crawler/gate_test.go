package crawler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/frontier"
)

func TestPauseGateTransitions(t *testing.T) {
	g := NewPauseGate(false)
	if g.Paused() || g.Resume() {
		t.Fatal("new open gate: Paused()/Resume() should be false")
	}
	if !g.Pause() || g.Pause() {
		t.Fatal("Pause should succeed once")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := g.Wait(ctx); err == nil {
		t.Fatal("Wait on paused gate should block until ctx expires")
	}

	done := make(chan error, 1)
	go func() { done <- g.Wait(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	if !g.Resume() {
		t.Fatal("Resume should succeed")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait() = %v after Resume", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after Resume")
	}
}

type countingFetcher struct{ n atomic.Int32 }

func (c *countingFetcher) Fetch(context.Context, string) ([]string, error) {
	c.n.Add(1)
	return nil, nil
}

type eventRecorder struct{ n atomic.Int32 }

func (e *eventRecorder) OnFetch(FetchEvent) { e.n.Add(1) }

func TestPausedPoolDoesNotFetchUntilResumed(t *testing.T) {
	f := frontier.New(nil, frontier.WithKeepAlive())
	gate := NewPauseGate(true)
	fetcher := &countingFetcher{}
	events := &eventRecorder{}
	pool := New(f, fetcher, Config{GlobalWorkers: 2, PerDomainLimit: 1, MaxDepth: 1, Gate: gate, Events: events})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { pool.Run(ctx); close(done) }()

	f.Add("https://example.com/", 0)
	time.Sleep(50 * time.Millisecond)
	if fetcher.n.Load() != 0 {
		t.Fatal("paused pool fetched a URL")
	}

	gate.Resume()
	deadline := time.Now().Add(2 * time.Second)
	for fetcher.n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if fetcher.n.Load() != 1 {
		t.Fatalf("fetches after resume = %d, want 1", fetcher.n.Load())
	}
	if events.n.Load() != 1 {
		t.Fatalf("events = %d, want 1", events.n.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pool did not stop on cancel")
	}
}
