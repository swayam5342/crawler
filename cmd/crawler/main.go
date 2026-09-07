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
	"github.com/swayamsb50/web_crawler/internal/fetcher"
	"github.com/swayamsb50/web_crawler/internal/frontier"
	"github.com/swayamsb50/web_crawler/internal/ratelimit"
	"github.com/swayamsb50/web_crawler/internal/robots"
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

	pool := crawler.New(f, &pageFetcher{client: httpClient}, crawler.Config{
		GlobalWorkers:  cfg.Concurrency.Global,
		PerDomainLimit: cfg.Concurrency.PerDomain,
		MaxDepth:       cfg.Crawl.MaxDepth,
		Logger:         logger,
		Robots:         robotsChecker,
		Limiter:        limiter,
		DeadLetter:     deadLetterAdapter{sink: deadLetterSink},
	})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Overall)
	defer cancel()

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
