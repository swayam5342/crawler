// Command crawler runs the web crawler.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/swayamsb50/web_crawler/internal/api"
	"github.com/swayamsb50/web_crawler/internal/breaker"
	"github.com/swayamsb50/web_crawler/internal/config"
	"github.com/swayamsb50/web_crawler/internal/control"
	"github.com/swayamsb50/web_crawler/internal/crawler"
	"github.com/swayamsb50/web_crawler/internal/deadletter"
	"github.com/swayamsb50/web_crawler/internal/extractor"
	"github.com/swayamsb50/web_crawler/internal/fetcher"
	"github.com/swayamsb50/web_crawler/internal/frontier"
	"github.com/swayamsb50/web_crawler/internal/health"
	"github.com/swayamsb50/web_crawler/internal/metrics"
	"github.com/swayamsb50/web_crawler/internal/ratelimit"
	"github.com/swayamsb50/web_crawler/internal/robots"
	"github.com/swayamsb50/web_crawler/internal/storage/blobstore"
	blobstorelocal "github.com/swayamsb50/web_crawler/internal/storage/blobstore/local"
	blobstores3 "github.com/swayamsb50/web_crawler/internal/storage/blobstore/s3"
	"github.com/swayamsb50/web_crawler/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := newLogger(cfg.Logging)
	slog.SetDefault(logger)

	logger.Info("config loaded",
		"config_path", *configPath,
		"global_concurrency", cfg.Concurrency.Global,
		"per_domain_concurrency", cfg.Concurrency.PerDomain,
		"seed_urls", len(cfg.Crawl.SeedURLs),
		"keep_alive", cfg.Crawl.KeepAlive,
	)

	// Frontier, with checkpoint resume if a prior checkpoint exists. In
	// keep-alive mode it stays open when drained so the dashboard can add
	// more URLs.
	filter := frontier.NewDomainFilter(cfg.Crawl.AllowedDomains)
	var frontierOpts []frontier.Option
	if cfg.Crawl.KeepAlive {
		frontierOpts = append(frontierOpts, frontier.WithKeepAlive())
	}
	f := frontier.New(filter, frontierOpts...)

	checkpointStore := frontier.FileCheckpointStore{Path: filepath.Join("data", "checkpoint.json")}
	if cp, found, err := checkpointStore.Load(); err != nil {
		logger.Warn("failed to load checkpoint; starting fresh", "error", err)
	} else if found {
		f.Restore(cp)
		logger.Info("resumed from checkpoint", "queued_items", len(cp.Queue), "visited", len(cp.Visited))
	}

	for _, seed := range cfg.Crawl.SeedURLs {
		if _, err := f.Add(seed, 0); err != nil {
			logger.Warn("skipping invalid seed URL", "url", seed, "error", err)
		}
	}

	// Politeness: robots.txt + per-domain rate limiting.
	robotsChecker := robots.New(http.DefaultClient, cfg.Crawl.UserAgent, cfg.Robots.CacheTTL, cfg.Robots.Timeout)
	limiter := ratelimit.New(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)

	// Resilience: circuit breaker, resilient HTTP client, dead-letter log.
	breakerMgr := breaker.NewManager(breaker.Config{
		FailureThreshold: cfg.Breaker.FailureThreshold,
		CooldownPeriod:   cfg.Breaker.CooldownPeriod,
		HalfOpenMaxCalls: cfg.Breaker.HalfOpenMaxCalls,
	})

	httpClient := fetcher.New(fetcher.Config{
		UserAgent:      cfg.Crawl.UserAgent,
		MaxBodyBytes:   cfg.Crawl.MaxBodyBytes,
		MaxRedirects:   cfg.Crawl.MaxRedirects,
		DNSTimeout:     cfg.Timeouts.DNS,
		ConnectTimeout: cfg.Timeouts.Connect,
		TLSTimeout:     cfg.Timeouts.TLS,
		RequestTimeout: cfg.Timeouts.Request,
		MaxAttempts:    cfg.Retry.MaxAttempts,
		BaseBackoff:    cfg.Retry.BaseBackoff,
		MaxBackoff:     cfg.Retry.MaxBackoff,
		JitterFactor:   cfg.Retry.JitterFactor,
		Breaker:        breakerMgr,
	})

	deadLetterSink, err := deadletter.NewFileSink(filepath.Join("data", "deadletter.jsonl"))
	if err != nil {
		return fmt.Errorf("opening dead-letter sink: %w", err)
	}
	defer deadLetterSink.Close()

	// Setup calls (connecting, not crawling) get their own short-lived
	// context, independent of the crawl's overall deadline or shutdown
	// signal handling below.
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	// Data pipeline: structured storage (Postgres) + raw body blob store.
	storageBackend, err := postgres.Open(setupCtx, postgres.Config{
		DSN:             cfg.Storage.Postgres.DSN,
		MaxOpenConns:    int32(cfg.Storage.Postgres.MaxOpenConns),
		MaxIdleConns:    int32(cfg.Storage.Postgres.MaxIdleConns),
		ConnMaxLifetime: cfg.Storage.Postgres.ConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("connecting to postgres storage backend: %w", err)
	}
	defer storageBackend.Close()

	blobStore, err := newBlobStore(setupCtx, cfg.Storage.BlobStore)
	if err != nil {
		return fmt.Errorf("initializing blob store: %w", err)
	}

	fetch := &pageFetcher{
		client:        httpClient,
		extractor:     extractor.NewHTML(),
		storage:       storageBackend,
		blobs:         blobStore,
		honorNofollow: cfg.Crawl.HonorNofollow,
		honorNoindex:  cfg.Crawl.HonorNoindex,
		logger:        logger,
	}

	// Control plane: pause/resume, add URLs, retry failures, reset, live
	// stats. Exposed as a JSON API consumed by the web dashboard (web/).
	gate := crawler.NewPauseGate(false)
	ctl, err := control.New(control.Options{
		Frontier:     f,
		Filter:       filter,
		Gate:         gate,
		DeadLetter:   deadLetterSink,
		Checkpoint:   checkpointStore,
		Pages:        storageBackend,
		Purger:       storageBackend,
		BreakerState: func(domain string) string { return breakerMgr.State(domain).String() },
		KeepAlive:    cfg.Crawl.KeepAlive,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("creating control plane: %w", err)
	}

	// Observability: Prometheus metrics + health check, served over HTTP
	// alongside the crawl.
	m := metrics.New()
	m.SetQueueDepthFunc(func() float64 { return float64(f.Len()) })

	if cfg.Server.APIToken == "" {
		logger.Warn("control API has no token; set server.api_token / CRAWLER_API_TOKEN if this port is reachable by others")
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.Handle("/healthz", health.Handler(5*time.Second, map[string]health.Check{
		"postgres": storageBackend.Ping,
	}))
	mux.Handle("/api/", api.Handler(ctl, api.Options{
		Token:          cfg.Server.APIToken,
		AllowedOrigins: cfg.Server.CORSOrigins,
		Config:         sanitizedConfig(cfg),
		Logger:         logger,
	}))
	metricsServer := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("API/metrics/health server listening", "addr", cfg.Server.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn("metrics/health server stopped unexpectedly", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracePeriod)
		defer shutdownCancel()
		metricsServer.Shutdown(shutdownCtx) //nolint:errcheck
	}()

	pool := crawler.New(f, fetch, crawler.Config{
		GlobalWorkers:  cfg.Concurrency.Global,
		PerDomainLimit: cfg.Concurrency.PerDomain,
		MaxDepth:       cfg.Crawl.MaxDepth,
		Logger:         logger,
		Robots:         robotsChecker,
		Limiter:        limiter,
		DeadLetter:     deadLetterAdapter{sink: deadLetterSink},
		Metrics:        m,
		Gate:           gate,
		Events:         ctl,
	})

	stopCheckpointing := make(chan struct{})
	checkpointDone := make(chan struct{})
	go periodicCheckpoint(f, checkpointStore, logger, 10*time.Second, stopCheckpointing, checkpointDone)

	// workCtx bounds the crawl itself. In batch mode it carries the overall
	// crawl deadline; in keep-alive (service) mode it runs until shutdown.
	// It is deliberately NOT canceled immediately on SIGINT/SIGTERM:
	// graceful shutdown pauses the pool (no new work is started) and lets
	// in-flight fetches finish, only force-canceling workCtx if they don't
	// finish within GracePeriod. Anything still queued stays in the
	// checkpoint for the next run.
	var (
		workCtx    context.Context
		cancelWork context.CancelFunc
	)
	if cfg.Crawl.KeepAlive {
		workCtx, cancelWork = context.WithCancel(context.Background())
	} else {
		workCtx, cancelWork = context.WithTimeout(context.Background(), cfg.Timeouts.Overall)
	}
	defer cancelWork()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	poolDone := make(chan struct{})
	logger.Info("starting crawl", "keep_alive", cfg.Crawl.KeepAlive, "overall_deadline", cfg.Timeouts.Overall)
	go func() {
		defer close(poolDone)
		pool.Run(workCtx)
	}()

	select {
	case <-poolDone:
		if workCtx.Err() == nil {
			ctl.MarkFinished()
			logger.Info("frontier drained; crawl complete")
		} else {
			logger.Warn("overall crawl deadline reached")
		}
	case sig := <-sigCh:
		logger.Info("received shutdown signal; draining in-flight requests", "signal", sig.String(), "grace_period", cfg.Server.GracePeriod)
		ctl.MarkStopping()
		gate.Pause()
		waitForInFlight(f, cfg.Server.GracePeriod, logger)
		cancelWork()
		<-poolDone
	case <-workCtx.Done():
		logger.Warn("overall crawl deadline reached; stopping")
		<-poolDone
	}
	cancelWork()

	close(stopCheckpointing)
	<-checkpointDone

	if err := checkpointStore.Save(f.Snapshot()); err != nil {
		logger.Warn("failed to save final checkpoint", "error", err)
	}

	logger.Info("crawl finished", "visited", f.Visited())
	return nil
}

// waitForInFlight polls until no fetches are in flight or grace elapses.
func waitForInFlight(f *frontier.Frontier, grace time.Duration, logger *slog.Logger) {
	deadline := time.Now().Add(grace)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if f.Stats().InFlight == 0 {
			return
		}
		if time.Now().After(deadline) {
			logger.Warn("shutdown grace period elapsed; forcing cancellation of in-flight requests", "in_flight", f.Stats().InFlight)
			return
		}
	}
}

// sanitizedConfig returns the config with secrets redacted, shaped like
// the YAML file (snake_case keys, "30s"-style durations), for display in
// the dashboard.
func sanitizedConfig(cfg config.Config) any {
	if u, err := url.Parse(cfg.Storage.Postgres.DSN); err == nil && u.User != nil {
		if _, hasPw := u.User.Password(); hasPw {
			u.User = url.UserPassword(u.User.Username(), "redacted")
		}
		cfg.Storage.Postgres.DSN = u.String()
	} else if err != nil {
		cfg.Storage.Postgres.DSN = "(redacted)"
	}
	if cfg.Server.APIToken != "" {
		cfg.Server.APIToken = "(set)"
	}
	var view map[string]any
	if data, err := yaml.Marshal(cfg); err == nil && yaml.Unmarshal(data, &view) == nil {
		return view
	}
	return cfg
}

// newBlobStore builds the configured blob store backend for raw page
// bodies: local disk or S3.
func newBlobStore(ctx context.Context, cfg config.BlobStoreConfig) (blobstore.Store, error) {
	switch cfg.Backend {
	case "s3":
		return blobstores3.New(ctx, blobstores3.Config{
			Bucket:   cfg.S3.Bucket,
			Region:   cfg.S3.Region,
			Prefix:   cfg.S3.Prefix,
			Endpoint: cfg.S3.Endpoint,
		})
	case "local", "":
		return blobstorelocal.New(cfg.Local.Dir)
	default:
		return nil, fmt.Errorf("unknown blob store backend %q", cfg.Backend)
	}
}

// deadLetterAdapter adapts *deadletter.FileSink to crawler.DeadLetter.
type deadLetterAdapter struct {
	sink *deadletter.FileSink
}

func (a deadLetterAdapter) Record(url, domain string, depth, attempts int, cause error) error {
	return a.sink.Record(deadletter.Entry{
		URL:      url,
		Domain:   domain,
		Depth:    depth,
		Attempts: attempts,
		Error:    cause.Error(),
	})
}

// periodicCheckpoint saves a frontier snapshot on a fixed interval until
// stop is closed, then signals done.
func periodicCheckpoint(f *frontier.Frontier, store frontier.FileCheckpointStore, logger *slog.Logger, interval time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := store.Save(f.Snapshot()); err != nil {
				logger.Warn("periodic checkpoint save failed", "error", err)
			}
		}
	}
}

func newLogger(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
