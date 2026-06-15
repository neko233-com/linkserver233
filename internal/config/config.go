package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/neko233-com/linkserver233/internal/link"
)

type ServeConfig struct {
	Addr            string
	DataPath        string
	BaseURL         string
	AdminToken      string
	ShortCodeLength int

	AllowPrivateTargets bool
	DefaultTTL          time.Duration
	MaxTTL              time.Duration
	RequireExpiry       bool

	RateLimitPerMinute float64
	RateLimitBurst     float64

	JanitorInterval time.Duration

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func ParseServeArgs(args []string) (ServeConfig, error) {
	cfg := ServeConfig{
		Addr:            envOrDefault("LINKSERVER_ADDR", ":8080"),
		DataPath:        envOrDefault("LINKSERVER_DATA", "data/links.json"),
		BaseURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("LINKSERVER_BASE_URL")), "/"),
		AdminToken:      os.Getenv("LINKSERVER_ADMIN_TOKEN"),
		ShortCodeLength: envIntOrDefault("LINKSERVER_CODE_LENGTH", 7),

		AllowPrivateTargets: envBoolOrDefault("LINKSERVER_ALLOW_PRIVATE_TARGETS", false),
		RequireExpiry:       envBoolOrDefault("LINKSERVER_REQUIRE_EXPIRY", false),

		RateLimitPerMinute: envFloatOrDefault("LINKSERVER_RATE_LIMIT_PER_MIN", 120),
		RateLimitBurst:     envFloatOrDefault("LINKSERVER_RATE_LIMIT_BURST", 60),

		JanitorInterval: envDurationOrDefault("LINKSERVER_JANITOR_INTERVAL", 5*time.Minute),

		ReadTimeout:  envDurationOrDefault("LINKSERVER_READ_TIMEOUT", 5*time.Second),
		WriteTimeout: envDurationOrDefault("LINKSERVER_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:  envDurationOrDefault("LINKSERVER_IDLE_TIMEOUT", 60*time.Second),
	}

	defaultTTLRaw := envOrDefault("LINKSERVER_DEFAULT_TTL", "30d")
	maxTTLRaw := envOrDefault("LINKSERVER_MAX_TTL", "0")

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DataPath, "data", cfg.DataPath, "JSON data file path")
	fs.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "public base URL used in API responses")
	fs.StringVar(&cfg.AdminToken, "admin-token", cfg.AdminToken, "optional Bearer token required for API access")
	fs.IntVar(&cfg.ShortCodeLength, "code-length", cfg.ShortCodeLength, "generated short-code length")
	fs.BoolVar(&cfg.AllowPrivateTargets, "allow-private-targets", cfg.AllowPrivateTargets, "allow redirect targets pointing to private/internal addresses")
	fs.StringVar(&defaultTTLRaw, "default-ttl", defaultTTLRaw, "default link lifetime, e.g. 30d/12h (0 to disable)")
	fs.StringVar(&maxTTLRaw, "max-ttl", maxTTLRaw, "maximum allowed link lifetime (0 for unlimited)")
	fs.BoolVar(&cfg.RequireExpiry, "require-expiry", cfg.RequireExpiry, "reject links that would never expire")
	fs.Float64Var(&cfg.RateLimitPerMinute, "rate-limit", cfg.RateLimitPerMinute, "per-client requests per minute (0 to disable)")
	fs.Float64Var(&cfg.RateLimitBurst, "rate-limit-burst", cfg.RateLimitBurst, "per-client burst allowance")
	fs.DurationVar(&cfg.JanitorInterval, "janitor-interval", cfg.JanitorInterval, "interval for purging expired links (0 to disable)")
	fs.DurationVar(&cfg.ReadTimeout, "read-timeout", cfg.ReadTimeout, "HTTP read timeout")
	fs.DurationVar(&cfg.WriteTimeout, "write-timeout", cfg.WriteTimeout, "HTTP write timeout")
	fs.DurationVar(&cfg.IdleTimeout, "idle-timeout", cfg.IdleTimeout, "HTTP idle timeout")

	if err := fs.Parse(args); err != nil {
		return ServeConfig{}, err
	}

	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.DataPath = strings.TrimSpace(cfg.DataPath)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	defaultTTL, err := parseTTL(defaultTTLRaw)
	if err != nil {
		return ServeConfig{}, fmt.Errorf("invalid default-ttl: %w", err)
	}
	cfg.DefaultTTL = defaultTTL

	maxTTL, err := parseTTL(maxTTLRaw)
	if err != nil {
		return ServeConfig{}, fmt.Errorf("invalid max-ttl: %w", err)
	}
	cfg.MaxTTL = maxTTL

	if cfg.Addr == "" {
		return ServeConfig{}, fmt.Errorf("addr cannot be empty")
	}
	if cfg.DataPath == "" {
		return ServeConfig{}, fmt.Errorf("data path cannot be empty")
	}
	if cfg.ShortCodeLength < 4 || cfg.ShortCodeLength > 32 {
		return ServeConfig{}, fmt.Errorf("code length must be between 4 and 32")
	}
	if cfg.MaxTTL > 0 && cfg.DefaultTTL > cfg.MaxTTL {
		return ServeConfig{}, fmt.Errorf("default-ttl cannot exceed max-ttl")
	}
	if cfg.RequireExpiry && cfg.DefaultTTL == 0 && cfg.MaxTTL == 0 {
		return ServeConfig{}, fmt.Errorf("require-expiry needs a default-ttl or max-ttl")
	}
	if cfg.RateLimitPerMinute < 0 {
		return ServeConfig{}, fmt.Errorf("rate-limit cannot be negative")
	}

	return cfg, nil
}

func parseTTL(raw string) (time.Duration, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "0", "none", "never", "off", "unlimited":
		return 0, nil
	}
	return link.ParseFlexibleDuration(value)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloatOrDefault(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
