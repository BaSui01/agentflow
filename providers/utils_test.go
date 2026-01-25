package providers

import (
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
)

// TestChooseModel_Priority tests the model selection priority:
// request > config > default (Requirements 14.1, 14.2, 14.3)
func TestChooseModel_Priority(t *testing.T) {
	tests := []struct {
		name          string
		req           *llm.ChatRequest
		configModel   string
		defaultModel  string
		expectedModel string
	}{
		{
			name: "Request model takes priority over config and default",
			req: &llm.ChatRequest{
				Model: "request-model",
			},
			configModel:   "config-model",
			defaultModel:  "default-model",
			expectedModel: "request-model",
		},
		{
			name: "Config model takes priority over default when request is empty",
			req: &llm.ChatRequest{
				Model: "",
			},
			configModel:   "config-model",
			defaultModel:  "default-model",
			expectedModel: "config-model",
		},
		{
			name:          "Default model used when both request and config are empty",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "",
			defaultModel:  "default-model",
			expectedModel: "default-model",
		},
		{
			name:          "Default model used when request is nil",
			req:           nil,
			configModel:   "",
			defaultModel:  "default-model",
			expectedModel: "default-model",
		},
		{
			name:          "Config model used when request is nil and config is set",
			req:           nil,
			configModel:   "config-model",
			defaultModel:  "default-model",
			expectedModel: "config-model",
		},
		{
			name: "Request model used even when it's the only one set",
			req: &llm.ChatRequest{
				Model: "request-model",
			},
			configModel:   "",
			defaultModel:  "",
			expectedModel: "request-model",
		},
		{
			name:          "Config model used even when it's the only one set",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "config-model",
			defaultModel:  "",
			expectedModel: "config-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChooseModel(tt.req, tt.configModel, tt.defaultModel)
			assert.Equal(t, tt.expectedModel, result, "Model selection priority mismatch")
		})
	}
}

// TestChooseModel_ProviderDefaults tests that each provider's default model
// is correctly returned when no other model is specified
func TestChooseModel_ProviderDefaults(t *testing.T) {
	providerDefaults := map[string]string{
		"grok":     "grok-beta",
		"glm":      "glm-4-plus",
		"minimax":  "abab6.5s-chat",
		"qwen":     "qwen-plus",
		"deepseek": "deepseek-chat",
	}

	for provider, defaultModel := range providerDefaults {
		t.Run(provider+"_default", func(t *testing.T) {
			result := ChooseModel(nil, "", defaultModel)
			assert.Equal(t, defaultModel, result, "Provider default model mismatch")
		})
	}
}

// TestChooseModel_EmptyStrings tests handling of empty strings vs nil
func TestChooseModel_EmptyStrings(t *testing.T) {
	tests := []struct {
		name          string
		req           *llm.ChatRequest
		configModel   string
		defaultModel  string
		expectedModel string
	}{
		{
			name:          "Empty request model string is treated as not set",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "config-model",
			defaultModel:  "default-model",
			expectedModel: "config-model",
		},
		{
			name:          "Empty config model string is treated as not set",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "",
			defaultModel:  "default-model",
			expectedModel: "default-model",
		},
		{
			name:          "All empty strings fall back to default",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "",
			defaultModel:  "default-model",
			expectedModel: "default-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChooseModel(tt.req, tt.configModel, tt.defaultModel)
			assert.Equal(t, tt.expectedModel, result, "Empty string handling mismatch")
		})
	}
}

// TestChooseModel_RealWorldScenarios tests realistic usage scenarios
func TestChooseModel_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name          string
		req           *llm.ChatRequest
		configModel   string
		defaultModel  string
		expectedModel string
		description   string
	}{
		{
			name: "User overrides provider default with specific model",
			req: &llm.ChatRequest{
				Model: "gpt-4-turbo",
			},
			configModel:   "",
			defaultModel:  "grok-beta",
			expectedModel: "gpt-4-turbo",
			description:   "User wants to use a specific model for this request",
		},
		{
			name:          "Application-wide config sets default model",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "glm-4-plus",
			defaultModel:  "glm-4",
			expectedModel: "glm-4-plus",
			description:   "Application config overrides provider default",
		},
		{
			name:          "Provider default used in simple setup",
			req:           &llm.ChatRequest{Model: ""},
			configModel:   "",
			defaultModel:  "qwen-plus",
			expectedModel: "qwen-plus",
			description:   "No customization, use provider default",
		},
		{
			name: "Request model overrides application config",
			req: &llm.ChatRequest{
				Model: "deepseek-coder",
			},
			configModel:   "deepseek-chat",
			defaultModel:  "deepseek-chat",
			expectedModel: "deepseek-coder",
			description:   "Per-request model takes highest priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChooseModel(tt.req, tt.configModel, tt.defaultModel)
			assert.Equal(t, tt.expectedModel, result, "Scenario: %s", tt.description)
		})
	}
}

// TestChooseModel_NilRequest tests that nil request is handled safely
func TestChooseModel_NilRequest(t *testing.T) {
	result := ChooseModel(nil, "config-model", "default-model")
	assert.Equal(t, "config-model", result, "Should use config model when request is nil")

	result = ChooseModel(nil, "", "default-model")
	assert.Equal(t, "default-model", result, "Should use default model when request is nil and config is empty")
}

// TestChooseModel_Consistency tests that the function is deterministic
func TestChooseModel_Consistency(t *testing.T) {
	req := &llm.ChatRequest{Model: "test-model"}
	configModel := "config-model"
	defaultModel := "default-model"

	// Call multiple times with same inputs
	result1 := ChooseModel(req, configModel, defaultModel)
	result2 := ChooseModel(req, configModel, defaultModel)
	result3 := ChooseModel(req, configModel, defaultModel)

	assert.Equal(t, result1, result2, "Function should be deterministic")
	assert.Equal(t, result2, result3, "Function should be deterministic")
	assert.Equal(t, "test-model", result1, "Should consistently return request model")
}
