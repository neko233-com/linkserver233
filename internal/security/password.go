package security

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Prefix     = "pbkdf2_sha256"
	pbkdf2Iterations = 210000
	pbkdf2KeyLength  = 32
	pbkdf2SaltLength = 16
)

// ErrInvalidPasswordHash indicates a stored hash is malformed.
var ErrInvalidPasswordHash = errors.New("invalid password hash")

// HashPassword derives a salted PBKDF2-HMAC-SHA256 hash for storage.
//
// The encoded form is: pbkdf2_sha256$<iterations>$<base64 salt>$<base64 key>.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is empty")
	}

	salt := make([]byte, pbkdf2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLength)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	return strings.Join([]string{
		pbkdf2Prefix,
		strconv.Itoa(pbkdf2Iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

// VerifyPassword reports whether password matches the encoded hash using a
// constant-time comparison.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != pbkdf2Prefix {
		return false, ErrInvalidPasswordHash
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}

	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}

	derived, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false, fmt.Errorf("derive key: %w", err)
	}

	return subtle.ConstantTimeCompare(derived, expected) == 1, nil
}

// ConstantTimeEqual compares two strings without leaking timing information.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
