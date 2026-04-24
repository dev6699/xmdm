package enrollment

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

var (
	ErrTokenNotFound = errors.New("enrollment token not found")
	ErrTokenConsumed = errors.New("enrollment token already consumed")
	ErrTokenExpired  = errors.New("enrollment token expired")
	ErrTokenRevoked  = errors.New("enrollment token revoked")
	ErrTokenInvalid  = errors.New("invalid enrollment token")
	ErrTokenConflict = errors.New("enrollment token conflict")
)

func NewTokenSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func HashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
