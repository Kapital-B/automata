package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
)

// NewRefreshToken returns a URL-safe random string (32 bytes raw).
func NewRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashRefreshToken returns a stable hash for DB lookup (SHA-256, base64 raw).
func HashRefreshToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

type TokenPair = domainauth.AuthTokenPair
