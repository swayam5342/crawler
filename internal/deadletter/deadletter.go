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
	return &FileSink{file: f}, nil
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
