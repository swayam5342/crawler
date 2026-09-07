package config

import "time"

// Default returns a Config populated with safe, conservative defaults so the
// crawler runs sanely out of the box with no config file at all.
func Default() Config {
	return Config{
		Crawl: CrawlConfig{
			MaxDepth:      5,
			UserAgent:     "web_crawler/1.0 (+https://example.com/bot)",
			MaxBodyBytes:  10 << 20, // 10 MiB
			MaxRedirects:  10,
			HonorNofollow: true,
			HonorNoindex:  true,
		},
		Concurrency: ConcurrencyConfig{
			Global:    50,
			PerDomain: 2,
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
		},
		Timeouts: TimeoutsConfig{
			DNS:     5 * time.Second,
			Connect: 5 * time.Second,
			TLS:     5 * time.Second,
			Request: 30 * time.Second,
			Overall: 1 * time.Hour,
		},
		Retry: RetryConfig{
			MaxAttempts:  4,
			BaseBackoff:  500 * time.Millisecond,
			MaxBackoff:   30 * time.Second,
			JitterFactor: 0.3,
		},
		Breaker: BreakerConfig{
			FailureThreshold: 5,
			CooldownPeriod:   1 * time.Minute,
			HalfOpenMaxCalls: 1,
		},
		Robots: RobotsConfig{
			CacheTTL: 1 * time.Hour,
			Timeout:  10 * time.Second,
		},
		Storage: StorageConfig{
			Postgres: PostgresConfig{
				DSN:             "postgres://crawler:crawler@localhost:5432/crawler?sslmode=disable",
				MaxOpenConns:    10,
				MaxIdleConns:    5,
				ConnMaxLifetime: 30 * time.Minute,
			},
			BlobStore: BlobStoreConfig{
				Backend: "local",
				Local: LocalBlobConfig{
					Dir: "./data/blobs",
				},
			},
		},
		Server: ServerConfig{
			Addr: ":9090",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}
