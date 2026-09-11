package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	// TokenPrefix marks every API token so leaks are easy to grep for.
	TokenPrefix = "jbt_"
	tokenLen    = 40
	// DisplayLen is how many leading characters are stored for identification.
	DisplayLen = 12

	ScopeRead  = "read"
	ScopeWrite = "write"
)

const tokenAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateToken returns a fresh plaintext token. Only its hash is persisted.
func GenerateToken() string {
	return TokenPrefix + randomString(tokenAlphabet, tokenLen)
}

// HashToken derives the storage key for a token. Tokens carry ~238 bits of
// entropy, so an unsalted SHA-256 is sufficient.
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// DisplayPrefix is the part of a token safe to show in listings.
func DisplayPrefix(plain string) string {
	if len(plain) <= DisplayLen {
		return plain
	}
	return plain[:DisplayLen]
}

// LooksLikeToken is a cheap pre-check before hitting the database.
func LooksLikeToken(s string) bool {
	return strings.HasPrefix(s, TokenPrefix) && len(s) == len(TokenPrefix)+tokenLen
}

func ValidScope(s string) bool { return s == ScopeRead || s == ScopeWrite }
