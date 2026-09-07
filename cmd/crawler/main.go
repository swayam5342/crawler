// Command crawler runs the web crawler.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/swayamsb50/web_crawler/internal/config"
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

	// TODO: wire up frontier, fetcher, storage, and worker pool (crawler package).
	logger.Info("crawler scaffold ready; pipeline not yet implemented")

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
