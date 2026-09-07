package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Load reads a YAML config file (if path is non-empty and exists), merges it
// over the defaults, applies environment variable overrides, and validates
// the result.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("reading config file: %w", err)
			}
		} else if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parsing config file: %w", err)
		}
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return Config{}, fmt.Errorf("applying env overrides: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// envOverrides maps CRAWLER_* environment variables onto config fields.
// Each entry knows how to parse its own string value.
func applyEnvOverrides(cfg *Config) error {
	type override struct {
		env   string
		apply func(string) error
	}

	overrides := []override{
		{"CRAWLER_SEED_URLS", func(v string) error {
			cfg.Crawl.SeedURLs = splitCSV(v)
			return nil
		}},
		{"CRAWLER_ALLOWED_DOMAINS", func(v string) error {
			cfg.Crawl.AllowedDomains = splitCSV(v)
			return nil
		}},
		{"CRAWLER_MAX_DEPTH", intSetter(&cfg.Crawl.MaxDepth)},
		{"CRAWLER_USER_AGENT", func(v string) error { cfg.Crawl.UserAgent = v; return nil }},
		{"CRAWLER_MAX_BODY_BYTES", int64Setter(&cfg.Crawl.MaxBodyBytes)},
		{"CRAWLER_MAX_REDIRECTS", intSetter(&cfg.Crawl.MaxRedirects)},
		{"CRAWLER_DEADLINE", durationSetter(&cfg.Crawl.Deadline)},

		{"CRAWLER_CONCURRENCY_GLOBAL", intSetter(&cfg.Concurrency.Global)},
		{"CRAWLER_CONCURRENCY_PER_DOMAIN", intSetter(&cfg.Concurrency.PerDomain)},

		{"CRAWLER_RATE_LIMIT_RPS", floatSetter(&cfg.RateLimit.RequestsPerSecond)},
		{"CRAWLER_RATE_LIMIT_BURST", intSetter(&cfg.RateLimit.Burst)},

		{"CRAWLER_TIMEOUT_DNS", durationSetter(&cfg.Timeouts.DNS)},
		{"CRAWLER_TIMEOUT_CONNECT", durationSetter(&cfg.Timeouts.Connect)},
		{"CRAWLER_TIMEOUT_TLS", durationSetter(&cfg.Timeouts.TLS)},
		{"CRAWLER_TIMEOUT_REQUEST", durationSetter(&cfg.Timeouts.Request)},
		{"CRAWLER_TIMEOUT_OVERALL", durationSetter(&cfg.Timeouts.Overall)},

		{"CRAWLER_RETRY_MAX_ATTEMPTS", intSetter(&cfg.Retry.MaxAttempts)},
		{"CRAWLER_RETRY_BASE_BACKOFF", durationSetter(&cfg.Retry.BaseBackoff)},
		{"CRAWLER_RETRY_MAX_BACKOFF", durationSetter(&cfg.Retry.MaxBackoff)},
		{"CRAWLER_RETRY_JITTER_FACTOR", floatSetter(&cfg.Retry.JitterFactor)},

		{"CRAWLER_BREAKER_FAILURE_THRESHOLD", intSetter(&cfg.Breaker.FailureThreshold)},
		{"CRAWLER_BREAKER_COOLDOWN_PERIOD", durationSetter(&cfg.Breaker.CooldownPeriod)},
		{"CRAWLER_BREAKER_HALF_OPEN_MAX_CALLS", intSetter(&cfg.Breaker.HalfOpenMaxCalls)},

		{"CRAWLER_ROBOTS_CACHE_TTL", durationSetter(&cfg.Robots.CacheTTL)},
		{"CRAWLER_ROBOTS_TIMEOUT", durationSetter(&cfg.Robots.Timeout)},

		{"CRAWLER_POSTGRES_DSN", func(v string) error { cfg.Storage.Postgres.DSN = v; return nil }},
		{"CRAWLER_POSTGRES_MAX_OPEN_CONNS", intSetter(&cfg.Storage.Postgres.MaxOpenConns)},
		{"CRAWLER_POSTGRES_MAX_IDLE_CONNS", intSetter(&cfg.Storage.Postgres.MaxIdleConns)},
		{"CRAWLER_POSTGRES_CONN_MAX_LIFETIME", durationSetter(&cfg.Storage.Postgres.ConnMaxLifetime)},

		{"CRAWLER_BLOB_STORE_BACKEND", func(v string) error { cfg.Storage.BlobStore.Backend = v; return nil }},
		{"CRAWLER_BLOB_STORE_LOCAL_DIR", func(v string) error { cfg.Storage.BlobStore.Local.Dir = v; return nil }},
		{"CRAWLER_BLOB_STORE_S3_BUCKET", func(v string) error { cfg.Storage.BlobStore.S3.Bucket = v; return nil }},
		{"CRAWLER_BLOB_STORE_S3_REGION", func(v string) error { cfg.Storage.BlobStore.S3.Region = v; return nil }},
		{"CRAWLER_BLOB_STORE_S3_PREFIX", func(v string) error { cfg.Storage.BlobStore.S3.Prefix = v; return nil }},
		{"CRAWLER_BLOB_STORE_S3_ENDPOINT", func(v string) error { cfg.Storage.BlobStore.S3.Endpoint = v; return nil }},

		{"CRAWLER_SERVER_ADDR", func(v string) error { cfg.Server.Addr = v; return nil }},

		{"CRAWLER_LOG_LEVEL", func(v string) error { cfg.Logging.Level = v; return nil }},
		{"CRAWLER_LOG_FORMAT", func(v string) error { cfg.Logging.Format = v; return nil }},
	}

	for _, o := range overrides {
		if v, ok := os.LookupEnv(o.env); ok {
			if err := o.apply(v); err != nil {
				return fmt.Errorf("%s: %w", o.env, err)
			}
		}
	}

	return nil
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func intSetter(dst *int) func(string) error {
	return func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*dst = n
		return nil
	}
}

func int64Setter(dst *int64) func(string) error {
	return func(v string) error {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return err
		}
		*dst = n
		return nil
	}
}

func floatSetter(dst *float64) func(string) error {
	return func(v string) error {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		*dst = n
		return nil
	}
}

func durationSetter(dst *time.Duration) func(string) error {
	return func(v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		*dst = d
		return nil
	}
}
