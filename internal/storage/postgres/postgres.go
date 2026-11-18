// Package postgres implements storage.Backend on top of PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/swayamsb50/web_crawler/internal/storage"
)

const schema = `
CREATE TABLE IF NOT EXISTS pages (
	url          TEXT PRIMARY KEY,
	domain       TEXT NOT NULL,
	status_code  INTEGER NOT NULL,
	title        TEXT NOT NULL DEFAULT '',
	language     TEXT NOT NULL DEFAULT '',
	no_index     BOOLEAN NOT NULL DEFAULT false,
	content_hash TEXT NOT NULL DEFAULT '',
	fetched_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pages_domain ON pages (domain);
`

// Config controls the Postgres connection pool.
type Config struct {
	DSN             string
	MaxOpenConns    int32
	MaxIdleConns    int32
	ConnMaxLifetime time.Duration
}

// Backend is a storage.Backend implementation backed by a Postgres
// connection pool. It creates its schema on Open if not already present.
type Backend struct {
	pool *pgxpool.Pool
}

var (
	_ storage.Backend = (*Backend)(nil)
	_ storage.Browser = (*Backend)(nil)
	_ storage.Purger  = (*Backend)(nil)
)

// Open connects to Postgres per cfg and ensures the schema exists.
func Open(ctx context.Context, cfg Config) (*Backend, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres dsn: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = cfg.MaxOpenConns
	}
	if cfg.MaxIdleConns > 0 {
		poolCfg.MinConns = cfg.MaxIdleConns
	}
	if cfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &Backend{pool: pool}, nil
}

// SavePage implements storage.Backend.
func (b *Backend) SavePage(ctx context.Context, p storage.Page) error {
	const query = `
INSERT INTO pages (url, domain, status_code, title, language, no_index, content_hash, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (url) DO UPDATE SET
	domain       = EXCLUDED.domain,
	status_code  = EXCLUDED.status_code,
	title        = EXCLUDED.title,
	language     = EXCLUDED.language,
	no_index     = EXCLUDED.no_index,
	content_hash = EXCLUDED.content_hash,
	fetched_at   = EXCLUDED.fetched_at
`
	_, err := b.pool.Exec(ctx, query,
		p.URL, p.Domain, p.StatusCode, p.Title, p.Language, p.NoIndex, p.ContentHash, p.FetchedAt)
	if err != nil {
		return fmt.Errorf("saving page %s: %w", p.URL, err)
	}
	return nil
}

// GetPage fetches a previously-saved page by URL, for tests and tooling.
func (b *Backend) GetPage(ctx context.Context, url string) (storage.Page, bool, error) {
	const query = `
SELECT url, domain, status_code, title, language, no_index, content_hash, fetched_at
FROM pages WHERE url = $1
`
	row := b.pool.QueryRow(ctx, query, url)
	var p storage.Page
	err := row.Scan(&p.URL, &p.Domain, &p.StatusCode, &p.Title, &p.Language, &p.NoIndex, &p.ContentHash, &p.FetchedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return storage.Page{}, false, nil
		}
		return storage.Page{}, false, fmt.Errorf("getting page %s: %w", url, err)
	}
	return p, true, nil
}

// ListPages implements storage.Browser. Results are ordered newest first.
func (b *Backend) ListPages(ctx context.Context, q storage.PageQuery) (storage.PageList, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 50
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	// $1/$2 are always bound; an empty string disables that filter.
	const where = `
WHERE ($1 = '' OR url ILIKE '%' || $1 || '%' OR title ILIKE '%' || $1 || '%')
  AND ($2 = '' OR domain = $2)`
	search := escapeLike(q.Search)

	var total int
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM pages`+where, search, q.Domain).Scan(&total); err != nil {
		return storage.PageList{}, fmt.Errorf("counting pages: %w", err)
	}

	rows, err := b.pool.Query(ctx, `
SELECT url, domain, status_code, title, language, no_index, content_hash, fetched_at
FROM pages`+where+`
ORDER BY fetched_at DESC, url
LIMIT $3 OFFSET $4`, search, q.Domain, q.Limit, q.Offset)
	if err != nil {
		return storage.PageList{}, fmt.Errorf("listing pages: %w", err)
	}
	defer rows.Close()

	pages := make([]storage.Page, 0, q.Limit)
	for rows.Next() {
		var p storage.Page
		if err := rows.Scan(&p.URL, &p.Domain, &p.StatusCode, &p.Title, &p.Language, &p.NoIndex, &p.ContentHash, &p.FetchedAt); err != nil {
			return storage.PageList{}, fmt.Errorf("scanning page: %w", err)
		}
		pages = append(pages, p)
	}
	if err := rows.Err(); err != nil {
		return storage.PageList{}, fmt.Errorf("listing pages: %w", err)
	}
	return storage.PageList{Pages: pages, Total: total}, nil
}

// DomainCounts implements storage.Browser.
func (b *Backend) DomainCounts(ctx context.Context) ([]storage.DomainCount, error) {
	rows, err := b.pool.Query(ctx, `SELECT domain, count(*) FROM pages GROUP BY domain ORDER BY count(*) DESC, domain LIMIT 500`)
	if err != nil {
		return nil, fmt.Errorf("counting pages by domain: %w", err)
	}
	defer rows.Close()
	var out []storage.DomainCount
	for rows.Next() {
		var dc storage.DomainCount
		if err := rows.Scan(&dc.Domain, &dc.Pages); err != nil {
			return nil, fmt.Errorf("scanning domain count: %w", err)
		}
		out = append(out, dc)
	}
	return out, rows.Err()
}

// DeleteAllPages implements storage.Purger.
func (b *Backend) DeleteAllPages(ctx context.Context) (int64, error) {
	tag, err := b.pool.Exec(ctx, `DELETE FROM pages`)
	if err != nil {
		return 0, fmt.Errorf("deleting pages: %w", err)
	}
	return tag.RowsAffected(), nil
}

// escapeLike escapes LIKE/ILIKE wildcards so user search text matches
// literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Ping implements storage.Backend.
func (b *Backend) Ping(ctx context.Context) error {
	if err := b.pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging postgres: %w", err)
	}
	return nil
}

// Close implements storage.Backend.
func (b *Backend) Close() error {
	b.pool.Close()
	return nil
}
