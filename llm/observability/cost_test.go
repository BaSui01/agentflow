package observability

import (
	"testing"
)

func TestCostCalculator_Calculate(t *testing.T) {
	calc := NewCostCalculator()

	tests := []struct {
		name         string
		provider     string
		model        string
		tokensInput  int
		tokensOutput int
		wantMin      float64
		wantMax      float64
	}{
		{
			name:         "gpt-5.4",
			provider:     "openai",
			model:        "gpt-5.4",
			tokensInput:  1000,
			tokensOutput: 500,
			wantMin:      0.01,
			wantMax:      0.02,
		},
		{
			name:         "gpt-5.4-nano",
			provider:     "openai",
			model:        "gpt-5.4-nano",
			tokensInput:  1000,
			tokensOutput: 500,
			wantMin:      0.0001,
			wantMax:      0.002,
		},
		{
			name:         "unknown model",
			provider:     "unknown",
			model:        "unknown",
			tokensInput:  1000,
			tokensOutput: 500,
			wantMin:      0,
			wantMax:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.Calculate(tt.provider, tt.model, tt.tokensInput, tt.tokensOutput)
			if cost < tt.wantMin || cost > tt.wantMax {
				t.Errorf("Calculate() = %v, want between %v and %v", cost, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCostCalculator_SetPrice(t *testing.T) {
	calc := NewCostCalculator()

	// 设置自定义价格
	calc.SetPrice("custom", "custom-model", 0.01, 0.02)

	cost := calc.Calculate("custom", "custom-model", 1000, 1000)
	expected := 0.01 + 0.02 // 1K input + 1K output
	if cost != expected {
		t.Errorf("Calculate() = %v, want %v", cost, expected)
	}
}

