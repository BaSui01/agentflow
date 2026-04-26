package bootstrap

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// workflowRawFingerprint returns a SHA-256 hex fingerprint for raw JSON data.
func workflowRawFingerprint(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

// workflowStringFingerprint returns a SHA-256 hex fingerprint for a string value.
func workflowStringFingerprint(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}
