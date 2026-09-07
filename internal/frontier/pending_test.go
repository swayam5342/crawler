package frontier

import (
	"context"
	"testing"
	"time"
)

func TestDoneAutoClosesWhenPendingReachesZero(t *testing.T) {
	f := New(nil)
	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	item, ok := f.Next(ctx)
	if !ok || item.URL != "https://example.com/a" {
		t.Fatalf("Next() = %+v, %v; want a, true", item, ok)
	}

	f.Done(item.URL)

	// Frontier should now be auto-closed with nothing left: Next returns false.
	if _, ok := f.Next(ctx); ok {
		t.Fatalf("expected Next() to return ok=false after Done() drains pending work")
	}
}

func TestDoneDoesNotCloseWhileWorkStillPending(t *testing.T) {
	f := New(nil)
	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := f.Add("https://example.com/b", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	first, ok := f.Next(ctx)
	if !ok {
		t.Fatalf("expected first Next() to succeed")
	}
	f.Done(first.URL) // one of two pending items done; frontier must stay open

	item, ok := f.Next(ctx)
	if !ok {
		t.Fatalf("expected second Next() to still succeed before Done()")
	}
	if item.URL != "https://example.com/b" {
		t.Fatalf("got %q, want https://example.com/b", item.URL)
	}
}

func TestFollowUpAddBeforeDoneKeepsFrontierOpen(t *testing.T) {
	f := New(nil)
	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	item, ok := f.Next(ctx)
	if !ok {
		t.Fatalf("expected Next() to succeed")
	}

	// Simulate discovering a link while processing item, before calling Done.
	if _, err := f.Add("https://example.com/b", item.Depth+1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f.Done(item.URL) // completes "a", but "b" is now pending

	next, ok := f.Next(ctx)
	if !ok || next.URL != "https://example.com/b" {
		t.Fatalf("Next() = %+v, %v; want b, true", next, ok)
	}
	f.Done(next.URL)

	if _, ok := f.Next(ctx); ok {
		t.Fatalf("expected frontier to auto-close once all pending work is Done")
	}
}

func TestDoneUnblocksWaitingWorkers(t *testing.T) {
	f := New(nil)
	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	item, ok := f.Next(ctx)
	if !ok {
		t.Fatalf("expected Next() to succeed")
	}

	doneCh := make(chan bool, 1)
	go func() {
		_, ok := f.Next(ctx)
		doneCh <- ok
	}()

	time.Sleep(20 * time.Millisecond)
	f.Done(item.URL) // pending drops to 0: auto-close should wake the blocked Next()

	select {
	case ok := <-doneCh:
		if ok {
			t.Fatal("expected blocked Next() to return ok=false after auto-close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked Next() to unblock after Done()")
	}
}
