package users

import (
	"testing"

	"xmdm/server/internal/enrollment"
)

func TestHashPasswordUsesSalt(t *testing.T) {
	first, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	second, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if first == second {
		t.Fatalf("expected salted hashes to differ")
	}
	if !VerifyPassword(first, "s3cret") || !VerifyPassword(second, "s3cret") {
		t.Fatalf("expected hashed passwords to verify")
	}
}

func TestVerifyPasswordAcceptsLegacyHash(t *testing.T) {
	legacy := enrollment.HashToken("s3cret")
	if !VerifyPassword(legacy, "s3cret") {
		t.Fatalf("expected legacy hash to verify")
	}
}
