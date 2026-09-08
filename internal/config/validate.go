package config

import "fmt"

// Validate checks the config for internally-consistent, sane values.
func (c Config) Validate() error {
	if c.Concurrency.Global <= 0 {
		return fmt.Errorf("concurrency.global must be > 0")
	}
	if c.Concurrency.PerDomain <= 0 {
		return fmt.Errorf("concurrency.per_domain must be > 0")
	}
	if c.Concurrency.PerDomain > c.Concurrency.Global {
		return fmt.Errorf("concurrency.per_domain (%d) must not exceed concurrency.global (%d)", c.Concurrency.PerDomain, c.Concurrency.Global)
	}
	if c.RateLimit.RequestsPerSecond <= 0 {
		return fmt.Errorf("rate_limit.requests_per_second must be > 0")
	}
	if c.RateLimit.Burst <= 0 {
		return fmt.Errorf("rate_limit.burst must be > 0")
	}
	if c.Crawl.MaxDepth < 0 {
		return fmt.Errorf("crawl.max_depth must be >= 0")
	}
	if c.Crawl.UserAgent == "" {
		return fmt.Errorf("crawl.user_agent must not be empty")
	}
	if c.Crawl.MaxBodyBytes <= 0 {
		return fmt.Errorf("crawl.max_body_bytes must be > 0")
	}
	if c.Crawl.MaxRedirects < 0 {
		return fmt.Errorf("crawl.max_redirects must be >= 0")
	}
	if c.Timeouts.DNS <= 0 || c.Timeouts.Connect <= 0 || c.Timeouts.TLS <= 0 || c.Timeouts.Request <= 0 || c.Timeouts.Overall <= 0 {
		return fmt.Errorf("all timeouts must be > 0")
	}
	if c.Retry.MaxAttempts < 1 {
		return fmt.Errorf("retry.max_attempts must be >= 1")
	}
	if c.Retry.BaseBackoff <= 0 {
		return fmt.Errorf("retry.base_backoff must be > 0")
	}
	if c.Retry.MaxBackoff < c.Retry.BaseBackoff {
		return fmt.Errorf("retry.max_backoff must be >= retry.base_backoff")
	}
	if c.Retry.JitterFactor < 0 || c.Retry.JitterFactor > 1 {
		return fmt.Errorf("retry.jitter_factor must be within [0,1]")
	}
	if c.Breaker.FailureThreshold <= 0 {
		return fmt.Errorf("circuit_breaker.failure_threshold must be > 0")
	}
	if c.Breaker.CooldownPeriod <= 0 {
		return fmt.Errorf("circuit_breaker.cooldown_period must be > 0")
	}
	if c.Breaker.HalfOpenMaxCalls <= 0 {
		return fmt.Errorf("circuit_breaker.half_open_max_calls must be > 0")
	}
	if c.Robots.CacheTTL <= 0 {
		return fmt.Errorf("robots.cache_ttl must be > 0")
	}
	if c.Robots.Timeout <= 0 {
		return fmt.Errorf("robots.timeout must be > 0")
	}
	if c.Storage.Postgres.DSN == "" {
		return fmt.Errorf("storage.postgres.dsn must not be empty")
	}
	switch c.Storage.BlobStore.Backend {
	case "local":
		if c.Storage.BlobStore.Local.Dir == "" {
			return fmt.Errorf("storage.blob_store.local.dir must not be empty when backend is 'local'")
		}
	case "s3":
		if c.Storage.BlobStore.S3.Bucket == "" {
			return fmt.Errorf("storage.blob_store.s3.bucket must not be empty when backend is 's3'")
		}
	default:
		return fmt.Errorf("storage.blob_store.backend must be 'local' or 's3', got %q", c.Storage.BlobStore.Backend)
	}
	if c.Server.Addr == "" {
		return fmt.Errorf("server.addr must not be empty")
	}
	if c.Server.GracePeriod <= 0 {
		return fmt.Errorf("server.grace_period must be > 0")
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of debug/info/warn/error, got %q", c.Logging.Level)
	}
	switch c.Logging.Format {
	case "text", "json":
	default:
		return fmt.Errorf("logging.format must be 'text' or 'json', got %q", c.Logging.Format)
	}
	return nil
}
