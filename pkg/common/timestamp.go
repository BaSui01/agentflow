// Package common provides reusable utility functions for the AgentFlow project.
// These functions eliminate code duplication and ensure consistent behavior across the codebase.
package common

import (
	"time"
)

// TimestampNow returns the current UTC time.
// This is a convenience function that wraps time.Now().UTC() for consistent usage.
func TimestampNow() time.Time {
	return time.Now().UTC()
}

// TimestampNowFormatted returns the current UTC time formatted as RFC3339.
// Common format used in API responses and logging.
func TimestampNowFormatted() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// FormatTimestamp formats a time.Time as RFC3339 string in UTC.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FormatTimestampCustom formats a time.Time with a custom layout in UTC.
func FormatTimestampCustom(t time.Time, layout string) string {
	return t.UTC().Format(layout)
}

// ParseTimestamp parses an RFC3339 timestamp string into time.Time.
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// TimestampOrNil returns a pointer to the time, or nil if zero.
// Useful for optional timestamp fields in API responses.
func TimestampOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
