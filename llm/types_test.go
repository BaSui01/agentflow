package llm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLLMProviderStatus_String(t *testing.T) {
	tests := []struct {
		status LLMProviderStatus
		want   string
	}{
		{LLMProviderStatusInactive, "inactive"},
		{LLMProviderStatusActive, "active"},
		{LLMProviderStatusDisabled, "disabled"},
		{LLMProviderStatus(99), "LLMProviderStatus(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}

func TestLLMProviderAPIKey_IncrementUsage(t *testing.T) {
	t.Run("success increments counters", func(t *testing.T) {
		k := &LLMProviderAPIKey{Enabled: true}
		k.IncrementUsage(true)

		assert.Equal(t, int64(1), k.TotalRequests)
		assert.Equal(t, int64(0), k.FailedRequests)
		assert.NotNil(t, k.LastUsedAt)
		assert.Nil(t, k.LastErrorAt)
		assert.Equal(t, 1, k.CurrentRPM)
		assert.Equal(t, 1, k.CurrentRPD)
	})

	t.Run("failure increments failure counters", func(t *testing.T) {
		k := &LLMProviderAPIKey{Enabled: true}
		k.IncrementUsage(false)

		assert.Equal(t, int64(1), k.TotalRequests)
		assert.Equal(t, int64(1), k.FailedRequests)
		assert.NotNil(t, k.LastUsedAt)
		assert.NotNil(t, k.LastErrorAt)
	})

	t.Run("RPM counter resets after window", func(t *testing.T) {
		k := &LLMProviderAPIKey{
			Enabled:    true,
			CurrentRPM: 50,
			RPMResetAt: time.Now().Add(-time.Minute), // expired
		}
		k.IncrementUsage(true)
		// RPM should have been reset to 0 then incremented to 1
		assert.Equal(t, 1, k.CurrentRPM)
	})

	t.Run("RPD counter resets after window", func(t *testing.T) {
		k := &LLMProviderAPIKey{
			Enabled:    true,
			CurrentRPD: 1000,
			RPDResetAt: time.Now().Add(-25 * time.Hour), // expired
		}
		k.IncrementUsage(true)
		assert.Equal(t, 1, k.CurrentRPD)
	})
}

func TestLLMProviderAPIKey_IsHealthy_RPDLimited(t *testing.T) {
	now := time.Now()
	k := &LLMProviderAPIKey{
		Enabled:      true,
		RateLimitRPD: 100,
		CurrentRPD:   100,
		RPDResetAt:   now.Add(time.Hour),
	}
	assert.False(t, k.IsHealthy())
}

func TestLLMProviderAPIKey_IsHealthy_LowRequests(t *testing.T) {
	// Under 100 total requests, fail rate check is skipped
	k := &LLMProviderAPIKey{
		Enabled:        true,
		TotalRequests:  50,
		FailedRequests: 40, // 80% fail rate but under threshold
	}
	assert.True(t, k.IsHealthy())
}

func TestTableNames(t *testing.T) {
	assert.Equal(t, "sc_llm_models", LLMModel{}.TableName())
	assert.Equal(t, "sc_llm_providers", LLMProvider{}.TableName())
	assert.Equal(t, "sc_llm_provider_models", LLMProviderModel{}.TableName())
	assert.Equal(t, "sc_llm_provider_api_keys", LLMProviderAPIKey{}.TableName())
}
