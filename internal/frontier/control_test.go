package frontier

import (
	"context"
	"testing"
	"time"
)

func TestKeepAliveDoesNotAutoClose(t *testing.T) {
	f := New(nil, WithKeepAlive())
	if _, err := f.Add("https://example.com/a", 0); err != nil {
		t.Fatal(err)
	}
	item, _ := f.Next(context.Background())
	f.Done(item.URL)

	if f.Stats().Closed {
		t.Fatal("keep-alive frontier closed after draining")
	}
	// New work must still be accepted and handed out.
	if ok, _ := f.Add("https://example.com/b", 0); !ok {
		t.Fatal("Add after drain was rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if got, ok := f.Next(ctx); !ok || got.URL != "https://example.com/b" {
		t.Fatalf("Next() = %+v, %v; want b, true", got, ok)
	}
}

func TestReleaseReturnsItemToFrontOfQueue(t *testing.T) {
	f := New(nil)
	f.Add("https://example.com/a", 0)
	f.Add("https://example.com/b", 0)

	a, _ := f.Next(context.Background())
	f.Release(a)

	st := f.Stats()
	if st.Queued != 2 || st.InFlight != 0 {
		t.Fatalf("after Release: %+v, want 2 queued 0 in flight", st)
	}
	// Done on a released item is a no-op and must not close the frontier.
	f.Done(a.URL)
	if f.Stats().Closed {
		t.Fatal("Done on released item closed the frontier")
	}
	if got, _ := f.Next(context.Background()); got.URL != a.URL {
		t.Fatalf("Next() after Release = %q, want %q first", got.URL, a.URL)
	}
}

func TestRequeue(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(f *Frontier)
		url    string
		filter Filter
		want   bool
	}{
		{
			name: "already visited and done is requeued",
			setup: func(f *Frontier) {
				f.Add("https://example.com/a", 0)
				it, _ := f.Next(context.Background())
				f.Done(it.URL)
			},
			url:  "https://example.com/a",
			want: true,
		},
		{
			name:  "already queued is not duplicated",
			setup: func(f *Frontier) { f.Add("https://example.com/a", 0) },
			url:   "https://example.com/a",
			want:  false,
		},
		{
			name: "in flight is not duplicated",
			setup: func(f *Frontier) {
				f.Add("https://example.com/a", 0)
				f.Next(context.Background())
			},
			url:  "https://example.com/a",
			want: false,
		},
		{
			name:   "out of scope is rejected",
			setup:  func(f *Frontier) {},
			url:    "https://other.com/",
			filter: NewDomainFilter([]string{"example.com"}),
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(tt.filter, WithKeepAlive())
			tt.setup(f)
			got, err := f.Requeue(tt.url, 0)
			if err != nil {
				t.Fatalf("Requeue() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Requeue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveQueuedAndReset(t *testing.T) {
	f := New(nil, WithKeepAlive())
	f.Add("https://example.com/a", 0)
	f.Add("https://example.com/b", 0)

	if !f.RemoveQueued("https://example.com/a") {
		t.Fatal("RemoveQueued(a) = false, want true")
	}
	if f.RemoveQueued("https://example.com/a") {
		t.Fatal("second RemoveQueued(a) = true, want false")
	}
	if st := f.Stats(); st.Queued != 1 || st.Visited != 2 {
		t.Fatalf("after remove: %+v, want 1 queued, 2 visited", st)
	}

	inflight, _ := f.Next(context.Background())
	f.Reset()
	if st := f.Stats(); st.Queued != 0 || st.Visited != 0 || st.InFlight != 0 {
		t.Fatalf("after Reset: %+v, want all zero", st)
	}
	f.Done(inflight.URL) // must be a harmless no-op
	if ok, _ := f.Add("https://example.com/a", 0); !ok {
		t.Fatal("Add after Reset should accept a previously visited URL")
	}
}

func TestDomainFilterMutations(t *testing.T) {
	f := NewDomainFilter([]string{"Example.com."})
	if !f.Allowed("https://www.example.com/") {
		t.Fatal("normalized domain should allow subdomain")
	}
	if f.Allowed("https://other.org/") {
		t.Fatal("other.org should be out of scope")
	}
	if !f.Add("other.org") || f.Add("OTHER.org") {
		t.Fatal("Add should add once, case-insensitively")
	}
	if !f.Allowed("https://other.org/x") {
		t.Fatal("added domain should be allowed")
	}
	if got := f.Domains(); len(got) != 2 || got[0] != "example.com" || got[1] != "other.org" {
		t.Fatalf("Domains() = %v", got)
	}
	f.Remove("example.com")
	f.Remove("other.org")
	if !f.Allowed("https://anything.net/") {
		t.Fatal("empty allow-list should allow everything")
	}
}
