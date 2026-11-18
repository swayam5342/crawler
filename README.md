# web_crawler

A production-grade web crawler in Go, built for unattended operation against
real-world sites: bounded concurrency, respect for robots.txt, defense in
depth against network failures, structured storage, and operational
visibility.

## Contents

- [Quick start](#quick-start)
- [Dashboard and website](#dashboard-and-website)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Data pipeline](#data-pipeline)
- [Observability](#observability)
- [Operations](#operations)
- [Development](#development)

## Quick start

The crawler requires PostgreSQL for structured page metadata. The fastest
path is Docker Compose, which runs Postgres and the crawler together:

```sh
docker compose up --build
```

This starts Postgres (with a health check gating crawler startup), the
crawler using `configs/config.yaml` (API/metrics/health server on `:9090`),
and the web dashboard on `:3000`. Open <http://localhost:3000/dashboard> and
add a URL to start crawling. Crawled page data and blobs persist in the `postgres_data` and
`crawler_data` named volumes.

To point it at real seed URLs, override config via environment variables
(see [Configuration](#configuration)) — for example, in a
`docker-compose.override.yml` or `-e` flags on `docker run`:

```sh
docker compose run -e CRAWLER_SEED_URLS="https://example.com/" \
                    -e CRAWLER_ALLOWED_DOMAINS="example.com" \
                    crawler
```

### Running locally without Docker

```sh
# Start Postgres (or point CRAWLER_POSTGRES_DSN at an existing instance)
docker run -d -p 5432:5432 \
  -e POSTGRES_USER=crawler -e POSTGRES_PASSWORD=crawler -e POSTGRES_DB=crawler \
  postgres:16-alpine

go run ./cmd/crawler --config configs/config.yaml
```

The binary needs write access to `./data` (checkpoint, dead-letter log, and
local blob storage default there).

## Dashboard and website

`web/` is a Next.js app with two parts:

- `/` is a public page explaining what the crawler does.
- `/dashboard` controls a running crawler: add URLs, pause and resume,
  browse crawled pages, inspect the queue, retry or discard failed URLs,
  edit the domain allow-list, reset state, and watch live throughput,
  per-domain health and recent activity.

The dashboard needs the crawler running in service mode
(`crawl.keep_alive: true`, the default in `configs/config.yaml`), so it
stays up when the queue drains and waits for new URLs. In batch mode
(`keep_alive: false`) it exits once every reachable URL is processed, and
`timeouts.overall` bounds the run.

The browser never talks to the crawler directly. The dashboard's server
proxies `/api/crawler/*` to the crawler's `/api/*` and adds the API token,
so the token stays server-side and no CORS setup is needed.

```sh
cd web
cp .env.example .env.local   # set CRAWLER_API_URL / CRAWLER_API_TOKEN
npm install
npm run dev                  # http://localhost:3000
```

| Variable | Purpose |
| --- | --- |
| `CRAWLER_API_URL` | Crawler server address (default `http://localhost:9090`) |
| `CRAWLER_API_TOKEN` | Must match the crawler's `server.api_token` |
| `DASHBOARD_PASSWORD` | If set, `/dashboard` requires basic auth as user `admin` |

### Control API

Served by the crawler under `/api/` on `server.addr`. When
`server.api_token` (`CRAWLER_API_TOKEN`) is set, every request needs
`Authorization: Bearer <token>`. Set it whenever the port is reachable by
anyone else: the API can reset state and delete stored pages. POST bodies
must be `application/json`, which keeps plain cross-site form posts out.

| Method and path | What it does |
| --- | --- |
| `GET /api/status` | Run state, queue sizes, totals, pages/sec, error rate, 60s throughput, per-domain stats with circuit breaker state |
| `GET /api/events?limit=` | Recently processed URLs, newest first |
| `POST /api/urls` | `{"urls":[...],"allow_domain":bool,"recrawl":bool}`, returns a status per URL |
| `POST /api/pause`, `POST /api/resume` | Stop or restart new fetches; in-flight requests finish |
| `GET /api/queue?limit=`, `DELETE /api/queue?url=` | Inspect the queue, or drop one queued URL |
| `GET /api/pages?q=&domain=&limit=&offset=` | Browse stored pages |
| `GET /api/pages/domains` | Stored page counts per domain |
| `GET /api/deadletters` | Failed URLs, newest first |
| `POST /api/deadletters/retry`, `.../discard` | `{"urls":[...]}`; an empty list means all |
| `POST /api/domains`, `DELETE /api/domains?domain=` | Edit the allow-list at runtime |
| `POST /api/reset` | `{"queue","dead_letters","pages","stats"}` booleans |
| `GET /api/config` | Running config with secrets redacted |

Runtime changes (allow-list edits, resets) aren't written back to
`config.yaml`.

## Architecture

Package boundaries mirror the crawl pipeline; each is independently unit
tested and most are wired together only through small interfaces, so any
piece can be swapped without touching the others.

```text
cmd/crawler          wiring: reads config, constructs every component, runs
                     the crawl, serves /metrics and /healthz, handles
                     SIGINT/SIGTERM

internal/config      YAML + env-var configuration, validation, defaults
internal/urlutil     URL normalization (used for deduplication and scoping)
internal/frontier    thread-safe dedup queue; checkpoint snapshot/restore
internal/crawler     worker pool: pulls from the frontier, drives a fetch,
                     feeds discovered links back in, subject to per-domain
                     and global concurrency limits
internal/robots      robots.txt fetch/parse/cache with TTL; Crawl-delay
internal/ratelimit   per-domain token bucket rate limiter
internal/fetcher     resilient HTTP client: layered timeouts, retry with
                     backoff+jitter, redirect-loop detection, body size
                     caps, gzip/br decompression, charset normalization,
                     HTTP/2-to-1.1 fallback, circuit breaker integration
internal/breaker     per-domain circuit breaker (closed/open/half-open)
internal/deadletter  append-only JSONL log of permanently-failed URLs
internal/dedup       bloom filter, for large-scale duplicate detection
                     scenarios (see note below)
internal/extractor   pluggable page extraction: links, title, language,
                     robots meta (noindex/nofollow)
internal/storage     Backend interface + Postgres implementation
                     (page metadata)
internal/storage/blobstore
                     Store interface + local-disk and S3 implementations
                     (raw page bodies, keyed by content hash)
internal/metrics     Prometheus collectors (pages fetched, queue depth)
internal/health      /healthz handler running readiness checks
```

### Request flow

For each URL the pool dequeues from the frontier:

1. Check robots.txt (`internal/robots`) — disallowed URLs are skipped.
2. Apply the domain's rate limit (`internal/ratelimit`), overridden by the
   site's `Crawl-delay` if robots.txt declares one.
3. Fetch (`internal/fetcher`), gated by that domain's circuit breaker
   (`internal/breaker`). Fetch failures that exhaust retries are logged to
   the dead-letter file.
4. Extract links and metadata (`internal/extractor`).
5. Persist page metadata to Postgres and the raw body to the blob store,
   skipped if the page declares `noindex`.
6. Enqueue newly discovered links (skipping `rel="nofollow"` targets, if
   `honor_nofollow` is set) back into the frontier, deduplicated and
   filtered by allowed domains.

### Design notes

- **Frontier dedup is an exact in-memory set, not the bloom filter.**
  Checkpointing needs to enumerate exactly which URLs have been visited to
  resume correctly; a bloom filter can't be enumerated. `internal/dedup`
  ships a tested, standalone bloom filter for scenarios where an
  in-process exact set doesn't scale (e.g. a sharded/distributed dedup
  service in front of a persistent store) — it isn't wired into the
  frontier by default.
- **Link extraction currently only covers HTML.** `internal/extractor`
  defines a pluggable `Extractor` interface; swap in a different
  implementation (e.g. for JSON APIs or sitemaps) without touching the
  crawler or fetcher.
- **Interfaces are decoupled by structural typing, not shared types.**
  `internal/crawler` never imports `internal/fetcher`, `internal/robots`,
  etc. — it defines the minimal interface it needs (e.g. `RobotsChecker`,
  `RateLimiter`, `DeadLetter`) and the concrete packages satisfy it
  implicitly. This keeps the dependency graph a DAG rooted at `cmd/crawler`.

## Configuration

Config loads from a YAML file (default `configs/config.yaml`, override with
`--config`), then environment variables override individual fields. Every
tunable has a safe default — the crawler runs with no config file at all
(see `internal/config/defaults.go`).

### Key sections (`configs/config.yaml`)

| Section | Purpose |
| --- | --- |
| `crawl` | seed URLs, allowed domains, max depth, User-Agent, body size cap, redirect limit, nofollow/noindex handling, `keep_alive` service mode |
| `concurrency` | global and per-domain worker limits |
| `rate_limit` | default requests/sec and burst per domain |
| `timeouts` | DNS, TCP connect, TLS handshake, per-request, and overall crawl deadline |
| `retry` | max attempts, backoff base/max, jitter factor |
| `circuit_breaker` | failure threshold, cooldown period, half-open trial count |
| `robots` | robots.txt cache TTL and fetch timeout |
| `storage.postgres` | connection string and pool sizing |
| `storage.blob_store` | `local` or `s3` backend and its settings |
| `server` | API/metrics/health bind address, shutdown grace period, API token, CORS origins |
| `logging` | level and format (`text`/`json`) |

### Environment variable overrides

Every field above has a `CRAWLER_*` environment variable equivalent (see
`internal/config/load.go` for the full list), e.g.:

```sh
CRAWLER_SEED_URLS="https://example.com/,https://example.org/"
CRAWLER_ALLOWED_DOMAINS="example.com,example.org"
CRAWLER_CONCURRENCY_GLOBAL=100
CRAWLER_RATE_LIMIT_RPS=2
CRAWLER_POSTGRES_DSN="postgres://user:pass@host:5432/db?sslmode=disable"
CRAWLER_BLOB_STORE_BACKEND=s3
CRAWLER_BLOB_STORE_S3_BUCKET=my-crawl-bucket
CRAWLER_LOG_LEVEL=debug
```

Comma-separated lists (seed URLs, allowed domains) are split and trimmed.
Durations use Go syntax (`30s`, `5m`, `1h`).

## Data pipeline

- **Page metadata** is upserted into Postgres's `pages` table, keyed by
  URL (schema is created automatically on startup).
- **Raw page bodies** are stored in the blob store, keyed by the SHA-256
  hash of the (UTF-8-normalized) body — so identical content fetched from
  different URLs is stored once.
- Switching from local disk to S3 is a config change
  (`storage.blob_store.backend: s3`), not a code change.

## Observability

With the server running (default `:9090`):

- `GET /metrics` — Prometheus text format. Key series:
  - `crawler_pages_total{domain,result}` — derive pages/sec and error
    rates with `rate(crawler_pages_total{result="success"}[1m])`.
  - `crawler_queue_depth` — live frontier queue size.
  - Standard Go runtime and process collectors.
- `GET /healthz` — 200 `{"status":"ok"}` if Postgres is reachable, 503
  with per-check error detail otherwise.

Structured logs go to stdout via `log/slog`, in text or JSON
(`logging.format`), with per-request context (URL, domain, depth) on key
events.

## Operations

- **Resumability**: the frontier's queue and visited set are snapshotted
  to `data/checkpoint.json` every 10 seconds and on exit (normal or
  signaled). Restarting the crawler with the same `data/` directory
  resumes without re-visiting completed URLs or losing in-flight work.
- **Dead-letter queue**: URLs that exhaust their retry budget are appended
  to `data/deadletter.jsonl` (one JSON object per line) with the error and
  attempt count — nothing fails silently.
- **Graceful shutdown**: SIGINT/SIGTERM pauses the workers so no new
  fetches start, lets in-flight fetches finish normally, then persists a
  final checkpoint before exiting. If a fetch hasn't finished within
  `server.grace_period` (default 30s), it's forcibly canceled and its URL
  goes back into the queue, so the next run picks it up. Verified
  against `docker stop` (real SIGTERM) with a deliberately slow in-flight
  request: the page was fetched, persisted, and the process exited cleanly
  only afterward.
  > Note: this can't be exercised via raw signal delivery on native
  > Windows (`os.Process.Signal` there returns "not supported"); it works
  > as intended under Docker/Linux, which is the deployment target.

## Development

```sh
go build ./...
go vet ./...
go test ./... -race
```

Postgres-backed tests in `internal/storage/postgres` are integration tests,
skipped unless `TEST_POSTGRES_DSN` is set:

```sh
docker run -d -p 15432:5432 \
  -e POSTGRES_USER=crawler -e POSTGRES_PASSWORD=crawler -e POSTGRES_DB=crawler_test \
  postgres:16-alpine

TEST_POSTGRES_DSN="postgres://crawler:crawler@127.0.0.1:15432/crawler_test?sslmode=disable" \
  go test ./internal/storage/postgres/... -v
```

Everything else runs with no external dependencies.
