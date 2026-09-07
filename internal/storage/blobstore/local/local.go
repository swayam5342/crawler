// Package local implements blobstore.Store on the local filesystem.
package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swayamsb50/web_crawler/internal/storage/blobstore"
)

// Store stores each blob as a file under Dir, named by its key. Keys are
// expected to be filesystem-safe (e.g. hex-encoded hashes); callers should
// not pass arbitrary untrusted strings.
type Store struct {
	Dir string
}

// New creates a Store rooted at dir, creating it if necessary.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating blob store directory: %w", err)
	}
	return &Store{Dir: dir}, nil
}

// Put implements blobstore.Store.
func (s *Store) Put(_ context.Context, key string, data []byte) error {
	path, err := s.pathFor(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating blob subdirectory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing blob: %w", err)
	}
	return nil
}

// Get implements blobstore.Store.
func (s *Store) Get(_ context.Context, key string) ([]byte, error) {
	path, err := s.pathFor(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, blobstore.ErrNotFound
		}
		return nil, fmt.Errorf("reading blob: %w", err)
	}
	return data, nil
}

// pathFor rejects keys that would escape Dir (e.g. containing "..").
func (s *Store) pathFor(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("blob key must not be empty")
	}
	path := filepath.Join(s.Dir, key)
	rel, err := filepath.Rel(s.Dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid blob key %q", key)
	}
	return path, nil
}
