package frontier

import (
	"path/filepath"
	"testing"
)

func TestFileCheckpointStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	store := FileCheckpointStore{Path: path}

	cp := Checkpoint{
		Queue:   []Item{{URL: "https://example.com/a", Depth: 0}, {URL: "https://example.com/b", Depth: 1}},
		Visited: []string{"https://example.com/a", "https://example.com/b"},
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, found, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !found {
		t.Fatalf("expected checkpoint to be found")
	}
	if len(got.Queue) != 2 || got.Queue[0].URL != "https://example.com/a" {
		t.Errorf("Load().Queue = %+v, want to match saved checkpoint", got.Queue)
	}
	if len(got.Visited) != 2 {
		t.Errorf("Load().Visited = %v, want 2 entries", got.Visited)
	}
}

func TestFileCheckpointStoreLoadMissingFile(t *testing.T) {
	store := FileCheckpointStore{Path: filepath.Join(t.TempDir(), "does-not-exist.json")}
	_, found, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Errorf("expected found=false for a missing checkpoint file")
	}
}

func TestFileCheckpointStoreOverwritesPreviousCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	store := FileCheckpointStore{Path: path}

	if err := store.Save(Checkpoint{Visited: []string{"https://example.com/old"}}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if err := store.Save(Checkpoint{Visited: []string{"https://example.com/new"}}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, found, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !found {
		t.Fatalf("expected checkpoint to be found")
	}
	if len(got.Visited) != 1 || got.Visited[0] != "https://example.com/new" {
		t.Errorf("Load().Visited = %v, want [https://example.com/new]", got.Visited)
	}
}

func TestFileCheckpointStoreCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "checkpoint.json")
	store := FileCheckpointStore{Path: path}

	if err := store.Save(Checkpoint{Visited: []string{"https://example.com/a"}}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if _, found, err := store.Load(); err != nil || !found {
		t.Fatalf("Load() = found=%v, err=%v; want found=true", found, err)
	}
}
