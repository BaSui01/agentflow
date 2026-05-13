// Package cryptoutil provides cryptographic utility functions.
package cryptoutil

import (
	"crypto/sha256"
	"crypto/subtle"
)

// SecureTokenEqual performs a constant-time comparison of two token strings
// using SHA-256 hashing to prevent timing attacks.
func SecureTokenEqual(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}
