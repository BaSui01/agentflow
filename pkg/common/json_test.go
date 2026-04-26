package common

import (
	"testing"
)

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestSafeMarshal(t *testing.T) {
	data := testStruct{Name: "test", Value: 42}

	result, err := SafeMarshal(data)
	if err != nil {
		t.Errorf("SafeMarshal error: %v", err)
	}
	expected := `{"name":"test","value":42}`
	if string(result) != expected {
		t.Errorf("SafeMarshal = %s, want %s", string(result), expected)
	}
}

func TestSafeUnmarshal(t *testing.T) {
	input := `{"name":"test","value":42}`
	var result testStruct

	err := SafeUnmarshal([]byte(input), &result)
	if err != nil {
		t.Errorf("SafeUnmarshal error: %v", err)
	}
	if result.Name != "test" || result.Value != 42 {
		t.Errorf("SafeUnmarshal = %+v, want {name:test value:42}", result)
	}
}

func TestMustMarshal(t *testing.T) {
	data := testStruct{Name: "test", Value: 42}

	result := MustMarshal(data)
	expected := `{"name":"test","value":42}`
	if string(result) != expected {
		t.Errorf("MustMarshal = %s, want %s", string(result), expected)
	}
}

func TestMustMarshalPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustMarshal should panic on unmarshalable value")
		}
	}()

	// Channel cannot be marshaled to JSON
	MustMarshal(make(chan int))
}

func TestMustUnmarshal(t *testing.T) {
	input := `{"name":"test","value":42}`
	var result testStruct

	MustUnmarshal([]byte(input), &result)
	if result.Name != "test" || result.Value != 42 {
		t.Errorf("MustUnmarshal = %+v, want {name:test value:42}", result)
	}
}

func TestMustUnmarshalPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustUnmarshal should panic on invalid JSON")
		}
	}()

	var result testStruct
	MustUnmarshal([]byte("invalid json"), &result)
}

func TestJSONClone(t *testing.T) {
	original := testStruct{Name: "original", Value: 100}

	cloned, err := JSONClone(original)
	if err != nil {
		t.Errorf("JSONClone error: %v", err)
	}

	// Verify values match
	if cloned.Name != original.Name || cloned.Value != original.Value {
		t.Errorf("JSONClone values don't match")
	}

	// Verify it's a copy (modify original shouldn't affect clone)
	original.Name = "modified"
	if cloned.Name == "modified" {
		t.Error("JSONClone should return independent copy")
	}
}

func TestMustJSONClone(t *testing.T) {
	original := testStruct{Name: "original", Value: 100}

	cloned := MustJSONClone(original)
	if cloned.Name != original.Name || cloned.Value != original.Value {
		t.Errorf("MustJSONClone values don't match")
	}
}

func TestMarshalIndent(t *testing.T) {
	data := testStruct{Name: "test", Value: 42}

	result, err := MarshalIndent(data, "", "  ")
	if err != nil {
		t.Errorf("MarshalIndent error: %v", err)
	}
	expected := "{\n  \"name\": \"test\",\n  \"value\": 42\n}"
	if string(result) != expected {
		t.Errorf("MarshalIndent = %s, want %s", string(result), expected)
	}
}

func TestIsValidJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`{"key": "value"}`, true},
		{`[1, 2, 3]`, true},
		{`"string"`, true},
		{`123`, true},
		{`true`, true},
		{`null`, true},
		{`invalid`, false},
		{`{unquoted: key}`, false},
		{`[1, 2,`, false},
	}

	for _, tt := range tests {
		result := IsValidJSON([]byte(tt.input))
		if result != tt.expected {
			t.Errorf("IsValidJSON(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func BenchmarkSafeMarshal(b *testing.B) {
	data := testStruct{Name: "test", Value: 42}
	for i := 0; i < b.N; i++ {
		_, _ = SafeMarshal(data)
	}
}

func BenchmarkJSONMarshal(b *testing.B) {
	data := testStruct{Name: "test", Value: 42}
	for i := 0; i < b.N; i++ {
		_, _ = MarshalJSON(data)
	}
}

func MarshalJSON(v any) ([]byte, error) {
	return SafeMarshal(v)
}
