package config

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPolicyManager(t *testing.T) {
	pm := NewPolicyManager()
	require.NotNil(t, pm)
	assert.Empty(t, pm.policies)
	assert.NotNil(t, pm.byProvider)
	assert.NotNil(t, pm.byError)
}

func TestPolicyManager_Update(t *testing.T) {
	pm := NewPolicyManager()

	policies := []FallbackPolicy{
		{ID: "p2", Priority: 10, Enabled: true, TriggerProvider: "openai"},
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "anthropic"},
		{ID: "p3", Priority: 5, Enabled: false, TriggerProvider: "google"},
	}

	pm.Update(policies)

	// Should be sorted by priority ascending
	assert.Equal(t, "p1", pm.policies[0].ID)
	assert.Equal(t, "p3", pm.policies[1].ID)
	assert.Equal(t, "p2", pm.policies[2].ID)
}

func TestPolicyManager_Update_RebuildIndex(t *testing.T) {
	pm := NewPolicyManager()

	policies := []FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai", TriggerErrors: []string{"rate_limit", "timeout"}},
		{ID: "p2", Priority: 2, Enabled: true, TriggerProvider: "", TriggerErrors: []string{"rate_limit"}},
		{ID: "p3", Priority: 3, Enabled: false, TriggerProvider: "anthropic"},
	}

	pm.Update(policies)

	// Disabled policies should not be indexed
	assert.Len(t, pm.byProvider["openai"], 1)
	assert.Len(t, pm.byProvider["*"], 1) // global policy (empty provider)
	assert.Empty(t, pm.byProvider["anthropic"])

	// Error index
	assert.Len(t, pm.byError["rate_limit"], 2)
	assert.Len(t, pm.byError["timeout"], 1)
}

func TestPolicyManager_FindPolicy(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "exact", Priority: 1, Enabled: true, TriggerProvider: "openai", TriggerModel: "gpt-4", TriggerErrors: []string{"rate_limit"}},
		{ID: "provider-only", Priority: 2, Enabled: true, TriggerProvider: "openai"},
		{ID: "global", Priority: 3, Enabled: true},
	})

	tests := []struct {
		name     string
		provider string
		model    string
		errCode  ErrorCode
		wantID   string
		wantNil  bool
	}{
		{
			name:     "exact match",
			provider: "openai",
			model:    "gpt-4",
			errCode:  "rate_limit",
			wantID:   "exact",
		},
		{
			name:     "provider match without error filter",
			provider: "openai",
			model:    "gpt-3.5",
			errCode:  "",
			wantID:   "provider-only",
		},
		{
			name:     "global match",
			provider: "anthropic",
			model:    "claude",
			errCode:  "",
			wantID:   "global",
		},
		{
			name:     "no match when error code required but not matched",
			provider: "openai",
			model:    "gpt-4",
			errCode:  "unknown_error",
			wantID:   "provider-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.FindPolicy(tt.provider, tt.model, tt.errCode)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantID, result.ID)
			}
		})
	}
}

func TestPolicyManager_FindPolicy_NoMatch(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai", TriggerModel: "gpt-4", TriggerErrors: []string{"rate_limit"}},
	})

	result := pm.FindPolicy("anthropic", "claude", "rate_limit")
	assert.Nil(t, result)
}

func TestPolicyManager_FindPolicy_DisabledSkipped(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "disabled", Priority: 1, Enabled: false, TriggerProvider: "openai"},
	})

	result := pm.FindPolicy("openai", "gpt-4", "")
	assert.Nil(t, result)
}

func TestPolicyManager_GetFallbackChain(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai", TriggerModel: "gpt-4"},
		{ID: "p2", Priority: 2, Enabled: true, TriggerProvider: "openai"},
		{ID: "p3", Priority: 3, Enabled: true},
		{ID: "p4", Priority: 4, Enabled: false, TriggerProvider: "openai"},
		{ID: "p5", Priority: 5, Enabled: true, TriggerProvider: "anthropic"},
	})

	chain := pm.GetFallbackChain("openai", "gpt-4")
	assert.Len(t, chain, 3)
	assert.Equal(t, "p1", chain[0].ID)
	assert.Equal(t, "p2", chain[1].ID)
	assert.Equal(t, "p3", chain[2].ID)
}

func TestPolicyManager_GetFallbackChain_Empty(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai", TriggerModel: "gpt-4"},
	})

	chain := pm.GetFallbackChain("anthropic", "claude")
	assert.Empty(t, chain)
}

