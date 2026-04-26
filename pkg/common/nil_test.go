package common

import (
	"testing"
)

func TestIsNil(t *testing.T) {
	// Test nil interface
	if !IsNil(nil) {
		t.Error("IsNil(nil) should return true")
	}

	// Test nil pointer
	var ptr *int
	if !IsNil(ptr) {
		t.Error("IsNil(nil pointer) should return true")
	}

	// Test nil slice
	var slice []int
	if !IsNil(slice) {
		t.Error("IsNil(nil slice) should return true")
	}

	// Test nil map
	var m map[string]int
	if !IsNil(m) {
		t.Error("IsNil(nil map) should return true")
	}

	// Test nil channel
	var ch chan int
	if !IsNil(ch) {
		t.Error("IsNil(nil channel) should return true")
	}

	// Test non-nil values
	if IsNil(42) {
		t.Error("IsNil(42) should return false")
	}

	if IsNil("hello") {
		t.Error("IsNil(string) should return false")
	}

	// Test non-nil pointer
	x := 42
	if IsNil(&x) {
		t.Error("IsNil(non-nil pointer) should return false")
	}
}

func TestCoalesce(t *testing.T) {
	// Test with first non-zero
	result := Coalesce(0, 1, 2)
	if result != 1 {
		t.Errorf("Coalesce(0, 1, 2) = %d, want 1", result)
	}

	// Test with all zero
	result = Coalesce(0, 0, 0)
	if result != 0 {
		t.Errorf("Coalesce(0, 0, 0) = %d, want 0", result)
	}

	// Test with strings
	strResult := Coalesce("", "hello", "world")
	if strResult != "hello" {
		t.Errorf("Coalesce strings = %q, want %q", strResult, "hello")
	}
}

func TestValueOrDefault(t *testing.T) {
	// Test with nil pointer
	var ptr *int
	result := ValueOrDefault(ptr, 42)
	if result != 42 {
		t.Errorf("ValueOrDefault(nil, 42) = %v, want 42", result)
	}

	// Test with non-nil pointer
	x := 100
	result = ValueOrDefault(&x, 42)
	if result != 100 {
		t.Errorf("ValueOrDefault(&x, 42) = %v, want 100", result)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	result := FirstNonEmpty("", "hello", "world")
	if result != "hello" {
		t.Errorf("FirstNonEmpty = %q, want %q", result, "hello")
	}

	result = FirstNonEmpty("", "", "")
	if result != "" {
		t.Errorf("FirstNonEmpty(all empty) = %q, want empty", result)
	}
}

func TestDefaultIfEmpty(t *testing.T) {
	result := DefaultIfEmpty("", "default")
	if result != "default" {
		t.Errorf("DefaultIfEmpty = %q, want %q", result, "default")
	}

	result = DefaultIfEmpty("value", "default")
	if result != "value" {
		t.Errorf("DefaultIfEmpty = %q, want %q", result, "value")
	}
}

func TestPtr(t *testing.T) {
	ptr := Ptr(42)
	if ptr == nil {
		t.Error("Ptr should not return nil")
	}
	if *ptr != 42 {
		t.Errorf("Ptr(42) = %d, want 42", *ptr)
	}
}

func TestDeref(t *testing.T) {
	// Test with nil pointer
	result := Deref(nil, 42)
	if result != 42 {
		t.Errorf("Deref(nil, 42) = %v, want 42", result)
	}

	// Test with non-nil pointer
	x := 100
	result = Deref(&x, 42)
	if result != 100 {
		t.Errorf("Deref(&x, 42) = %v, want 100", result)
	}
}
