package config

import (
	"testing"
	"time"
)

func TestParseServeArgsDefaults(t *testing.T) {
	t.Setenv("LINKSERVER_ADDR", "")
	t.Setenv("LINKSERVER_DATA", "")
	t.Setenv("LINKSERVER_DEFAULT_TTL", "")

	cfg, err := ParseServeArgs(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("expected default addr, got %q", cfg.Addr)
	}
	if cfg.DefaultTTL != 30*24*time.Hour {
		t.Fatalf("expected 30d default ttl, got %s", cfg.DefaultTTL)
	}
	if cfg.RateLimitPerMinute != 120 {
		t.Fatalf("expected default rate limit, got %v", cfg.RateLimitPerMinute)
	}
}

func TestParseServeArgsFlags(t *testing.T) {
	cfg, err := ParseServeArgs([]string{
		"--addr", ":9090",
		"--default-ttl", "7d",
		"--max-ttl", "30d",
		"--rate-limit", "60",
		"--allow-private-targets",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Fatalf("addr: got %q", cfg.Addr)
	}
	if cfg.DefaultTTL != 7*24*time.Hour {
		t.Fatalf("default ttl: got %s", cfg.DefaultTTL)
	}
	if cfg.MaxTTL != 30*24*time.Hour {
		t.Fatalf("max ttl: got %s", cfg.MaxTTL)
	}
	if !cfg.AllowPrivateTargets {
		t.Fatal("expected allow-private-targets to be true")
	}
}

func TestParseServeArgsValidation(t *testing.T) {
	if _, err := ParseServeArgs([]string{"--default-ttl", "60d", "--max-ttl", "30d"}); err == nil {
		t.Fatal("expected default-ttl > max-ttl to fail")
	}
	if _, err := ParseServeArgs([]string{"--code-length", "2"}); err == nil {
		t.Fatal("expected code-length validation to fail")
	}
	if _, err := ParseServeArgs([]string{"--rate-limit", "-1"}); err == nil {
		t.Fatal("expected negative rate limit to fail")
	}
	if _, err := ParseServeArgs([]string{"--default-ttl", "0", "--max-ttl", "0", "--require-expiry"}); err == nil {
		t.Fatal("expected require-expiry without ttl to fail")
	}
}

func TestParseTTL(t *testing.T) {
	for _, value := range []string{"0", "", "none", "never", "off"} {
		got, err := parseTTL(value)
		if err != nil {
			t.Fatalf("parseTTL(%q): %v", value, err)
		}
		if got != 0 {
			t.Fatalf("parseTTL(%q): expected 0, got %s", value, got)
		}
	}
	if got, err := parseTTL("12h"); err != nil || got != 12*time.Hour {
		t.Fatalf("parseTTL(12h): got %s err %v", got, err)
	}
}
