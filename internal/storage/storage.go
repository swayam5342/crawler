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

// PageQuery selects a page of stored pages for browsing.
type PageQuery struct {
	Search string // case-insensitive substring match on URL or title
	Domain string // exact domain match; empty for all
	Limit  int
	Offset int
}

// PageList is one page of results plus the total number of matches.
type PageList struct {
	Pages []Page `json:"pages"`
	Total int    `json:"total"`
}

// DomainCount is the number of stored pages for one domain.
type DomainCount struct {
	Domain string `json:"domain"`
	Pages  int    `json:"pages"`
}

// Browser is an optional extension of Backend for reading stored pages
// back, used by the control dashboard. Backends that cannot support it
// simply don't implement it.
type Browser interface {
	ListPages(ctx context.Context, q PageQuery) (PageList, error)
	DomainCounts(ctx context.Context) ([]DomainCount, error)
}

// Purger is an optional extension of Backend that can delete every stored
// page (used by the dashboard's reset action).
type Purger interface {
	DeleteAllPages(ctx context.Context) (int64, error)
}
