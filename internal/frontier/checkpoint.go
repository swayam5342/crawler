package frontier

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CheckpointStore persists and reloads a Checkpoint. The concrete
// implementation here is file-based; a database-backed implementation can
// be swapped in once the storage backend (phase 4) exists.
type CheckpointStore interface {
	Save(cp Checkpoint) error
	Load() (cp Checkpoint, found bool, err error)
}

// FileCheckpointStore persists a Checkpoint as JSON to a single file. Save
// writes to a temporary file and renames it into place, so a crash mid-write
// cannot corrupt the last good checkpoint.
type FileCheckpointStore struct {
	Path string
}

// Save writes cp to the store's file, replacing any prior checkpoint.
func (s FileCheckpointStore) Save(cp Checkpoint) error {
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshaling checkpoint: %w", err)
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating checkpoint directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".checkpoint-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp checkpoint file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp checkpoint file: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("committing checkpoint: %w", err)
	}
	return nil
}

// Load reads a previously-saved checkpoint. found is false if no checkpoint
// file exists yet.
func (s FileCheckpointStore) Load() (Checkpoint, bool, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return Checkpoint{}, false, nil
		}
		return Checkpoint{}, false, fmt.Errorf("reading checkpoint: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, false, fmt.Errorf("parsing checkpoint: %w", err)
	}
	return cp, true, nil
}
