package frontier

import (
	"context"
	"sort"
	"testing"
)

func TestSnapshotIncludesQueuedAndInFlightItems(t *testing.T) {
	f := New(nil)
	ctx := context.Background()

	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := f.Add("https://example.com/b", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dequeue "a" but don't call Done: it's now in-flight and must still
	// appear in the snapshot so a crash doesn't silently lose it.
	if _, ok := f.Next(ctx); !ok {
		t.Fatalf("expected Next() to succeed")
	}

	cp := f.Snapshot()
	urls := itemURLs(cp.Queue)
	sort.Strings(urls)
	want := []string{"https://example.com/a", "https://example.com/b"}
	if !equalStrings(urls, want) {
		t.Errorf("Snapshot().Queue URLs = %v, want %v", urls, want)
	}

	sort.Strings(cp.Visited)
	if !equalStrings(cp.Visited, want) {
		t.Errorf("Snapshot().Visited = %v, want %v", cp.Visited, want)
	}
}

func TestRestoreResumesWithoutDuplicateVisits(t *testing.T) {
	f1 := New(nil)
	if _, err := f1.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := f1.Add("https://example.com/b", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cp := f1.Snapshot()

	f2 := New(nil)
	f2.Restore(cp)

	// Re-adding an already-visited URL must be rejected (dedup state carried
	// over from the checkpoint).
	added, err := f2.Add("https://example.com/a", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Errorf("expected restored frontier to reject already-visited URL")
	}

	ctx := context.Background()
	got := map[string]bool{}
	for range 2 {
		item, ok := f2.Next(ctx)
		if !ok {
			t.Fatalf("expected Next() to succeed while resuming from checkpoint")
		}
		got[item.URL] = true
		f2.Done(item.URL)
	}
	if !got["https://example.com/a"] || !got["https://example.com/b"] {
		t.Errorf("expected both checkpointed URLs to be resumed, got %v", got)
	}
}

func TestRestoredFrontierAutoClosesAfterDraining(t *testing.T) {
	f1 := New(nil)
	if _, err := f1.Add("https://example.com/a", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cp := f1.Snapshot()

	f2 := New(nil)
	f2.Restore(cp)

	ctx := context.Background()
	item, ok := f2.Next(ctx)
	if !ok {
		t.Fatalf("expected Next() to succeed")
	}
	f2.Done(item.URL)

	if _, ok := f2.Next(ctx); ok {
		t.Fatalf("expected restored frontier to auto-close once drained")
	}
}

func itemURLs(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.URL
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
