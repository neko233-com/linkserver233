package link

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/neko233-com/linkserver233/internal/security"
)

const DefaultRedirectStatus = 302

var reservedRoots = []string{"api", "healthz", "agent", "llms.txt", "favicon.ico", "robots.txt"}

func NormalizePath(raw string) (string, error) {
	return normalizePath(raw, true)
}

func NormalizeLookupPath(raw string) (string, error) {
	return normalizePath(raw, false)
}

// NormalizeTargetURL validates a redirect target and rejects internal hosts
// unless allowPrivate is set.
func NormalizeTargetURL(raw string, allowPrivate bool) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", errors.New("target_url is required")
	}
	return security.ValidateTargetURL(target, allowPrivate)
}

func NormalizeRedirectStatus(value int) (int, error) {
	if value == 0 {
		return DefaultRedirectStatus, nil
	}

	switch value {
	case 301, 302, 307, 308:
		return value, nil
	default:
		return 0, fmt.Errorf("redirect_status %d is not supported", value)
	}
}

func GenerateShortCode(length int) (string, error) {
	if length < 4 {
		return "", errors.New("short code length must be at least 4")
	}

	const alphabet = "23456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate short code: %w", err)
	}

	code := make([]byte, length)
	for i, value := range randomBytes {
		code[i] = alphabet[int(value)%len(alphabet)]
	}
	return string(code), nil
}

func IsReservedPath(value string) bool {
	for _, root := range reservedRoots {
		if value == root || strings.HasPrefix(value, root+"/") {
			return true
		}
	}
	return false
}

func normalizePath(raw string, rejectReserved bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", errors.New("path is required")
	}
	if strings.Contains(trimmed, "\\") {
		return "", errors.New("path cannot contain backslashes")
	}
	if strings.Contains(trimmed, "..") {
		return "", errors.New("path cannot contain '..'")
	}

	cleaned := strings.TrimPrefix(path.Clean("/"+trimmed), "/")
	if cleaned == "" || cleaned == "." {
		return "", errors.New("path is required")
	}

	segments := strings.Split(cleaned, "/")
	for _, segment := range segments {
		if err := validateSegment(segment); err != nil {
			return "", err
		}
	}

	if rejectReserved && IsReservedPath(cleaned) {
		return "", fmt.Errorf("path %q is reserved", cleaned)
	}

	return cleaned, nil
}

func validateSegment(segment string) error {
	if segment == "" {
		return errors.New("path contains an empty segment")
	}

	for _, r := range segment {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '_', '.', '~':
			continue
		default:
			return fmt.Errorf("path segment %q contains unsupported character %q", segment, r)
		}
	}

	return nil
}
