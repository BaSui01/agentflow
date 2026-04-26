package common

import (
	"reflect"
)

// IsNil checks if a value is nil.
// This handles both nil interfaces and nil pointers/slices/maps/channels.
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// Coalesce returns the first non-zero value from the provided values.
// If all values are zero, returns the zero value of type T.
func Coalesce[T comparable](values ...T) T {
	var zero T
	for _, v := range values {
		if v != zero {
			return v
		}
	}
	return zero
}

// ValueOrDefault returns the value pointed to by ptr, or defaultValue if ptr is nil.
// Useful for optional pointer parameters.
func ValueOrDefault[T any](ptr *T, defaultValue T) T {
	if ptr == nil {
		return defaultValue
	}
	return *ptr
}

// FirstNonEmpty returns the first non-empty string from the provided strings.
// Returns empty string if all are empty.
func FirstNonEmpty(strings ...string) string {
	for _, s := range strings {
		if s != "" {
			return s
		}
	}
	return ""
}

// DefaultIfEmpty returns the value if it's not empty, otherwise returns the default.
func DefaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// Ptr returns a pointer to the given value.
// Useful for creating pointer literals for optional parameters.
func Ptr[T any](v T) *T {
	return &v
}

// Deref returns the value pointed to by ptr, or defaultValue if ptr is nil.
func Deref[T any](ptr *T, defaultValue T) T {
	if ptr == nil {
		return defaultValue
	}
	return *ptr
}
