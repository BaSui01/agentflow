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
			name:         "gpt-4o",
			provider:     "openai",
			model:        "gpt-4o",
			tokensInput:  1000,
			tokensOutput: 500,
			wantMin:      0.01,
			wantMax:      0.02,
		},
		{
			name:         "gpt-3.5-turbo",
			provider:     "openai",
			model:        "gpt-3.5-turbo",
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

func TestCostTracker_Track(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	// 追踪多次请求
	tracker.Track("openai", "gpt-4o", 1000, 500)
	tracker.Track("openai", "gpt-4o", 2000, 1000)

	summary := tracker.Summary()

	if summary.RequestCount != 2 {
		t.Errorf("RequestCount = %d, want 2", summary.RequestCount)
	}
	if summary.TokensInput != 3000 {
		t.Errorf("TokensInput = %d, want 3000", summary.TokensInput)
	}
	if summary.TokensOutput != 1500 {
		t.Errorf("TokensOutput = %d, want 1500", summary.TokensOutput)
	}
	if summary.TotalCost <= 0 {
		t.Error("TotalCost should be > 0")
	}
}

func TestCostTracker_Reset(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Track("openai", "gpt-4o", 1000, 500)
	tracker.Reset()

	summary := tracker.Summary()
	if summary.RequestCount != 0 {
		t.Errorf("RequestCount after reset = %d, want 0", summary.RequestCount)
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
