package deadletter

import (
	"path/filepath"
	"testing"
)

func TestFileSinkRemoveRewritesAndKeepsAppending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dl.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()

	for _, u := range []string{"https://a/", "https://b/", "https://c/"} {
		if err := sink.Record(Entry{URL: u}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := sink.Remove(func(e Entry) bool { return e.URL == "https://b/" })
	if err != nil || n != 1 {
		t.Fatalf("Remove() = %d, %v; want 1, nil", n, err)
	}
	if err := sink.Record(Entry{URL: "https://d/"}); err != nil {
		t.Fatalf("Record after Remove: %v", err)
	}

	got, err := sink.Entries()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a/", "https://c/", "https://d/"}
	if len(got) != len(want) {
		t.Fatalf("entries = %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].URL != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i].URL, want[i])
		}
	}

	if n, err := sink.Remove(func(Entry) bool { return false }); err != nil || n != 0 {
		t.Fatalf("no-op Remove() = %d, %v", n, err)
	}
}
