package link

import (
	"testing"
	"time"
)

func TestParseFlexibleDuration(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"90m", 90 * time.Minute},
		{"24h", 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"45s", 45 * time.Second},
	}

	for _, tc := range cases {
		got, err := ParseFlexibleDuration(tc.input)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %s, want %s", tc.input, got, tc.want)
		}
	}
}

func TestParseFlexibleDurationErrors(t *testing.T) {
	for _, input := range []string{"", "abc", "10", "-5h", "5y", "1.2.3h"} {
		if _, err := ParseFlexibleDuration(input); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}
}
