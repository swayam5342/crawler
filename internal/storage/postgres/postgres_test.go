package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/swayamsb50/web_crawler/internal/storage"
)

// These are integration tests against a real Postgres instance. They run
// only when TEST_POSTGRES_DSN is set (e.g. in CI with a Postgres service
// container, or locally against a disposable `docker run postgres`), and
// are skipped otherwise so `go test ./...` doesn't require a live database.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping Postgres integration test")
	}
	return dsn
}

func openTestBackend(t *testing.T) *Backend {
	t.Helper()
	dsn := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, err := Open(ctx, Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func TestSaveAndGetPage(t *testing.T) {
	b := openTestBackend(t)
	ctx := context.Background()

	page := storage.Page{
		URL:         "https://example.com/a",
		Domain:      "example.com",
		StatusCode:  200,
		Title:       "Example A",
		Language:    "en",
		NoIndex:     false,
		ContentHash: "abc123",
		FetchedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := b.SavePage(ctx, page); err != nil {
		t.Fatalf("SavePage() error: %v", err)
	}

	got, found, err := b.GetPage(ctx, page.URL)
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if !found {
		t.Fatalf("expected page to be found")
	}
	if got.Title != page.Title || got.Domain != page.Domain || got.StatusCode != page.StatusCode {
		t.Errorf("GetPage() = %+v, want to match %+v", got, page)
	}
	if !got.FetchedAt.Equal(page.FetchedAt) {
		t.Errorf("FetchedAt = %v, want %v", got.FetchedAt, page.FetchedAt)
	}
}

func TestSavePageUpsertsOnConflict(t *testing.T) {
	b := openTestBackend(t)
	ctx := context.Background()

	url := "https://example.com/upsert-test"
	if err := b.SavePage(ctx, storage.Page{URL: url, Domain: "example.com", StatusCode: 200, Title: "First", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("SavePage() error: %v", err)
	}
	if err := b.SavePage(ctx, storage.Page{URL: url, Domain: "example.com", StatusCode: 200, Title: "Second", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("SavePage() error: %v", err)
	}

	got, found, err := b.GetPage(ctx, url)
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if !found || got.Title != "Second" {
		t.Errorf("expected upsert to overwrite Title with Second, got %+v (found=%v)", got, found)
	}
}

func TestPing(t *testing.T) {
	b := openTestBackend(t)
	if err := b.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestGetPageNotFound(t *testing.T) {
	b := openTestBackend(t)
	ctx := context.Background()

	_, found, err := b.GetPage(ctx, "https://example.com/does-not-exist-"+time.Now().String())
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if found {
		t.Errorf("expected found=false for a nonexistent page")
	}
}
