// Command crawler runs the web crawler.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/swayamsb50/web_crawler/internal/config"
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

	filter := frontier.NewDomainFilter(cfg.Crawl.AllowedDomains)
	f := frontier.New(filter)
	for _, seed := range cfg.Crawl.SeedURLs {
		if _, err := f.Add(seed, 0); err != nil {
			logger.Warn("skipping invalid seed URL", "url", seed, "error", err)
		}
	}

	robotsChecker := robots.New(http.DefaultClient, cfg.Crawl.UserAgent, cfg.Robots.CacheTTL, cfg.Robots.Timeout)
	limiter := ratelimit.New(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)

	logger.Info("politeness layer ready",
		"allowed_domains", cfg.Crawl.AllowedDomains,
		"robots_cache_ttl", cfg.Robots.CacheTTL,
		"default_rate_limit_rps", cfg.RateLimit.RequestsPerSecond,
	)
	_ = robotsChecker
	_ = limiter

	// TODO: wire up the fetcher (HTTP client with retries/timeouts/circuit
	// breaker) and storage backend, then run crawler.Pool.
	logger.Info("crawler scaffold ready; fetch pipeline not yet implemented")

	return nil
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
