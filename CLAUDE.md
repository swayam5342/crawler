Build a production-grade web crawler in Go. I want it robust enough to run
unattended against real-world sites, not a toy scraper.


When committing, never include Co-Authored-By or any Claude/Anthropic attribution in commit messages.

## Core requirements

1. **Concurrency & rate limiting**
   - Worker pool with configurable concurrency (per-domain AND global limits)
   - Per-domain rate limiting (token bucket), respecting Crawl-delay from robots.txt
   - Context-based cancellation propagated through the whole pipeline

2. **Politeness & compliance**
   - robots.txt parsing and caching (with TTL + refresh)
   - Configurable User-Agent, honor noindex/nofollow where relevant
   - Same-domain / allowed-domain scoping with a pluggable URL filter

3. **Fallbacks & resilience** (this is the important part — I want defense in depth)
   - Retry with exponential backoff + jitter on transient errors (5xx, timeouts, connection resets)
   - Circuit breaker per-domain: stop hammering a domain that's consistently failing, cool down, retry later
   - Timeouts at every layer: DNS resolution, TCP connect, TLS handshake, per-request, and overall crawl deadline
   - Graceful handling of: redirects (with loop detection and max-hop limit), malformed HTML, non-UTF8 encodings, oversized responses (cap body size), non-200 status codes, dead/unreachable hosts
   - Fallback HTTP client behavior: if HTTP/2 fails, fall back to HTTP/1.1; handle both compressed (gzip/br) and uncompressed responses
   - Dead letter queue (or persistent failure log) for URLs that exhausted retries, so nothing silently vanishes
   - Checkpointing/resumability: persist frontier + visited state so a crashed or restarted crawl can resume without starting over

4. **Data pipeline**
   - Deduplication (normalize URLs — strip fragments, sort query params, etc. — and use a bloom filter or persistent visited-set for scale)
   - Pluggable storage backend interface (start with local disk/SQLite, but design so Postgres/S3 could be swapped in)
   - Structured extraction interface (e.g., link extraction + optional pluggable content parser)

5. **Observability**
   - Structured logging (slog), with per-request and per-domain context
   - Metrics: pages/sec, error rates, queue depth, per-domain success/failure counts (expose via Prometheus-style /metrics endpoint if reasonable)
   - Health check endpoint

6. **Config & operability**
   - All tunables (concurrency, rate limits, timeouts, retry policy, max depth, allowed domains, storage backend) via a config file (YAML) + env var overrides
   - Clean shutdown on SIGINT/SIGTERM that finishes in-flight requests and persists state
   - Reasonable defaults so it runs safely out of the box

## Architecture preferences
- Idiomatic Go, standard library first; only pull in well-justified deps (e.g. golang.org/x/net/html, robots.txt parser, a logging lib if slog isn't enough)
- Clear package boundaries: fetcher, frontier/scheduler, storage, robots, config, metrics
- Interfaces for the swappable parts (storage, extractor, filter) so this isn't a monolith
- Table-driven tests for the tricky bits (URL normalization, retry/backoff logic, robots.txt parsing, circuit breaker state transitions)
- Include a docker-compose or Dockerfile for running it

## Workflow instructions
- Work feature by feature, in the order above (concurrency core → politeness → fallbacks/resilience → pipeline → observability → config/ops)
- Commit after each completed feature with a clear, conventional-commit-style message (e.g. `feat: add per-domain rate limiter with token bucket`)
- Do NOT add "Co-Authored-By: Claude" or any Claude/Anthropic attribution to commit messages — author them as if from me
- Before starting, give me a brief plan of the package layout and the order you'll implement things in, so I can sanity-check before you start writing code