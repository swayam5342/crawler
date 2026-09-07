// Package config defines the crawler's configuration schema and loading
// (YAML file with environment variable overrides).
package config

import "time"

// Config is the root configuration for the crawler.
type Config struct {
	Crawl      CrawlConfig      `yaml:"crawl"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
	RateLimit  RateLimitConfig  `yaml:"rate_limit"`
	Timeouts   TimeoutsConfig   `yaml:"timeouts"`
	Retry      RetryConfig      `yaml:"retry"`
	Breaker    BreakerConfig    `yaml:"circuit_breaker"`
	Robots     RobotsConfig     `yaml:"robots"`
	Storage    StorageConfig    `yaml:"storage"`
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// CrawlConfig controls crawl scope and boundaries.
type CrawlConfig struct {
	SeedURLs       []string      `yaml:"seed_urls"`
	AllowedDomains []string      `yaml:"allowed_domains"`
	MaxDepth       int           `yaml:"max_depth"`
	UserAgent      string        `yaml:"user_agent"`
	MaxBodyBytes   int64         `yaml:"max_body_bytes"`
	MaxRedirects   int           `yaml:"max_redirects"`
	Deadline       time.Duration `yaml:"deadline"`
	HonorNofollow  bool          `yaml:"honor_nofollow"`
	HonorNoindex   bool          `yaml:"honor_noindex"`
}

// ConcurrencyConfig controls worker pool sizing.
type ConcurrencyConfig struct {
	Global    int `yaml:"global"`
	PerDomain int `yaml:"per_domain"`
}

// RateLimitConfig controls per-domain request pacing.
type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

// TimeoutsConfig controls timeouts at every network layer plus the overall
// crawl deadline.
type TimeoutsConfig struct {
	DNS       time.Duration `yaml:"dns"`
	Connect   time.Duration `yaml:"connect"`
	TLS       time.Duration `yaml:"tls"`
	Request   time.Duration `yaml:"request"`
	Overall   time.Duration `yaml:"overall"`
}

// RetryConfig controls retry/backoff behavior on transient errors.
type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	BaseBackoff  time.Duration `yaml:"base_backoff"`
	MaxBackoff   time.Duration `yaml:"max_backoff"`
	JitterFactor float64       `yaml:"jitter_factor"`
}

// BreakerConfig controls per-domain circuit breaker behavior.
type BreakerConfig struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	CooldownPeriod   time.Duration `yaml:"cooldown_period"`
	HalfOpenMaxCalls int           `yaml:"half_open_max_calls"`
}

// RobotsConfig controls robots.txt fetching and caching.
type RobotsConfig struct {
	CacheTTL time.Duration `yaml:"cache_ttl"`
	Timeout  time.Duration `yaml:"timeout"`
}

// StorageConfig controls the structured store and blob store backends.
type StorageConfig struct {
	Postgres  PostgresConfig  `yaml:"postgres"`
	BlobStore BlobStoreConfig `yaml:"blob_store"`
}

// PostgresConfig configures the Postgres connection used for structured
// state (frontier, visited set, dead-letter, checkpoints, page metadata).
type PostgresConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// BlobStoreConfig configures where raw page bodies are stored.
type BlobStoreConfig struct {
	// Backend is "local" or "s3".
	Backend string          `yaml:"backend"`
	Local   LocalBlobConfig `yaml:"local"`
	S3      S3BlobConfig    `yaml:"s3"`
}

// LocalBlobConfig configures the disk-backed blob store.
type LocalBlobConfig struct {
	Dir string `yaml:"dir"`
}

// S3BlobConfig configures the S3 blob store.
type S3BlobConfig struct {
	Bucket    string `yaml:"bucket"`
	Region    string `yaml:"region"`
	Prefix    string `yaml:"prefix"`
	Endpoint  string `yaml:"endpoint"` // for S3-compatible services; empty uses AWS default
}

// ServerConfig controls the metrics/health HTTP server.
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// LoggingConfig controls structured logging output.
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text or json
}
