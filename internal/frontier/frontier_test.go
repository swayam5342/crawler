package frontier

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAddDeduplicates(t *testing.T) {
	f := New(nil)

	added, err := f.Add("https://example.com/a", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Fatalf("expected first add to succeed")
	}

	added, err = f.Add("https://Example.com/a/", 0) // normalizes to the same URL
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Fatalf("expected duplicate add to be rejected")
	}

	if f.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", f.Len())
	}
	if f.Visited() != 1 {
		t.Fatalf("Visited() = %d, want 1", f.Visited())
	}
}

func TestAddRejectsInvalidURL(t *testing.T) {
	f := New(nil)
	_, err := f.Add("not a url", 0)
	if err == nil {
		t.Fatalf("expected error for invalid url")
	}
}

type denyAllFilter struct{}

func (denyAllFilter) Allowed(string) bool { return false }

func TestAddRespectsFilter(t *testing.T) {
	f := New(denyAllFilter{})
	added, err := f.Add("https://example.com/a", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Fatalf("expected filtered URL to be rejected")
	}
	if f.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", f.Len())
	}
}

func TestNextFIFOOrder(t *testing.T) {
	f := New(nil)
	urls := []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"}
	for _, u := range urls {
		if _, err := f.Add(u, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	ctx := context.Background()
	for _, want := range urls {
		item, ok := f.Next(ctx)
		if !ok {
			t.Fatalf("Next() returned ok=false, want true")
		}
		if item.URL != want {
			t.Errorf("Next() = %q, want %q", item.URL, want)
		}
	}
}

func TestNextBlocksThenReceivesAdd(t *testing.T) {
	f := New(nil)
	ctx := context.Background()

	type result struct {
		item Item
		ok   bool
	}
	resCh := make(chan result, 1)
	go func() {
		item, ok := f.Next(ctx)
		resCh <- result{item, ok}
	}()

	time.Sleep(20 * time.Millisecond) // let the goroutine block in Next
	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case res := <-resCh:
		if !res.ok || res.item.URL != "https://example.com/a" {
			t.Fatalf("got %+v, want ok=true url=https://example.com/a", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Next() to unblock")
	}
}

func TestNextRespectsContextCancellation(t *testing.T) {
	f := New(nil)
	ctx, cancel := context.WithCancel(context.Background())

	doneCh := make(chan bool, 1)
	go func() {
		_, ok := f.Next(ctx)
		doneCh <- ok
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case ok := <-doneCh:
		if ok {
			t.Fatal("expected ok=false after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Next() to return after cancel")
	}
}

func TestCloseUnblocksAllWaiters(t *testing.T) {
	f := New(nil)
	ctx := context.Background()

	const numWaiters = 5
	var wg sync.WaitGroup
	results := make([]bool, numWaiters)
	for i := range numWaiters {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, ok := f.Next(ctx)
			results[i] = ok
		}(i)
	}

	time.Sleep(20 * time.Millisecond)
	f.Close()

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for all Next() calls to unblock after Close()")
	}

	for i, ok := range results {
		if ok {
			t.Errorf("waiter %d: ok = true, want false after Close() with empty queue", i)
		}
	}
}

func TestAddAfterCloseIsNoop(t *testing.T) {
	f := New(nil)
	f.Close()
	added, err := f.Add("https://example.com/a", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Fatalf("expected Add after Close to be rejected")
	}
}
