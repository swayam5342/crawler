package control

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/crawler"
	"github.com/swayamsb50/web_crawler/internal/deadletter"
	"github.com/swayamsb50/web_crawler/internal/frontier"
)

type fixture struct {
	ctl    *Controller
	f      *frontier.Frontier
	filter *frontier.DomainFilter
	gate   *crawler.PauseGate
	dl     *deadletter.FileSink
}

func newFixture(t *testing.T, domains ...string) fixture {
	t.Helper()
	filter := frontier.NewDomainFilter(domains)
	f := frontier.New(filter, frontier.WithKeepAlive())
	gate := crawler.NewPauseGate(false)
	dl, err := deadletter.NewFileSink(filepath.Join(t.TempDir(), "dl.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dl.Close() })
	ctl, err := New(Options{
		Frontier:   f,
		Filter:     filter,
		Gate:       gate,
		DeadLetter: dl,
		Checkpoint: frontier.FileCheckpointStore{Path: filepath.Join(t.TempDir(), "cp.json")},
		KeepAlive:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{ctl: ctl, f: f, filter: filter, gate: gate, dl: dl}
}

func TestAddURLsStatuses(t *testing.T) {
	tests := []struct {
		name        string
		domains     []string
		pre         []string
		url         string
		opts        AddOptions
		wantStatus  string
		wantAllowed []string
	}{
		{name: "new URL is queued", url: "https://example.com/a", wantStatus: "queued"},
		{name: "scheme defaults to https", url: "example.com/a", wantStatus: "queued"},
		{name: "duplicate", pre: []string{"https://example.com/a"}, url: "https://example.com/a#frag", wantStatus: "duplicate"},
		{name: "recrawl of a still-queued URL is not duplicated", pre: []string{"https://example.com/a"}, url: "https://example.com/a", opts: AddOptions{Recrawl: true}, wantStatus: "duplicate"},
		{name: "unsupported scheme", url: "ftp://example.com/", wantStatus: "invalid"},
		{name: "out of scope", domains: []string{"example.com"}, url: "https://other.org/", wantStatus: "out_of_scope"},
		{
			name: "allow_domain extends scoped allow-list", domains: []string{"example.com"},
			url: "https://other.org/", opts: AddOptions{AllowDomain: true},
			wantStatus: "queued", wantAllowed: []string{"example.com", "other.org"},
		},
		{
			name: "allow_domain leaves an empty (allow-all) list alone",
			url:  "https://other.org/", opts: AddOptions{AllowDomain: true},
			wantStatus: "queued", wantAllowed: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t, tt.domains...)
			for _, u := range tt.pre {
				fx.f.Add(u, 0)
			}
			res, err := fx.ctl.AddURLs([]string{tt.url}, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(res) != 1 || res[0].Status != tt.wantStatus {
				t.Fatalf("AddURLs() = %+v, want status %q", res, tt.wantStatus)
			}
			if tt.wantAllowed != nil {
				got := fx.filter.Domains()
				if len(got) != len(tt.wantAllowed) {
					t.Fatalf("allowed domains = %v, want %v", got, tt.wantAllowed)
				}
			}
		})
	}
}

func TestAddURLsRejectsTooMany(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.ctl.AddURLs(make([]string, MaxURLsPerAdd+1), AddOptions{}); err == nil {
		t.Fatal("expected error for oversized batch")
	}
}

func TestStateTransitions(t *testing.T) {
	fx := newFixture(t)
	if s := fx.ctl.State(); s != StateIdle {
		t.Fatalf("empty crawl state = %s, want idle", s)
	}
	fx.f.Add("https://example.com/", 0)
	if s := fx.ctl.State(); s != StateRunning {
		t.Fatalf("state with queue = %s, want running", s)
	}
	fx.ctl.Pause()
	if s := fx.ctl.State(); s != StatePaused {
		t.Fatalf("state = %s, want paused", s)
	}
	fx.ctl.Resume()
	fx.ctl.MarkStopping()
	if s := fx.ctl.State(); s != StateStopping {
		t.Fatalf("state = %s, want stopping", s)
	}
}

func TestRetryAndDiscardDeadLetters(t *testing.T) {
	fx := newFixture(t, "example.com")
	// Simulate URLs that were visited and then failed.
	for _, u := range []string{"https://example.com/a", "https://example.com/b", "https://outside.org/c"} {
		fx.dl.Record(deadletter.Entry{URL: u, Depth: 2})
		fx.ctl.OnFetch(crawler.FetchEvent{URL: u, Outcome: crawler.OutcomeFailure, Err: errors.New("boom")})
	}
	if got := fx.ctl.Status().DeadLetters; got != 3 {
		t.Fatalf("dead letter count = %d, want 3", got)
	}

	n, err := fx.ctl.RetryDeadLetters([]string{"https://example.com/a"})
	if err != nil || n != 1 {
		t.Fatalf("RetryDeadLetters(a) = %d, %v; want 1", n, err)
	}
	// Retry all: b is requeued, the out-of-scope URL stays in the log.
	if n, _ := fx.ctl.RetryDeadLetters(nil); n != 1 {
		t.Fatalf("RetryDeadLetters(all) = %d, want 1", n)
	}
	entries, _ := fx.ctl.DeadLetters()
	if len(entries) != 1 || entries[0].URL != "https://outside.org/c" {
		t.Fatalf("remaining dead letters = %+v", entries)
	}
	if q := fx.f.Stats().Queued; q != 2 {
		t.Fatalf("queued = %d, want 2", q)
	}

	if n, _ := fx.ctl.DiscardDeadLetters(nil); n != 1 {
		t.Fatalf("DiscardDeadLetters(all) = %d, want 1", n)
	}
	if got := fx.ctl.Status().DeadLetters; got != 0 {
		t.Fatalf("dead letter count after discard = %d", got)
	}
}

func TestResetSelectively(t *testing.T) {
	fx := newFixture(t)
	fx.f.Add("https://example.com/", 0)
	fx.ctl.OnFetch(crawler.FetchEvent{URL: "https://example.com/x", Domain: "example.com", Outcome: crawler.OutcomeSuccess, At: time.Now()})
	fx.dl.Record(deadletter.Entry{URL: "https://example.com/dead"})

	res, err := fx.ctl.Reset(context.Background(), ResetOptions{Queue: true, Stats: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.QueueCleared || !res.StatsCleared {
		t.Fatalf("Reset() = %+v", res)
	}
	st := fx.ctl.Status()
	if st.Frontier.Queued != 0 || st.Frontier.Visited != 0 || st.Totals.Success != 0 {
		t.Fatalf("status after reset = %+v", st)
	}
	if entries, _ := fx.ctl.DeadLetters(); len(entries) != 1 {
		t.Fatal("dead letters should survive a queue+stats reset")
	}

	// Pages reset without a purger reports ErrUnsupported.
	if _, err := fx.ctl.Reset(context.Background(), ResetOptions{Pages: true}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("pages reset error = %v, want ErrUnsupported", err)
	}
}

func TestStatsThroughputAndRecent(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := newStats(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		s.record(crawler.FetchEvent{URL: "u", Domain: "a.com", Outcome: crawler.OutcomeSuccess, At: now.Add(-2 * time.Second), Duration: 100 * time.Millisecond})
	}
	s.record(crawler.FetchEvent{URL: "last", Domain: "b.com", Outcome: crawler.OutcomeFailure, At: now.Add(-1 * time.Second), Err: errors.New("x")})

	snap := s.snapshot(10)
	if snap.Totals.Success != 5 || snap.Totals.Failure != 1 {
		t.Fatalf("totals = %+v", snap.Totals)
	}
	if snap.PagesPerS != 0.5 {
		t.Fatalf("pages/s = %v, want 0.5 (5 in a 10s window)", snap.PagesPerS)
	}
	if len(snap.Throughput) != throughputWindow {
		t.Fatalf("throughput points = %d", len(snap.Throughput))
	}
	if snap.Domains[0].Domain != "a.com" || snap.Domains[0].AvgDurationMS != 100 {
		t.Fatalf("top domain = %+v", snap.Domains[0])
	}
	if r := s.recentEvents(1); len(r) != 1 || r[0].URL != "last" {
		t.Fatalf("recent = %+v, want newest first", r)
	}

	// Ring buffer wraps without losing ordering.
	for i := 0; i < recentCap+5; i++ {
		s.record(crawler.FetchEvent{URL: "w", Outcome: crawler.OutcomeBlocked, At: now})
	}
	s.record(crawler.FetchEvent{URL: "newest", Outcome: crawler.OutcomeBlocked, At: now})
	if r := s.recentEvents(2); r[0].URL != "newest" || len(s.recentEvents(0)) != recentCap {
		t.Fatalf("after wrap recent[0] = %q", r[0].URL)
	}
}
