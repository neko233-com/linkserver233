package link

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	day  = 24 * time.Hour
	week = 7 * day
)

// ParseFlexibleDuration parses a human friendly duration string.
//
// It extends time.ParseDuration with day ("d") and week ("w") units so that
// link TTLs can be written as "30d", "2w", "1d12h", or "90m".
func ParseFlexibleDuration(raw string) (time.Duration, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 0, errors.New("duration is empty")
	}
	if strings.HasPrefix(value, "-") {
		return 0, errors.New("duration cannot be negative")
	}

	var total time.Duration
	var number strings.Builder
	sawUnit := false

	for _, r := range value {
		if (r >= '0' && r <= '9') || r == '.' {
			number.WriteRune(r)
			continue
		}

		if number.Len() == 0 {
			return 0, fmt.Errorf("invalid duration %q", raw)
		}

		amount, err := strconv.ParseFloat(number.String(), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
		}
		number.Reset()

		switch r {
		case 'w':
			total += time.Duration(amount * float64(week))
		case 'd':
			total += time.Duration(amount * float64(day))
		case 'h':
			total += time.Duration(amount * float64(time.Hour))
		case 'm':
			total += time.Duration(amount * float64(time.Minute))
		case 's':
			total += time.Duration(amount * float64(time.Second))
		default:
			return 0, fmt.Errorf("invalid duration unit %q in %q", string(r), raw)
		}
		sawUnit = true
	}

	if number.Len() != 0 {
		return 0, fmt.Errorf("invalid duration %q: missing unit", raw)
	}
	if !sawUnit {
		return 0, fmt.Errorf("invalid duration %q", raw)
	}

	return total, nil
}
