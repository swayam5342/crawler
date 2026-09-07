package deadletter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// ReadAll reads every entry from a JSONL dead-letter file at path. It
// returns an empty slice (no error) if the file does not exist yet.
func ReadAll(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening dead-letter file: %w", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parsing dead-letter entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading dead-letter file: %w", err)
	}
	return entries, nil
}
