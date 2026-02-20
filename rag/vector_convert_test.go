package rag

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestFloat32ToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected []float64
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []float32{},
			expected: []float64{},
		},
		{
			name:     "single element",
			input:    []float32{1.5},
			expected: []float64{1.5},
		},
		{
			name:     "multiple elements",
			input:    []float32{0.1, 0.2, 0.3},
			expected: []float64{0.10000000149011612, 0.20000000298023224, 0.30000001192092896},
		},
		{
			name:     "negative values",
			input:    []float32{-1.0, 0.0, 1.0},
			expected: []float64{-1.0, 0.0, 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float32ToFloat64(tt.input)
			if tt.input == nil {
				assert.Nil(t, result)
				return
			}
			require.Len(t, result, len(tt.expected))
			for i := range result {
				assert.InDelta(t, tt.expected[i], result[i], 1e-6, "index %d", i)
			}
		})
	}
}

func TestFloat64ToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected []float32
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []float64{},
			expected: []float32{},
		},
		{
			name:     "single element",
			input:    []float64{1.5},
			expected: []float32{1.5},
		},
		{
			name:     "multiple elements",
			input:    []float64{0.1, 0.2, 0.3},
			expected: []float32{0.1, 0.2, 0.3},
		},
		{
			name:     "negative values",
			input:    []float64{-1.0, 0.0, 1.0},
			expected: []float32{-1.0, 0.0, 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float64ToFloat32(tt.input)
			if tt.input == nil {
				assert.Nil(t, result)
				return
			}
			require.Len(t, result, len(tt.expected))
			for i := range result {
				assert.InDelta(t, float64(tt.expected[i]), float64(result[i]), 1e-6, "index %d", i)
			}
		})
	}
}

func TestProperty_Float32Float64_RoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		length := rapid.IntRange(0, 100).Draw(rt, "length")
		original := make([]float32, length)
		for i := range original {
			original[i] = float32(rapid.Float64Range(-1e6, 1e6).Draw(rt, "element"))
		}

		converted := Float32ToFloat64(original)
		require.Len(t, converted, length)

		roundTripped := Float64ToFloat32(converted)
		require.Len(t, roundTripped, length)

		for i := range original {
			if math.IsNaN(float64(original[i])) {
				assert.True(t, math.IsNaN(float64(roundTripped[i])), "NaN should survive round-trip at index %d", i)
			} else {
				assert.Equal(t, original[i], roundTripped[i], "round-trip should preserve float32 precision at index %d", i)
			}
		}
	})
}

func TestProperty_Float64Float32_LengthPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		length := rapid.IntRange(0, 200).Draw(rt, "length")
		input := make([]float64, length)
		for i := range input {
			input[i] = rapid.Float64Range(-1e6, 1e6).Draw(rt, "element")
		}

		result := Float64ToFloat32(input)
		assert.Len(t, result, length)

		backToFloat64 := Float32ToFloat64(result)
		assert.Len(t, backToFloat64, length)
	})
}
