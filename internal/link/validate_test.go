package link

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "simple", input: "docs", want: "docs"},
		{name: "multi segment", input: "/docs/latest/", want: "docs/latest"},
		{name: "trim slashes", input: "//a/b//", want: "a/b"},
		{name: "empty", input: "  ", wantErr: true},
		{name: "traversal", input: "../etc/passwd", wantErr: true},
		{name: "backslash", input: "a\\b", wantErr: true},
		{name: "reserved api", input: "api/links", wantErr: true},
		{name: "reserved healthz", input: "healthz", wantErr: true},
		{name: "bad char", input: "a b", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeTargetURL(t *testing.T) {
	if _, err := NormalizeTargetURL("https://example.com/path", false); err != nil {
		t.Fatalf("expected valid URL, got %v", err)
	}
	if _, err := NormalizeTargetURL("ftp://example.com", false); err == nil {
		t.Fatal("expected scheme rejection")
	}
	if _, err := NormalizeTargetURL("http://127.0.0.1/admin", false); err == nil {
		t.Fatal("expected loopback rejection")
	}
	if _, err := NormalizeTargetURL("http://127.0.0.1/admin", true); err != nil {
		t.Fatalf("expected loopback allowed with allowPrivate: %v", err)
	}
}

func TestNormalizeRedirectStatus(t *testing.T) {
	if got, _ := NormalizeRedirectStatus(0); got != DefaultRedirectStatus {
		t.Fatalf("expected default %d, got %d", DefaultRedirectStatus, got)
	}
	if _, err := NormalizeRedirectStatus(418); err == nil {
		t.Fatal("expected unsupported status error")
	}
	for _, status := range []int{301, 302, 307, 308} {
		if got, err := NormalizeRedirectStatus(status); err != nil || got != status {
			t.Fatalf("status %d: got %d err %v", status, got, err)
		}
	}
}

func TestGenerateShortCode(t *testing.T) {
	const alphabet = "23456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	code, err := GenerateShortCode(8)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(code) != 8 {
		t.Fatalf("expected length 8, got %d", len(code))
	}
	for _, r := range code {
		if !strings.ContainsRune(alphabet, r) {
			t.Fatalf("unexpected character %q", r)
		}
	}
	if _, err := GenerateShortCode(2); err == nil {
		t.Fatal("expected length validation error")
	}
}

func TestRecordStatus(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	active := Record{Enabled: true}
	if active.Status(now) != StatusActive {
		t.Fatalf("expected active")
	}

	disabled := Record{Enabled: false}
	if disabled.Status(now) != StatusDisabled {
		t.Fatalf("expected disabled")
	}

	past := now.Add(-time.Hour)
	expired := Record{Enabled: true, ExpiresAt: &past}
	if expired.Status(now) != StatusExpired {
		t.Fatalf("expected expired")
	}

	exhausted := Record{Enabled: true, MaxClicks: 1, Clicks: 1}
	if exhausted.Status(now) != StatusExhausted {
		t.Fatalf("expected exhausted")
	}
	if remaining := exhausted.RemainingClicks(); remaining == nil || *remaining != 0 {
		t.Fatalf("expected 0 remaining clicks")
	}
}
