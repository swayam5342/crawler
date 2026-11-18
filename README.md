<div align="center">

<img src="docs/icon.svg" width="88" height="88" alt="web_crawler">

# web_crawler

**A web crawler that backs off.**

`web crawler` is a self-hosted web crawler written in Go, built to run unattended against real-world sites. It reads **robots.txt**, paces itself per domain, stops calling hosts that keep failing, retries with backoff, and resumes where it stopped after a crash. A **Next.js dashboard** lets you add URLs, pause, retry failures and reset state while it runs.

[![CI](https://img.shields.io/github/actions/workflow/status/swayam5342/crawler/ci.yml?branch=main&style=flat-square&label=CI&logo=githubactions&logoColor=white&labelColor=0d3a55)](https://github.com/swayam5342/crawler/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25%2B-4cb3f0?style=flat-square&logo=go&logoColor=white&labelColor=0d3a55)](go.mod)
[![Docker](https://img.shields.io/badge/run%20with-Docker%20Compose-4cb3f0?style=flat-square&logo=docker&logoColor=white&labelColor=0d3a55)](#option-a-docker-compose-recommended)
[![Storage](https://img.shields.io/badge/storage-Postgres%20%2B%20disk%20or%20S3-4cb3f0?style=flat-square&logo=postgresql&logoColor=white&labelColor=0d3a55)](#data-pipeline)
[![Metrics](https://img.shields.io/badge/metrics-Prometheus-4cb3f0?style=flat-square&logo=prometheus&logoColor=white&labelColor=0d3a55)](#observability)
[![Dashboard](https://img.shields.io/badge/dashboard-Next.js-4cb3f0?style=flat-square&logo=nextdotjs&logoColor=white&labelColor=0d3a55)](#dashboard)

[**Website**](http://swayamcyber.in/crawler/) &nbsp;&middot;&nbsp; [Dashboard](#dashboard) &nbsp;&middot;&nbsp; [Control API](#control-api) &nbsp;&middot;&nbsp; [Configuration](#configuration) &nbsp;&middot;&nbsp; [Architecture](#architecture)

</div>

<br>

## Table of Contents

| | |
| :--- | :--- |
| **Overview** | [What It Does With Every URL](#what-it-does-with-every-url) &middot; [When Things Go Wrong](#when-things-go-wrong) |
| **Setup** | [Prerequisites](#prerequisites) &middot; [Running the Crawler](#running-the-crawler) &middot; [Option A: Docker Compose (Recommended)](#option-a-docker-compose-recommended) &middot; [Option B: Locally (Bare Metal)](#option-b-locally-bare-metal) |
| **Use** | [Dashboard](#dashboard) &middot; [Control API](#control-api) &middot; [API Examples (CURL)](#api-examples-curl) |
| **Reference** | [Configuration](#configuration) &middot; [Architecture](#architecture) &middot; [Data Pipeline](#data-pipeline) &middot; [Observability](#observability) &middot; [Operations](#operations) &middot; [Verification & Testing](#verification--testing) |

<br>

## What It Does With Every URL

Every link the crawler finds takes the same path, in this order. Any step can stop it.

| | Step | What happens | Package |
| :---: | :--- | :--- | :--- |
| 1 | **Normalize** | Fragments stripped, query parameters sorted, default ports dropped; a URL seen before, in any spelling, is skipped. | `internal/urlutil` |
| 2 | **Scope** | Only allowed domains (and their subdomains) get through. | `internal/frontier` |
| 3 | **Robots** | robots.txt is fetched once, cached with a TTL and checked before every request. | `internal/robots` |
| 4 | **Pace** | A per-domain concurrency cap and token-bucket rate limit; a `Crawl-delay` overrides the default pace. | `internal/ratelimit` |
| 5 | **Fetch** | Separate DNS, connect, TLS and request timeouts, retries with backoff, a per-domain circuit breaker. | `internal/fetcher` |
| 6 | **Extract** | Links, title and language from HTML in any encoding; `nofollow` and `noindex` honored. | `internal/extractor` |
| 7 | **Store** | Metadata to Postgres, the raw body to local disk or S3, keyed by content hash. | `internal/storage` |

<br>

## When Things Go Wrong

| When | What the crawler does |
| :--- | :--- |
| A server answers 503, times out or resets the connection | Retries with exponential backoff and jitter: up to **4 attempts**, starting at **500 ms**, never waiting more than **30 s**. |
| A whole domain keeps failing | After **5 failures in a row** its circuit breaker opens: no requests for **1 minute**, then one trial request decides whether to resume. |
| A URL still fails after every retry | It's written to the dead-letter log with its error and attempt count, and you can retry it from the dashboard. Nothing is dropped silently. |
| A page redirects in a loop, or too many times | Loops are detected and hops are capped at **10**. |
| A response is huge or the HTML is broken | Bodies stop at **10 MiB**; malformed markup and non-UTF-8 text still parse. |
| The process crashes or gets SIGTERM | The queue and visited set are checkpointed every **10 seconds**. On SIGTERM, new work stops, in-flight requests finish, and the next start resumes from there. |

Every limit above is a default in [`configs/config.yaml`](configs/config.yaml) and can be changed.

<br>

## Prerequisites

| Requirement | When |
| :--- | :--- |
| **Docker & Docker Compose** | Recommended. Runs Postgres, the crawler and the dashboard together. |
| **Go 1.25+** | If running the crawler locally. |
| **PostgreSQL** | If running locally; stores page metadata. |
| **Node.js 22+** | If running the dashboard locally. |

<br>

## Running the Crawler

### Option A: Docker Compose (Recommended)

**1. Clone and start everything**

```bash
git clone https://github.com/swayam5342/crawler.git
cd crawler
docker compose up --build
```

This starts Postgres (with a health check gating crawler startup), the crawler using [`configs/config.yaml`](configs/config.yaml), and the dashboard.

**2. Open the dashboard**

Go to <http://localhost:8067> and add a URL to start crawling.

> [!NOTE]
> Only the dashboard is published, and only on localhost (`127.0.0.1:8067`). Postgres and the crawler's
> API, metrics and health server (`crawler:9090`) stay on the internal compose network. Crawled data and
> blobs persist in the `postgres_data` and `crawler_data` volumes.

**3. Set an API token (strongly recommended)**

> [!IMPORTANT]
> The control API can reset state and delete stored pages. Compose shares a default token,
> `change-me-local-token`, between the crawler and the dashboard. Set your own with `CRAWLER_API_TOKEN`
> (in your shell or a `.env` file) before exposing anything beyond your machine, and set
> `DASHBOARD_PASSWORD` to require a login on the dashboard.

> [!TIP]
> If the build stops with `additional privileges requested: pass "--allow=fs.read=..."` (Compose's bake
> builder on Windows), build the images directly and start without rebuilding:
>
> ```bash
> docker build -t web_crawler-crawler .
> docker build -t web_crawler-web ./web
> docker compose -p web_crawler up -d --no-build
> ```

### Option B: Locally (Bare Metal)

**1. Start Postgres** (or point `CRAWLER_POSTGRES_DSN` at an existing instance)

```bash
docker run -d -p 5432:5432 \
  -e POSTGRES_USER=crawler -e POSTGRES_PASSWORD=crawler -e POSTGRES_DB=crawler \
  postgres:16-alpine
```

**2. Run the crawler**

```bash
go run ./cmd/crawler --config configs/config.yaml
```

The binary needs write access to `./data`, where the checkpoint, dead-letter log and local blob storage go by default.

**3. Run the dashboard**

```bash
cd web
cp .env.example .env.local   # set CRAWLER_API_URL / CRAWLER_API_TOKEN
npm install
npm run dev                  # http://localhost:3000
```

<br>

## Dashboard

[`web/`](web) is a Next.js app (served at `/dashboard`; `/` redirects there) for controlling a running crawler.

| Page | What you can do |
| :--- | :--- |
| **Overview** | Add a URL, see pages crawled, errors, robots.txt blocks, per-domain state and recent activity. |
| **Add URLs** | Paste up to 1,000 URLs; each comes back as queued, already seen or domain not allowed. |
| **Crawled pages** | Search stored pages by URL or title, filtered by domain. |
| **Queue** | See what's in progress and what's waiting; remove a queued URL. |
| **Failed URLs** | See why each URL failed, then retry or discard it, one at a time or all together. |
| **Settings** | Edit allowed domains, reset state, view the running config with secrets hidden. |

The dashboard needs the crawler in **service mode** (`crawl.keep_alive: true`, the default in `configs/config.yaml`), so it stays up when the queue drains and waits for new URLs. In batch mode (`keep_alive: false`) it exits once every reachable URL is processed, and `timeouts.overall` bounds the run.

The browser never talks to the crawler directly: the dashboard's server proxies `/api/crawler/*` to the crawler's `/api/*` and adds the API token, so the token stays server-side and no CORS setup is needed.

| Variable | Purpose |
| :--- | :--- |
| `CRAWLER_API_URL` | Crawler server address (default `http://localhost:9090`). |
| `CRAWLER_API_TOKEN` | Must match the crawler's `server.api_token`. |
| `DASHBOARD_PASSWORD` | If set, `/dashboard` requires basic auth as user `admin`. |

> [!NOTE]
> The project's showcase website lives on the [`website`](https://github.com/swayam5342/crawler/tree/website) branch and publishes to GitHub Pages.

<br>

## Control API

Served by the crawler under `/api/` on `server.addr` (`:9090`). With Docker Compose, reach it through the dashboard's proxy at `http://localhost:8067/api/crawler/...`, which adds the token for you.

| Method | Route | Purpose |
| :--- | :--- | :--- |
| `GET` | `/api/status` | Run state, queue sizes, totals, pages/sec, error rate, 60 s throughput, per-domain stats with circuit breaker state |
| `GET` | `/api/events?limit=` | Recently processed URLs, newest first |
| `POST` | `/api/urls` | Add URLs: `{"urls":[...],"allow_domain":bool,"recrawl":bool}`, returns a status per URL |
| `POST` | `/api/pause`, `/api/resume` | Stop or restart new fetches; in-flight requests finish |
| `GET` / `DELETE` | `/api/queue?limit=` / `/api/queue?url=` | Inspect the queue, or drop one queued URL |
| `GET` | `/api/pages?q=&domain=&limit=&offset=` | Browse stored pages |
| `GET` | `/api/pages/domains` | Stored page counts per domain |
| `GET` | `/api/deadletters` | Failed URLs, newest first |
| `POST` | `/api/deadletters/retry`, `/api/deadletters/discard` | `{"urls":[...]}`; an empty list means all |
| `POST` / `DELETE` | `/api/domains` / `/api/domains?domain=` | Edit the allowed domains at runtime |
| `POST` | `/api/reset` | Clear `queue`, `dead_letters`, `pages` and/or `stats` |
| `GET` | `/api/config` | Running config with secrets redacted |

| | |
| :--- | :--- |
| **Authorization** | When `server.api_token` (`CRAWLER_API_TOKEN`) is set, every request needs `Authorization: Bearer <token>`; a missing or wrong token returns `401 Unauthorized`. |
| **Content type** | `POST` bodies must be `application/json`, which keeps plain cross-site form posts out. |
| **Persistence** | Runtime changes (allowed-domain edits, resets) aren't written back to `config.yaml`. |

<details>
<summary><b>Response</b> of <code>GET /api/status</code> (abbreviated)</summary>

```json
{
  "state": "running",
  "keep_alive": true,
  "frontier": { "queued": 12, "in_flight": 2, "visited": 140, "closed": false },
  "totals": { "success": 120, "failure": 3, "blocked": 1, "links_found": 860 },
  "pages_per_second": 0.9,
  "error_rate": 0.024,
  "dead_letters": 3,
  "domains": [
    { "domain": "example.com", "success": 120, "failure": 3, "blocked": 1, "breaker": "closed" }
  ],
  "allowed_domains": ["example.com"]
}
```

</details>

<details>
<summary><b>Error Response (4xx/5xx)</b></summary>

```json
{ "error": "missing or invalid API token" }
```

</details>

<br>

## API Examples (CURL)

These go through the dashboard's proxy, so they work as-is with Docker Compose.

<details open>
<summary><b>Add URLs</b> (<code>POST /api/urls</code>)</summary>

```bash
curl -X POST http://localhost:8067/api/crawler/urls \
  -H "Content-Type: application/json" \
  -d '{ "urls": ["https://example.com/"], "allow_domain": true }'
```

```json
{ "results": [ { "url": "https://example.com/", "status": "queued" } ] }
```

</details>

<details>
<summary><b>Pause and resume</b> (<code>POST /api/pause</code>, <code>/api/resume</code>)</summary>

```bash
curl -X POST http://localhost:8067/api/crawler/pause  -H "Content-Type: application/json" -d '{}'
curl -X POST http://localhost:8067/api/crawler/resume -H "Content-Type: application/json" -d '{}'
```

</details>

<details>
<summary><b>Retry every failed URL</b> (<code>POST /api/deadletters/retry</code>)</summary>

```bash
curl -X POST http://localhost:8067/api/crawler/deadletters/retry \
  -H "Content-Type: application/json" -d '{ "urls": [] }'
```

```json
{ "requeued": 3 }
```

</details>

<details>
<summary><b>Reset the queue and counters</b> (<code>POST /api/reset</code>)</summary>

```bash
curl -X POST http://localhost:8067/api/crawler/reset \
  -H "Content-Type: application/json" -d '{ "queue": true, "stats": true }'
```

</details>

<br>

## Configuration

Config loads from a YAML file (default [`configs/config.yaml`](configs/config.yaml), override with `--config`), then environment variables override individual fields. Every tunable has a safe default, so the crawler runs with no config file at all (see [`internal/config/defaults.go`](internal/config/defaults.go)).

| Section | Purpose |
| :--- | :--- |
| `crawl` | Seed URLs, allowed domains, max depth, User-Agent, body size cap, redirect limit, nofollow/noindex handling, `keep_alive` service mode |
| `concurrency` | Global and per-domain worker limits |
| `rate_limit` | Default requests per second and burst per domain |
| `timeouts` | DNS, TCP connect, TLS handshake, per-request, and the overall crawl deadline |
| `retry` | Max attempts, backoff base and max, jitter factor |
| `circuit_breaker` | Failure threshold, cooldown period, half-open trial count |
| `robots` | robots.txt cache TTL and fetch timeout |
| `storage.postgres` | Connection string and pool sizing |
| `storage.blob_store` | `local` or `s3` backend and its settings |
| `server` | API, metrics and health bind address, shutdown grace period, API token, CORS origins |
| `logging` | Level and format (`text` or `json`) |

Every field has a `CRAWLER_*` environment variable (the full list is in [`internal/config/load.go`](internal/config/load.go)), for example:

```env
CRAWLER_SEED_URLS=https://example.com/,https://example.org/
CRAWLER_ALLOWED_DOMAINS=example.com,example.org
CRAWLER_CONCURRENCY_GLOBAL=100
CRAWLER_RATE_LIMIT_RPS=2
CRAWLER_POSTGRES_DSN=postgres://user:pass@host:5432/db?sslmode=disable
CRAWLER_BLOB_STORE_BACKEND=s3
CRAWLER_BLOB_STORE_S3_BUCKET=my-crawl-bucket
CRAWLER_KEEP_ALIVE=true
CRAWLER_API_TOKEN=change-me
CRAWLER_LOG_LEVEL=debug
```

> [!TIP]
> Comma-separated lists (seed URLs, allowed domains) are split and trimmed. Durations use Go syntax (`30s`, `5m`, `1h`).

<br>

## Architecture

Package boundaries mirror the crawl pipeline. Each is unit tested on its own and most are wired together only through small interfaces, so any piece can be swapped without touching the others.

| Package | Responsibility |
| :--- | :--- |
| `cmd/crawler` | Wiring: reads config, builds every component, runs the crawl, serves `/api`, `/metrics` and `/healthz`, handles SIGINT/SIGTERM |
| `internal/config` | YAML and env-var configuration, validation, defaults |
| `internal/urlutil` | URL normalization, used for deduplication and scoping |
| `internal/frontier` | Thread-safe dedup queue; checkpoint snapshot and restore |
| `internal/crawler` | Worker pool: pulls from the frontier, drives each fetch, feeds discovered links back in, under per-domain and global limits; pause gate |
| `internal/control` | Runtime control plane: add URLs, pause and resume, retry failures, edit scope, reset, live stats |
| `internal/api` | The control plane as a JSON HTTP API under `/api/` |
| `internal/robots` | robots.txt fetch, parse and cache with TTL; Crawl-delay |
| `internal/ratelimit` | Per-domain token bucket rate limiter |
| `internal/fetcher` | Resilient HTTP client: layered timeouts, retry with backoff and jitter, redirect-loop detection, body size caps, gzip and brotli, charset normalization, HTTP/2 to HTTP/1.1 fallback, circuit breaker |
| `internal/breaker` | Per-domain circuit breaker (closed, open, half-open) |
| `internal/deadletter` | Append-only JSONL log of permanently failed URLs |
| `internal/dedup` | Bloom filter for large-scale duplicate detection (not wired into the frontier; see the notes below) |
| `internal/extractor` | Pluggable page extraction: links, title, language, robots meta |
| `internal/storage` | Backend interface and the Postgres implementation (page metadata) |
| `internal/storage/blobstore` | Store interface with local-disk and S3 implementations (raw bodies, keyed by content hash) |
| `internal/metrics` | Prometheus collectors |
| `internal/health` | `/healthz` handler running readiness checks |
| `web` | The Next.js control dashboard |

<details>
<summary><b>Design notes</b></summary>

- **Frontier dedup is an exact in-memory set, not the bloom filter.** Checkpointing needs to list exactly which URLs were visited to resume correctly, and a bloom filter can't be enumerated. `internal/dedup` ships a tested, standalone bloom filter for cases where an exact in-process set doesn't scale (for example a sharded dedup service in front of a persistent store).
- **Link extraction covers HTML.** `internal/extractor` defines a pluggable `Extractor` interface, so another implementation (JSON APIs, sitemaps) needs no changes to the crawler or fetcher.
- **Interfaces are decoupled by structural typing.** `internal/crawler` never imports `internal/fetcher` or `internal/robots`; it defines the small interface it needs (`RobotsChecker`, `RateLimiter`, `DeadLetter`) and the concrete packages satisfy it implicitly. The dependency graph stays a DAG rooted at `cmd/crawler`.

</details>

<br>

## Data Pipeline

| What | Where | How |
| :--- | :--- | :--- |
| **Page metadata** | Postgres `pages` table | Upserted by URL; the schema is created on startup. |
| **Raw page bodies** | Blob store (local disk or S3) | Keyed by the SHA-256 of the UTF-8-normalized body, so identical content from different URLs is stored once. |

> [!TIP]
> Switching from local disk to S3 is a config change (`storage.blob_store.backend: s3`), not a code change.

<br>

## Observability

| Endpoint | What it serves |
| :--- | :--- |
| `GET /metrics` | Prometheus text format: `crawler_pages_total{domain,result}` (derive pages/sec and error rates with `rate(crawler_pages_total{result="success"}[1m])`), `crawler_queue_depth`, and the Go runtime and process collectors. |
| `GET /healthz` | `200 {"status":"ok"}` if Postgres is reachable, `503` with per-check error detail otherwise. |

Structured logs go to stdout via `log/slog`, as text or JSON (`logging.format`), with per-request context (URL, domain, depth) on key events.

<br>

## Operations

| | |
| :--- | :--- |
| **Resumability** | The frontier's queue and visited set are saved to `data/checkpoint.json` every 10 seconds and on exit. Restarting with the same `data/` directory resumes without revisiting completed URLs or losing in-flight work. |
| **Dead-letter log** | URLs that exhaust their retries are appended to `data/deadletter.jsonl` (one JSON object per line) with the error and attempt count. |
| **Graceful shutdown** | SIGINT/SIGTERM pauses the workers so no new fetches start, lets in-flight fetches finish, then saves a final checkpoint. A fetch still running after `server.grace_period` (default 30 s) is canceled and its URL goes back into the queue for the next run. |

> [!NOTE]
> Graceful shutdown can't be exercised with raw signal delivery on native Windows (`os.Process.Signal` returns
> "not supported" there). Docker and Linux are the deployment target; `docker stop` sends a real SIGTERM, and
> Compose gives the crawler a 60 s stop grace period.

<br>

## Verification & Testing

```bash
go build ./...
go vet ./...
go test ./... -race
```

The Postgres tests in `internal/storage/postgres` are integration tests, skipped unless `TEST_POSTGRES_DSN` is set:

```bash
docker run -d -p 15432:5432 \
  -e POSTGRES_USER=crawler -e POSTGRES_PASSWORD=crawler -e POSTGRES_DB=crawler_test \
  postgres:16-alpine

TEST_POSTGRES_DSN="postgres://crawler:crawler@127.0.0.1:15432/crawler_test?sslmode=disable" \
  go test ./internal/storage/postgres/... -v
```

Everything else runs with no external dependencies. The dashboard type-checks and builds with:

```bash
cd web && npm run typecheck && npm run build
```
