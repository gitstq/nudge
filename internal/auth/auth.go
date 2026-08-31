// Package auth provides token generation, one-way hashing and the principal
// model used by the HTTP layer. Raw tokens are shown to the operator exactly
// once; only SHA-256 hashes are persisted.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateToken returns a 32-byte URL-safe token with the given prefix,
// e.g. "nudg_k_...". Prefixes help operators spot tokens in shell history.
func GenerateToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s", prefix, base64.RawURLEncoding.EncodeToString(b)), nil
}

// HashToken returns the hex SHA-256 of a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Prefix returns a safe, non-revealing display fragment (first 12 chars).
func Prefix(token string) string {
	if len(token) > 12 {
		return token[:12] + "…"
	}
	return token
}

// EqualHash reports whether token hashes to hash, in constant time.
func EqualHash(token, hash string) bool {
	got := HashToken(token)
	return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1
}
