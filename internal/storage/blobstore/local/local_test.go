package local

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/swayamsb50/web_crawler/internal/storage/blobstore"
)

func TestPutAndGetRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	ctx := context.Background()

	if err := store.Put(ctx, "abc123", []byte("hello world")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	got, err := store.Get(ctx, "abc123")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("Get() = %q, want hello world", got)
	}
}

func TestGetMissingKeyReturnsErrNotFound(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	_, err = store.Get(context.Background(), "does-not-exist")
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestPutOverwritesExistingKey(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	ctx := context.Background()

	if err := store.Put(ctx, "key", []byte("first")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if err := store.Put(ctx, "key", []byte("second")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	got, err := store.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("Get() = %q, want second", got)
	}
}

func TestPathTraversalKeyRejected(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	ctx := context.Background()

	if err := store.Put(ctx, "../escape", []byte("data")); err == nil {
		t.Errorf("expected Put() to reject a path-traversal key")
	}
	if _, err := store.Get(ctx, "../../etc/passwd"); err == nil {
		t.Errorf("expected Get() to reject a path-traversal key")
	}
}

func TestNewCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "blobs")
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if err := store.Put(context.Background(), "k", []byte("v")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
}

func TestSupportsNestedKeys(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	ctx := context.Background()

	// Keys with path separators (e.g. sharded by hash prefix) should work.
	key := filepath.Join("ab", "abcdef1234567890")
	if err := store.Put(ctx, key, []byte("sharded")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != "sharded" {
		t.Errorf("Get() = %q, want sharded", got)
	}
}
