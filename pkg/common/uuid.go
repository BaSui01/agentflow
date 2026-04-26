package common

import (
	"github.com/google/uuid"
)

// NewUUID returns a new random UUID as a string.
// This is a convenience wrapper around uuid.New().String().
func NewUUID() string {
	return uuid.New().String()
}

// NewUUIDBytes returns a new random UUID as a byte slice.
func NewUUIDBytes() []byte {
	id := uuid.New()
	return id[:]
}

// NewPrefixedUUID returns a new UUID with the given prefix.
// Example: NewPrefixedUUID("exec_") returns "exec_550e8400-e29b-41d4-a716-446655440000".
func NewPrefixedUUID(prefix string) string {
	return prefix + uuid.New().String()
}

// NewShortPrefixedUUID returns a new UUID with prefix and truncated to first 12 chars.
// Example: NewShortPrefixedUUID("exec_") returns "exec_550e8400-e29".
func NewShortPrefixedUUID(prefix string) string {
	return prefix + uuid.New().String()[:12]
}

// NewExecutionID returns a new execution ID with "exec_" prefix.
// Format: "exec_" + 12-char UUID (e.g., "exec_550e8400-e29").
func NewExecutionID() string {
	return NewShortPrefixedUUID("exec_")
}

// NewRunID returns a new run ID with "run_" prefix.
// Format: "run_" + full UUID.
func NewRunID() string {
	return NewPrefixedUUID("run_")
}

// NewSpanID returns a new span ID with "span_" prefix.
// Format: "span_" + full UUID.
func NewSpanID() string {
	return NewPrefixedUUID("span_")
}

// NewPlanID returns a new plan ID with "plan_" prefix.
// Format: "plan_" + full UUID.
func NewPlanID() string {
	return NewPrefixedUUID("plan_")
}

// NewTaskID returns a new task ID.
// Format: full UUID string.
func NewTaskID() string {
	return NewUUID()
}

// NewMessageID returns a new message ID.
// Format: full UUID string.
func NewMessageID() string {
	return NewUUID()
}

// ParseUUID parses a UUID string and returns the UUID or an error.
func ParseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// MustParseUUID parses a UUID string and panics on error.
// Use only in tests or when the input is guaranteed to be valid.
func MustParseUUID(s string) uuid.UUID {
	return uuid.MustParse(s)
}

// IsValidUUID checks if a string is a valid UUID.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
