package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixRouter_RouteByModelID(t *testing.T) {
	tests := []struct {
		name             string
		rules            []PrefixRule
		modelID          string
		expectedProvider string
		expectedFound    bool
		description      string
	}{
		{
			name: "精确前缀匹配 - OpenAI",
			rules: []PrefixRule{
				{Prefix: "gpt-4o", Provider: "openai"},
				{Prefix: "claude-3", Provider: "anthropic"},
			},
			modelID:          "gpt-4o-mini",
			expectedProvider: "openai",
			expectedFound:    true,
			description:      "应匹配到 OpenAI",
		},
		{
			name: "精确前缀匹配 - Anthropic",
			rules: []PrefixRule{
				{Prefix: "gpt-4o", Provider: "openai"},
				{Prefix: "claude-3-5-sonnet", Provider: "anthropic"},
			},
			modelID:          "claude-3-5-sonnet-20240620",
			expectedProvider: "anthropic",
			expectedFound:    true,
			description:      "应匹配到 Anthropic",
		},
		{
			name: "最长前缀优先",
			rules: []PrefixRule{
				{Prefix: "gpt-4", Provider: "openai_v1"},
				{Prefix: "gpt-4o", Provider: "openai_v2"},
			},
			modelID:          "gpt-4o-mini",
			expectedProvider: "openai_v2",
			expectedFound:    true,
			description:      "应优先匹配最长前缀 'gpt-4o'",
		},
		{
			name: "无匹配规则",
			rules: []PrefixRule{
				{Prefix: "gpt-4o", Provider: "openai"},
				{Prefix: "claude-3", Provider: "anthropic"},
			},
			modelID:          "gemini-2.0-flash",
			expectedProvider: "",
			expectedFound:    false,
			description:      "不匹配任何规则时应返回 false",
		},
		{
			name:             "空规则列表",
			rules:            []PrefixRule{},
			modelID:          "gpt-4o-mini",
			expectedProvider: "",
			expectedFound:    false,
			description:      "空规则列表应返回 false",
		},
		{
			name: "空模型 ID",
			rules: []PrefixRule{
				{Prefix: "gpt-4o", Provider: "openai"},
			},
			modelID:          "",
			expectedProvider: "",
			expectedFound:    false,
			description:      "空模型 ID 应返回 false",
		},
		{
			name: "大小写敏感",
			rules: []PrefixRule{
				{Prefix: "gpt-4o", Provider: "openai"},
			},
			modelID:          "GPT-4O-MINI",
			expectedProvider: "",
			expectedFound:    false,
			description:      "前缀匹配应区分大小写",
		},
		{
			name: "多个规则匹配 - 最长前缀优先",
			rules: []PrefixRule{
				{Prefix: "claude", Provider: "anthropic_base"},
				{Prefix: "claude-3", Provider: "anthropic_v3"},
				{Prefix: "claude-3-5", Provider: "anthropic_v3.5"},
			},
			modelID:          "claude-3-5-sonnet-20240620",
			expectedProvider: "anthropic_v3.5",
			expectedFound:    true,
			description:      "应匹配最长前缀 'claude-3-5'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewPrefixRouter(tt.rules)

			provider, found := router.RouteByModelID(tt.modelID)

			assert.Equal(t, tt.expectedFound, found, tt.description)
			if tt.expectedFound {
				assert.Equal(t, tt.expectedProvider, provider, tt.description)
			}
		})
	}
}

func TestPrefixRouter_GetRules(t *testing.T) {
	rules := []PrefixRule{
		{Prefix: "gpt-4o", Provider: "openai"},
		{Prefix: "claude-3", Provider: "anthropic"},
	}

	router := NewPrefixRouter(rules)
	retrievedRules := router.GetRules()

	assert.Equal(t, len(rules), len(retrievedRules), "规则数量应相等")
}

func TestPrefixRouter_RuleSorting(t *testing.T) {
	// 测试规则是否按前缀长度降序排列
	rules := []PrefixRule{
		{Prefix: "gpt", Provider: "p1"},         // 最短
		{Prefix: "gpt-4o-mini", Provider: "p2"}, // 最长
		{Prefix: "gpt-4", Provider: "p3"},       // 中等
		{Prefix: "gpt-4o", Provider: "p4"},      // 中等
	}

	router := NewPrefixRouter(rules)
	sortedRules := router.GetRules()

	// 验证排序：最长前缀应在前面
	assert.Equal(t, "gpt-4o-mini", sortedRules[0].Prefix, "最长前缀应排在第一位")
	assert.Equal(t, "gpt", sortedRules[len(sortedRules)-1].Prefix, "最短前缀应排在最后")

	// 验证排序后匹配逻辑正确
	provider, found := router.RouteByModelID("gpt-4o-mini-test")
	assert.True(t, found)
	assert.Equal(t, "p2", provider, "应匹配最长前缀")
}

func TestPrefixRouter_NilRouter(t *testing.T) {
	var router *PrefixRouter = nil

	provider, found := router.RouteByModelID("gpt-4o-mini")
	assert.False(t, found, "nil 路由器应返回 false")
	assert.Equal(t, "", provider, "nil 路由器应返回空字符串")
}

func BenchmarkPrefixRouter_RouteByModelID(b *testing.B) {
	rules := []PrefixRule{
		{Prefix: "gpt-4o", Provider: "openai"},
		{Prefix: "gpt-4-turbo", Provider: "openai"},
		{Prefix: "gpt-3.5", Provider: "openai"},
		{Prefix: "claude-3-5-sonnet", Provider: "anthropic"},
		{Prefix: "claude-3-opus", Provider: "anthropic"},
		{Prefix: "gemini-2.0-flash", Provider: "google"},
		{Prefix: "gemini-1.5-pro", Provider: "google"},
	}

	router := NewPrefixRouter(rules)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.RouteByModelID("gpt-4o-mini")
	}
}
