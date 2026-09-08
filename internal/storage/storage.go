// Package storage defines the structured storage backend interface for
// crawled page metadata. The interface is deliberately small so a
// different backend can be swapped in without touching the crawler.
package storage

import (
	"context"
	"time"
)

// Page is one crawled page's metadata, ready to persist.
type Page struct {
	URL         string
	Domain      string
	StatusCode  int
	Title       string
	Language    string
	NoIndex     bool
	ContentHash string // sha256 of the page body; doubles as the blob store key
	FetchedAt   time.Time
}

// Backend persists crawled page metadata. Implementations must be safe for
// concurrent use.
type Backend interface {
	// SavePage upserts p, keyed by p.URL.
	SavePage(ctx context.Context, p Page) error
	// Ping checks connectivity to the underlying store, for health checks.
	Ping(ctx context.Context) error
	// Close releases any underlying resources (connections, files, etc).
	Close() error
}
