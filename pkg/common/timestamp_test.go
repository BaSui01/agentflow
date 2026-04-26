package common

import (
	"testing"
	"time"
)

func TestTimestampNow(t *testing.T) {
	before := time.Now().UTC()
	result := TimestampNow()
	after := time.Now().UTC()

	if result.Before(before) {
		t.Error("TimestampNow returned time before call")
	}
	if result.After(after) {
		t.Error("TimestampNow returned time after call")
	}

	// Verify UTC timezone
	if result.Location() != time.UTC {
		t.Errorf("TimestampNow should return UTC time, got %v", result.Location())
	}
}

func TestTimestampNowFormatted(t *testing.T) {
	result := TimestampNowFormatted()

	// Verify it can be parsed back
	parsed, err := time.Parse(time.RFC3339, result)
	if err != nil {
		t.Errorf("TimestampNowFormatted returned invalid RFC3339 format: %v", err)
	}

	// Verify UTC
	if parsed.Location() != time.UTC {
		t.Errorf("Parsed timestamp should be UTC, got %v", parsed.Location())
	}
}

func TestFormatTimestamp(t *testing.T) {
	// Test with known time
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := FormatTimestamp(testTime)

	expected := "2024-01-15T10:30:00Z"
	if result != expected {
		t.Errorf("FormatTimestamp() = %q, want %q", result, expected)
	}
}

func TestFormatTimestampCustom(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		layout   string
		expected string
	}{
		{"2006-01-02", "2024-01-15"},
		{"15:04:05", "10:30:00"},
		{"2006/01/02 15:04:05", "2024/01/15 10:30:00"},
	}

	for _, tt := range tests {
		result := FormatTimestampCustom(testTime, tt.layout)
		if result != tt.expected {
			t.Errorf("FormatTimestampCustom(%q) = %q, want %q", tt.layout, result, tt.expected)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		wantErr  bool
		expected time.Time
	}{
		{"2024-01-15T10:30:00Z", false, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"2024-01-15T10:30:00+08:00", false, time.Date(2024, 1, 15, 2, 30, 0, 0, time.UTC)},
		{"invalid", true, time.Time{}},
	}

	for _, tt := range tests {
		result, err := ParseTimestamp(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseTimestamp(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseTimestamp(%q) unexpected error: %v", tt.input, err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("ParseTimestamp(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		}
	}
}

func TestTimestampOrNil(t *testing.T) {
	// Test with non-zero time
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := TimestampOrNil(testTime)
	if result == nil {
		t.Error("TimestampOrNil should return non-nil for non-zero time")
	}
	if !result.Equal(testTime) {
		t.Errorf("TimestampOrNil returned wrong value: %v, want %v", *result, testTime)
	}

	// Test with zero time
	zeroTime := time.Time{}
	result = TimestampOrNil(zeroTime)
	if result != nil {
		t.Error("TimestampOrNil should return nil for zero time")
	}
}

func BenchmarkTimestampNow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = TimestampNow()
	}
}

func BenchmarkTimestampNowOriginal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = time.Now().UTC()
	}
}

func BenchmarkTimestampNowFormatted(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = TimestampNowFormatted()
	}
}

func BenchmarkTimestampNowFormattedOriginal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = time.Now().UTC().Format(time.RFC3339)
	}
}
