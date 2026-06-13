package users

import (
	"strings"
	"xmdm/server/internal/enrollment"

	"golang.org/x/crypto/bcrypt"
)

func VerifyPassword(hash, password string) bool {
	return verifyPassword(hash, password)
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	return hash == enrollment.HashToken(password)
}
