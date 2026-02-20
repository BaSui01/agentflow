package providers

import (
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
)

// TestChooseModel Priority 测试模型选择优先级 :
// 请求 > 配置 > 默认(要求14.1、14.2、14.3)
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

// 测试ChooseModel  提供缺陷测试, 每个提供者的默认模式
// 在没有指定其他模型时正确返回
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

// 测试ChooseModel EmptyStrings 空字符串处理对零
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

// TestChooseModel Real WorldScreators 测试现实的使用情景
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

// 测试模式  Nil 请求安全处理零请求
func TestChooseModel_NilRequest(t *testing.T) {
	result := ChooseModel(nil, "config-model", "default-model")
	assert.Equal(t, "config-model", result, "Should use config model when request is nil")

	result = ChooseModel(nil, "", "default-model")
	assert.Equal(t, "default-model", result, "Should use default model when request is nil and config is empty")
}

// 测试ChooseModel 一致性测试,该函数具有确定性
func TestChooseModel_Consistency(t *testing.T) {
	req := &llm.ChatRequest{Model: "test-model"}
	configModel := "config-model"
	defaultModel := "default-model"

	// 用相同的输入调用多次
	result1 := ChooseModel(req, configModel, defaultModel)
	result2 := ChooseModel(req, configModel, defaultModel)
	result3 := ChooseModel(req, configModel, defaultModel)

	assert.Equal(t, result1, result2, "Function should be deterministic")
	assert.Equal(t, result2, result3, "Function should be deterministic")
	assert.Equal(t, "test-model", result1, "Should consistently return request model")
}
