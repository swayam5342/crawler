package deadletter

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestRecordAndReadAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deadletter.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink() error: %v", err)
	}
	defer sink.Close()

	entries := []Entry{
		{URL: "https://example.com/a", Domain: "example.com", Attempts: 3, Error: "timeout"},
		{URL: "https://example.com/b", Domain: "example.com", Attempts: 4, Error: "connection reset"},
	}
	for _, e := range entries {
		if err := sink.Record(e); err != nil {
			t.Fatalf("Record() error: %v", err)
		}
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ReadAll() returned %d entries, want 2", len(got))
	}
	if got[0].URL != entries[0].URL || got[0].Error != entries[0].Error {
		t.Errorf("entry 0 = %+v, want %+v", got[0], entries[0])
	}
	if got[1].URL != entries[1].URL || got[1].Attempts != entries[1].Attempts {
		t.Errorf("entry 1 = %+v, want %+v", got[1], entries[1])
	}
	for _, e := range got {
		if e.FailedAt.IsZero() {
			t.Errorf("expected FailedAt to be auto-populated for %+v", e)
		}
	}
}

func TestReadAllMissingFileReturnsEmpty(t *testing.T) {
	got, err := ReadAll(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no entries for a missing file, got %v", got)
	}
}

func TestRecordIsSafeForConcurrentUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deadletter.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink() error: %v", err)
	}
	defer sink.Close()

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = sink.Record(Entry{URL: "https://example.com/x", Attempts: i})
		}(i)
	}
	wg.Wait()

	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if len(got) != n {
		t.Errorf("ReadAll() returned %d entries, want %d (concurrent writes must not corrupt lines)", len(got), n)
	}
}

func TestNewFileSinkCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "deadletter.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink() error: %v", err)
	}
	defer sink.Close()

	if err := sink.Record(Entry{URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Record() error: %v", err)
	}
	got, err := ReadAll(path)
	if err != nil || len(got) != 1 {
		t.Fatalf("ReadAll() = %v, %v; want 1 entry", got, err)
	}
}
