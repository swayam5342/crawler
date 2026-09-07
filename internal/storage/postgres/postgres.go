// Package postgres implements storage.Backend on top of PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
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

var _ storage.Backend = (*Backend)(nil)

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

// Close implements storage.Backend.
func (b *Backend) Close() error {
	b.pool.Close()
	return nil
}
