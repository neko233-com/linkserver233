package security

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail")
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	if _, err := VerifyPassword("not-a-valid-hash", "x"); err != ErrInvalidPasswordHash {
		t.Fatalf("expected ErrInvalidPasswordHash, got %v", err)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("token", "token") {
		t.Fatal("expected equal tokens to match")
	}
	if ConstantTimeEqual("token", "other") {
		t.Fatal("expected different tokens to differ")
	}
}
