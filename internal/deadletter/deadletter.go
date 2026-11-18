// Package deadletter records URLs that exhausted their retry budget (or
// were otherwise permanently unfetchable) so they are never silently
// dropped, and can be inspected or replayed later.
package deadletter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one permanently-failed URL.
type Entry struct {
	URL      string    `json:"url"`
	Domain   string    `json:"domain"`
	Depth    int       `json:"depth"`
	Attempts int       `json:"attempts"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
}

// Sink records dead-letter entries. Implementations must be safe for
// concurrent use.
type Sink interface {
	Record(e Entry) error
}

// FileSink appends entries as newline-delimited JSON (JSONL) to a file, so
// nothing is silently lost and the log can be tailed or replayed. It is
// safe for concurrent use.
type FileSink struct {
	mu   sync.Mutex
	path string
	file *os.File
}

// NewFileSink opens (creating if necessary) path for appending.
func NewFileSink(path string) (*FileSink, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating dead-letter directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening dead-letter file: %w", err)
	}
	return &FileSink{path: path, file: f}, nil
}

// Entries reads back every entry recorded so far.
func (s *FileSink) Entries() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ReadAll(s.path)
}

// Remove rewrites the log without the entries for which drop returns
// true, and returns how many were removed. The rewrite goes through a
// temp file + rename so a crash mid-rewrite cannot lose the log.
func (s *FileSink) Remove(drop func(Entry) bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := ReadAll(s.path)
	if err != nil {
		return 0, err
	}
	kept := entries[:0]
	removed := 0
	for _, e := range entries {
		if drop(e) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	if removed == 0 {
		return 0, nil
	}

	var buf []byte
	for _, e := range kept {
		line, err := json.Marshal(e)
		if err != nil {
			return 0, fmt.Errorf("marshaling dead-letter entry: %w", err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".deadletter-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("creating temp dead-letter file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after a successful rename
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		return 0, fmt.Errorf("writing temp dead-letter file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("closing temp dead-letter file: %w", err)
	}

	// The open append handle must be closed before the rename on Windows.
	if err := s.file.Close(); err != nil {
		return 0, fmt.Errorf("closing dead-letter file: %w", err)
	}
	renameErr := os.Rename(tmpPath, s.path)
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("reopening dead-letter file: %w", err)
	}
	s.file = f
	if renameErr != nil {
		return 0, fmt.Errorf("replacing dead-letter file: %w", renameErr)
	}
	return removed, nil
}

// Record appends e as a single JSON line.
func (s *FileSink) Record(e Entry) error {
	if e.FailedAt.IsZero() {
		e.FailedAt = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling dead-letter entry: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.file.Write(data); err != nil {
		return fmt.Errorf("writing dead-letter entry: %w", err)
	}
	return nil
}

// Close closes the underlying file.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}
