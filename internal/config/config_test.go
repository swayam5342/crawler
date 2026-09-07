package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Concurrency.Global != Default().Concurrency.Global {
		t.Fatalf("expected defaults to be used")
	}
}

func TestLoadYAMLOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
crawl:
  max_depth: 3
  user_agent: "test-bot/1.0"
concurrency:
  global: 10
  per_domain: 1
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Crawl.MaxDepth != 3 {
		t.Errorf("MaxDepth = %d, want 3", cfg.Crawl.MaxDepth)
	}
	if cfg.Crawl.UserAgent != "test-bot/1.0" {
		t.Errorf("UserAgent = %q, want test-bot/1.0", cfg.Crawl.UserAgent)
	}
	if cfg.Concurrency.Global != 10 {
		t.Errorf("Concurrency.Global = %d, want 10", cfg.Concurrency.Global)
	}
	// Fields not set in YAML should retain defaults.
	if cfg.RateLimit.RequestsPerSecond != Default().RateLimit.RequestsPerSecond {
		t.Errorf("RateLimit.RequestsPerSecond should retain default")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("CRAWLER_MAX_DEPTH", "7")
	t.Setenv("CRAWLER_CONCURRENCY_GLOBAL", "100")
	t.Setenv("CRAWLER_RATE_LIMIT_RPS", "2.5")
	t.Setenv("CRAWLER_TIMEOUT_REQUEST", "45s")
	t.Setenv("CRAWLER_ALLOWED_DOMAINS", "example.com, foo.org")
	t.Setenv("CRAWLER_POSTGRES_DSN", "postgres://x")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Crawl.MaxDepth != 7 {
		t.Errorf("MaxDepth = %d, want 7", cfg.Crawl.MaxDepth)
	}
	if cfg.Concurrency.Global != 100 {
		t.Errorf("Concurrency.Global = %d, want 100", cfg.Concurrency.Global)
	}
	if cfg.RateLimit.RequestsPerSecond != 2.5 {
		t.Errorf("RequestsPerSecond = %v, want 2.5", cfg.RateLimit.RequestsPerSecond)
	}
	if cfg.Timeouts.Request != 45*time.Second {
		t.Errorf("Timeouts.Request = %v, want 45s", cfg.Timeouts.Request)
	}
	if len(cfg.Crawl.AllowedDomains) != 2 || cfg.Crawl.AllowedDomains[0] != "example.com" || cfg.Crawl.AllowedDomains[1] != "foo.org" {
		t.Errorf("AllowedDomains = %v, want [example.com foo.org]", cfg.Crawl.AllowedDomains)
	}
	if cfg.Storage.Postgres.DSN != "postgres://x" {
		t.Errorf("Postgres.DSN = %q, want postgres://x", cfg.Storage.Postgres.DSN)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid default", func(c *Config) {}, false},
		{"zero global concurrency", func(c *Config) { c.Concurrency.Global = 0 }, true},
		{"per-domain exceeds global", func(c *Config) { c.Concurrency.PerDomain = c.Concurrency.Global + 1 }, true},
		{"zero rps", func(c *Config) { c.RateLimit.RequestsPerSecond = 0 }, true},
		{"negative max depth", func(c *Config) { c.Crawl.MaxDepth = -1 }, true},
		{"empty user agent", func(c *Config) { c.Crawl.UserAgent = "" }, true},
		{"max backoff below base", func(c *Config) { c.Retry.MaxBackoff = c.Retry.BaseBackoff - time.Millisecond }, true},
		{"jitter out of range", func(c *Config) { c.Retry.JitterFactor = 1.5 }, true},
		{"empty postgres dsn", func(c *Config) { c.Storage.Postgres.DSN = "" }, true},
		{"unknown blob backend", func(c *Config) { c.Storage.BlobStore.Backend = "ftp" }, true},
		{"s3 backend missing bucket", func(c *Config) {
			c.Storage.BlobStore.Backend = "s3"
			c.Storage.BlobStore.S3.Bucket = ""
		}, true},
		{"s3 backend with bucket", func(c *Config) {
			c.Storage.BlobStore.Backend = "s3"
			c.Storage.BlobStore.S3.Bucket = "my-bucket"
		}, false},
		{"bad log level", func(c *Config) { c.Logging.Level = "verbose" }, true},
		{"bad log format", func(c *Config) { c.Logging.Format = "xml" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
