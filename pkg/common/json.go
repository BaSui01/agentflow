package common

import (
	"encoding/json"
)

// SafeMarshal marshals a value to JSON, returning the bytes and error.
// This is a convenience wrapper around json.Marshal with consistent error handling.
func SafeMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// SafeUnmarshal unmarshals JSON bytes into the target, returning any error.
// This is a convenience wrapper around json.Unmarshal with consistent error handling.
func SafeUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

// MustMarshal marshals a value to JSON, panicking on error.
// Use only in tests or when the value is guaranteed to be marshalable.
func MustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// MustUnmarshal unmarshals JSON bytes into the target, panicking on error.
// Use only in tests or when the input is guaranteed to be valid JSON.
func MustUnmarshal(data []byte, target any) {
	if err := json.Unmarshal(data, target); err != nil {
		panic(err)
	}
}

// JSONClone creates a deep copy of a value by marshaling and unmarshaling.
// Returns an error if marshaling or unmarshaling fails.
func JSONClone[T any](v T) (T, error) {
	var result T
	data, err := json.Marshal(v)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

// MustJSONClone creates a deep copy of a value, panicking on error.
func MustJSONClone[T any](v T) T {
	result, err := JSONClone(v)
	if err != nil {
		panic(err)
	}
	return result
}

// MarshalIndent marshals a value to indented JSON.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// IsValidJSON checks if a byte slice is valid JSON.
func IsValidJSON(data []byte) bool {
	return json.Valid(data)
}
