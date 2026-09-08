// Command crawler runs the web crawler.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/swayamsb50/web_crawler/internal/breaker"
	"github.com/swayamsb50/web_crawler/internal/config"
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
	)

	// Frontier, with checkpoint resume if a prior checkpoint exists.
	filter := frontier.NewDomainFilter(cfg.Crawl.AllowedDomains)
	f := frontier.New(filter)

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

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Overall)
	defer cancel()

	// Data pipeline: structured storage (Postgres) + raw body blob store.
	storageBackend, err := postgres.Open(ctx, postgres.Config{
		DSN:             cfg.Storage.Postgres.DSN,
		MaxOpenConns:    int32(cfg.Storage.Postgres.MaxOpenConns),
		MaxIdleConns:    int32(cfg.Storage.Postgres.MaxIdleConns),
		ConnMaxLifetime: cfg.Storage.Postgres.ConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("connecting to postgres storage backend: %w", err)
	}
	defer storageBackend.Close()

	blobStore, err := newBlobStore(ctx, cfg.Storage.BlobStore)
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

	// Observability: Prometheus metrics + health check, served over HTTP
	// alongside the crawl.
	m := metrics.New()
	m.SetQueueDepthFunc(func() float64 { return float64(f.Len()) })

	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.Handle("/healthz", health.Handler(5*time.Second, map[string]health.Check{
		"postgres": storageBackend.Ping,
	}))
	metricsServer := &http.Server{Addr: cfg.Server.Addr, Handler: mux}
	go func() {
		logger.Info("metrics/health server listening", "addr", cfg.Server.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn("metrics/health server stopped unexpectedly", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	})

	stopCheckpointing := make(chan struct{})
	checkpointDone := make(chan struct{})
	go periodicCheckpoint(f, checkpointStore, logger, 10*time.Second, stopCheckpointing, checkpointDone)

	logger.Info("starting crawl", "overall_deadline", cfg.Timeouts.Overall)
	pool.Run(ctx)
	close(stopCheckpointing)
	<-checkpointDone

	if err := checkpointStore.Save(f.Snapshot()); err != nil {
		logger.Warn("failed to save final checkpoint", "error", err)
	}

	logger.Info("crawl finished", "visited", f.Visited())
	return nil
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