func TestPolicyManager_GetFallbackAction(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{
			ID:               "p1",
			Priority:         1,
			Enabled:          true,
			TriggerProvider:  "openai",
			FallbackType:     FallbackModel,
			FallbackTarget:   "gpt-3.5-turbo",
			FallbackTemplate: "fallback template",
		},
	})

	action := pm.GetFallbackAction("openai", "gpt-4", "")
	require.NotNil(t, action)
	assert.Equal(t, FallbackModel, action.Type)
	assert.Equal(t, "gpt-3.5-turbo", action.Target)
	assert.Equal(t, "fallback template", action.Template)
	assert.NotNil(t, action.Policy)
}

func TestPolicyManager_GetFallbackAction_NoMatch(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai"},
	})

	action := pm.GetFallbackAction("anthropic", "claude", "")
	assert.Nil(t, action)
}

func TestPolicyManager_ShouldRetry(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai", RetryMax: 3},
	})

	assert.True(t, pm.ShouldRetry("openai", "gpt-4", "", 0))
	assert.True(t, pm.ShouldRetry("openai", "gpt-4", "", 2))
	assert.False(t, pm.ShouldRetry("openai", "gpt-4", "", 3))
	assert.False(t, pm.ShouldRetry("openai", "gpt-4", "", 5))
}

func TestPolicyManager_ShouldRetry_NoPolicy(t *testing.T) {
	pm := NewPolicyManager()
	assert.False(t, pm.ShouldRetry("openai", "gpt-4", "", 0))
}

func TestPolicyManager_GetRetryDelay(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, RetryDelayMs: 1000, RetryMultiplier: 2.0},
	})

	assert.Equal(t, 1000, pm.GetRetryDelay("openai", "gpt-4", "", 0))
	assert.Equal(t, 2000, pm.GetRetryDelay("openai", "gpt-4", "", 1))
	assert.Equal(t, 4000, pm.GetRetryDelay("openai", "gpt-4", "", 2))
}

func TestPolicyManager_GetRetryDelay_NoPolicy(t *testing.T) {
	pm := NewPolicyManager()
	assert.Equal(t, 1000, pm.GetRetryDelay("unknown", "model", "", 0))
}

func TestPolicyManager_ConcurrentAccess(t *testing.T) {
	pm := NewPolicyManager()
	pm.Update([]FallbackPolicy{
		{ID: "p1", Priority: 1, Enabled: true, TriggerProvider: "openai", RetryMax: 3, RetryDelayMs: 100, RetryMultiplier: 1.5},
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pm.FindPolicy("openai", "gpt-4", "")
			pm.GetFallbackChain("openai", "gpt-4")
			pm.GetFallbackAction("openai", "gpt-4", "")
			pm.ShouldRetry("openai", "gpt-4", "", 1)
			pm.GetRetryDelay("openai", "gpt-4", "", 1)
		}()
	}
	wg.Wait()
}

func TestMatchPolicy(t *testing.T) {
	pm := NewPolicyManager()

	tests := []struct {
		name     string
		policy   FallbackPolicy
		provider string
		model    string
		errCode  ErrorCode
		want     bool
	}{
		{
			name:     "empty policy matches everything",
			policy:   FallbackPolicy{Enabled: true},
			provider: "openai",
			model:    "gpt-4",
			errCode:  "",
			want:     true,
		},
		{
			name:     "provider mismatch",
			policy:   FallbackPolicy{Enabled: true, TriggerProvider: "anthropic"},
			provider: "openai",
			model:    "gpt-4",
			errCode:  "",
			want:     false,
		},
		{
			name:     "model mismatch",
			policy:   FallbackPolicy{Enabled: true, TriggerModel: "gpt-3.5"},
			provider: "openai",
			model:    "gpt-4",
			errCode:  "",
			want:     false,
		},
		{
			name:     "error code match",
			policy:   FallbackPolicy{Enabled: true, TriggerErrors: []string{"rate_limit", "timeout"}},
			provider: "openai",
			model:    "gpt-4",
			errCode:  "timeout",
			want:     true,
		},
		{
			name:     "error code mismatch",
			policy:   FallbackPolicy{Enabled: true, TriggerErrors: []string{"rate_limit"}},
			provider: "openai",
			model:    "gpt-4",
			errCode:  "timeout",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pm.matchPolicy(&tt.policy, tt.provider, tt.model, tt.errCode)
			assert.Equal(t, tt.want, got)
		})
	}
}
