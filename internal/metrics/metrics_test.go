package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("scrape returned status %d, want 200", rec.Code)
	}
	return string(body)
}

func TestRecordFetchAppearsInScrape(t *testing.T) {
	m := New()
	m.RecordFetch("example.com", true)
	m.RecordFetch("example.com", true)
	m.RecordFetch("example.com", false)

	body := scrape(t, m)

	if !strings.Contains(body, `crawler_pages_total{domain="example.com",result="success"} 2`) {
		t.Errorf("expected success counter = 2 in scrape output, got:\n%s", body)
	}
	if !strings.Contains(body, `crawler_pages_total{domain="example.com",result="failure"} 1`) {
		t.Errorf("expected failure counter = 1 in scrape output, got:\n%s", body)
	}
}

func TestPerDomainCountersAreIndependent(t *testing.T) {
	m := New()
	m.RecordFetch("a.com", true)
	m.RecordFetch("b.com", true)
	m.RecordFetch("b.com", true)

	body := scrape(t, m)
	if !strings.Contains(body, `crawler_pages_total{domain="a.com",result="success"} 1`) {
		t.Errorf("expected a.com success = 1, got:\n%s", body)
	}
	if !strings.Contains(body, `crawler_pages_total{domain="b.com",result="success"} 2`) {
		t.Errorf("expected b.com success = 2, got:\n%s", body)
	}
}

func TestQueueDepthGaugeReflectsCurrentValue(t *testing.T) {
	m := New()
	depth := 0.0
	m.SetQueueDepthFunc(func() float64 { return depth })

	body := scrape(t, m)
	if !strings.Contains(body, "crawler_queue_depth 0") {
		t.Errorf("expected queue depth 0 in scrape output, got:\n%s", body)
	}

	depth = 42
	body = scrape(t, m)
	if !strings.Contains(body, "crawler_queue_depth 42") {
		t.Errorf("expected queue depth 42 after update, got:\n%s", body)
	}
}
