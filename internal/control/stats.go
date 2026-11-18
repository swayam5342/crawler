package control

import (
	"sort"
	"sync"
	"time"

	"github.com/swayamsb50/web_crawler/internal/crawler"
)

const (
	recentCap         = 200 // recent events kept for the activity feed
	throughputWindow  = 60  // seconds of per-second throughput history
	rateWindowSeconds = 10  // window for the headline pages/sec figure
	maxDomainsTracked = 10000
)

// Event is one processed URL as shown in the activity feed.
type Event struct {
	URL        string    `json:"url"`
	Domain     string    `json:"domain"`
	Depth      int       `json:"depth"`
	Outcome    string    `json:"outcome"`
	Links      int       `json:"links"`
	Error      string    `json:"error,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	At         time.Time `json:"at"`
}

// Totals are crawl-wide counters since start (or the last stats reset).
type Totals struct {
	Success int64 `json:"success"`
	Failure int64 `json:"failure"`
	Blocked int64 `json:"blocked"`
	Links   int64 `json:"links_found"`
}

// DomainStats summarizes fetch outcomes for one domain.
type DomainStats struct {
	Domain        string    `json:"domain"`
	Success       int64     `json:"success"`
	Failure       int64     `json:"failure"`
	Blocked       int64     `json:"blocked"`
	AvgDurationMS int64     `json:"avg_duration_ms"`
	LastSeen      time.Time `json:"last_seen"`
	LastError     string    `json:"last_error,omitempty"`
	Breaker       string    `json:"breaker,omitempty"`

	totalDuration time.Duration
}

// Point is one second of throughput history.
type Point struct {
	At      int64 `json:"t"` // unix seconds
	Success int   `json:"ok"`
	Failure int   `json:"fail"`
}

type bucket struct {
	sec           int64
	success, fail int
}

// stats aggregates FetchEvents in memory. It is safe for concurrent use.
type stats struct {
	mu        sync.Mutex
	since     time.Time
	totals    Totals
	domains   map[string]*DomainStats
	recent    []Event // ring buffer
	recentPos int
	buckets   [throughputWindow]bucket
	now       func() time.Time
}

func newStats(now func() time.Time) *stats {
	if now == nil {
		now = time.Now
	}
	s := &stats{now: now}
	s.reset()
	return s
}

func (s *stats) reset() {
	s.since = s.now()
	s.totals = Totals{}
	s.domains = make(map[string]*DomainStats)
	s.recent = make([]Event, 0, recentCap)
	s.recentPos = 0
	s.buckets = [throughputWindow]bucket{}
}

func (s *stats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset()
}

func (s *stats) record(ev crawler.FetchEvent) {
	e := Event{
		URL:        ev.URL,
		Domain:     ev.Domain,
		Depth:      ev.Depth,
		Outcome:    string(ev.Outcome),
		Links:      ev.Links,
		DurationMS: ev.Duration.Milliseconds(),
		At:         ev.At,
	}
	if ev.Err != nil {
		e.Error = ev.Err.Error()
	}
	if e.At.IsZero() {
		e.At = s.now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.domains[ev.Domain]
	if !ok {
		if len(s.domains) >= maxDomainsTracked {
			d = &DomainStats{} // count totals but stop growing the map
		} else {
			d = &DomainStats{Domain: ev.Domain}
			s.domains[ev.Domain] = d
		}
	}
	d.LastSeen = e.At

	switch ev.Outcome {
	case crawler.OutcomeSuccess:
		s.totals.Success++
		s.totals.Links += int64(ev.Links)
		d.Success++
		d.totalDuration += ev.Duration
		s.bump(e.At, true)
	case crawler.OutcomeFailure:
		s.totals.Failure++
		d.Failure++
		d.totalDuration += ev.Duration
		d.LastError = e.Error
		s.bump(e.At, false)
	case crawler.OutcomeBlocked:
		s.totals.Blocked++
		d.Blocked++
	}

	if len(s.recent) < recentCap {
		s.recent = append(s.recent, e)
	} else {
		s.recent[s.recentPos] = e
	}
	s.recentPos = (s.recentPos + 1) % recentCap
}

// bump must be called with s.mu held.
func (s *stats) bump(at time.Time, success bool) {
	sec := at.Unix()
	b := &s.buckets[sec%throughputWindow]
	if b.sec != sec {
		*b = bucket{sec: sec}
	}
	if success {
		b.success++
	} else {
		b.fail++
	}
}

type statsSnapshot struct {
	Since      time.Time
	Totals     Totals
	PagesPerS  float64
	Throughput []Point
	Domains    []DomainStats
}

func (s *stats) snapshot(maxDomains int) statsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowSec := s.now().Unix()
	points := make([]Point, 0, throughputWindow)
	var recentOK int
	// Oldest to newest, excluding the current (partial) second.
	for sec := nowSec - throughputWindow; sec < nowSec; sec++ {
		p := Point{At: sec}
		if b := s.buckets[((sec%throughputWindow)+throughputWindow)%throughputWindow]; b.sec == sec {
			p.Success, p.Failure = b.success, b.fail
		}
		if sec >= nowSec-rateWindowSeconds {
			recentOK += p.Success
		}
		points = append(points, p)
	}

	domains := make([]DomainStats, 0, len(s.domains))
	for _, d := range s.domains {
		ds := *d
		if n := ds.Success + ds.Failure; n > 0 {
			ds.AvgDurationMS = (ds.totalDuration / time.Duration(n)).Milliseconds()
		}
		domains = append(domains, ds)
	}
	sort.Slice(domains, func(i, j int) bool {
		ti := domains[i].Success + domains[i].Failure + domains[i].Blocked
		tj := domains[j].Success + domains[j].Failure + domains[j].Blocked
		if ti != tj {
			return ti > tj
		}
		return domains[i].Domain < domains[j].Domain
	})
	if maxDomains > 0 && len(domains) > maxDomains {
		domains = domains[:maxDomains]
	}

	return statsSnapshot{
		Since:      s.since,
		Totals:     s.totals,
		PagesPerS:  float64(recentOK) / rateWindowSeconds,
		Throughput: points,
		Domains:    domains,
	}
}

// recentEvents returns up to n events, newest first.
func (s *stats) recentEvents(n int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	size := len(s.recent)
	if n <= 0 || n > size {
		n = size
	}
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		// Newest is just before recentPos.
		idx := (s.recentPos - 1 - i + size) % size
		if size < recentCap {
			idx = size - 1 - i
		}
		out = append(out, s.recent[idx])
	}
	return out
}
