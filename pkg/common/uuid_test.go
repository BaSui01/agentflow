package common

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewUUID(t *testing.T) {
	id1 := NewUUID()
	id2 := NewUUID()

	// Should be different
	if id1 == id2 {
		t.Error("NewUUID should return unique values")
	}

	// Should be valid UUID format (36 chars with hyphens)
	if len(id1) != 36 {
		t.Errorf("NewUUID length = %d, want 36", len(id1))
	}
}

func TestNewUUIDBytes(t *testing.T) {
	id := NewUUIDBytes()

	// UUID is 16 bytes
	if len(id) != 16 {
		t.Errorf("NewUUIDBytes length = %d, want 16", len(id))
	}
}

func TestNewPrefixedUUID(t *testing.T) {
	prefix := "test_"
	result := NewPrefixedUUID(prefix)

	if !strings.HasPrefix(result, prefix) {
		t.Errorf("NewPrefixedUUID should start with %q, got %q", prefix, result)
	}

	// Total length: prefix + 36 (UUID)
	expectedLen := len(prefix) + 36
	if len(result) != expectedLen {
		t.Errorf("NewPrefixedUUID length = %d, want %d", len(result), expectedLen)
	}

	// Verify the UUID part is valid
	uuidPart := result[len(prefix):]
	if !IsValidUUID(uuidPart) {
		t.Errorf("UUID part %q is not valid", uuidPart)
	}
}

func TestNewShortPrefixedUUID(t *testing.T) {
	prefix := "exec_"
	result := NewShortPrefixedUUID(prefix)

	if !strings.HasPrefix(result, prefix) {
		t.Errorf("NewShortPrefixedUUID should start with %q, got %q", prefix, result)
	}

	// Total length: prefix + 12
	expectedLen := len(prefix) + 12
	if len(result) != expectedLen {
		t.Errorf("NewShortPrefixedUUID length = %d, want %d", len(result), expectedLen)
	}
}

func TestNewExecutionID(t *testing.T) {
	id := NewExecutionID()

	if !strings.HasPrefix(id, "exec_") {
		t.Errorf("NewExecutionID should start with 'exec_', got %q", id)
	}

	// Should be unique
	id2 := NewExecutionID()
	if id == id2 {
		t.Error("NewExecutionID should return unique values")
	}
}

func TestNewRunID(t *testing.T) {
	id := NewRunID()

	if !strings.HasPrefix(id, "run_") {
		t.Errorf("NewRunID should start with 'run_', got %q", id)
	}
}

func TestNewSpanID(t *testing.T) {
	id := NewSpanID()

	if !strings.HasPrefix(id, "span_") {
		t.Errorf("NewSpanID should start with 'span_', got %q", id)
	}
}

func TestNewPlanID(t *testing.T) {
	id := NewPlanID()

	if !strings.HasPrefix(id, "plan_") {
		t.Errorf("NewPlanID should start with 'plan_', got %q", id)
	}
}

func TestNewTaskID(t *testing.T) {
	id := NewTaskID()

	if !IsValidUUID(id) {
		t.Errorf("NewTaskID should return valid UUID, got %q", id)
	}
}

func TestNewMessageID(t *testing.T) {
	id := NewMessageID()

	if !IsValidUUID(id) {
		t.Errorf("NewMessageID should return valid UUID, got %q", id)
	}
}

func TestParseUUID(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	result, err := ParseUUID(validUUID)
	if err != nil {
		t.Errorf("ParseUUID unexpected error: %v", err)
	}
	if result.String() != validUUID {
		t.Errorf("ParseUUID = %q, want %q", result.String(), validUUID)
	}

	// Invalid UUID
	_, err = ParseUUID("invalid")
	if err == nil {
		t.Error("ParseUUID should return error for invalid UUID")
	}
}

func TestMustParseUUID(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	result := MustParseUUID(validUUID)
	if result.String() != validUUID {
		t.Errorf("MustParseUUID = %q, want %q", result.String(), validUUID)
	}
}

func TestMustParseUUIDPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseUUID should panic for invalid UUID")
		}
	}()

	MustParseUUID("invalid")
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"550e8400e29b41d4a716446655440000", true}, // without hyphens
		{"invalid", false},
		{"", false},
		{"550e8400-e29b-41d4-a716", false}, // incomplete
	}

	for _, tt := range tests {
		result := IsValidUUID(tt.input)
		if result != tt.expected {
			t.Errorf("IsValidUUID(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func BenchmarkNewUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewUUID()
	}
}

func BenchmarkNewUUIDOriginal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = uuid.New().String()
	}
}

func BenchmarkNewExecutionID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewExecutionID()
	}
}

func BenchmarkNewExecutionIDOriginal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = "exec_" + uuid.New().String()[:12]
	}
}
