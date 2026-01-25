package cache

import (
	"testing"

	llmpkg "github.com/BaSui01/agentflow/llm"

	"github.com/stretchr/testify/assert"
)

func TestHashKeyStrategy_GenerateKey(t *testing.T) {
	strategy := NewHashKeyStrategy()

	req := &llmpkg.ChatRequest{
		TenantID: "tenant1",
		Model:    "gpt-4o-mini",
		Messages: []llmpkg.Message{
			{Role: llmpkg.RoleUser, Content: "Hello"},
		},
	}

	key1 := strategy.GenerateKey(req)
	key2 := strategy.GenerateKey(req)

	assert.NotEmpty(t, key1, "缓存键不应为空")
	assert.Equal(t, key1, key2, "相同请求应生成相同的键")
	assert.Contains(t, key1, "llm:cache:", "键应包含前缀")
}

func TestHashKeyStrategy_Name(t *testing.T) {
	strategy := NewHashKeyStrategy()
	assert.Equal(t, "hash", strategy.Name())
}

func TestHierarchicalKeyStrategy_GenerateKey(t *testing.T) {
	strategy := NewHierarchicalKeyStrategy()

	tests := []struct {
		name        string
		req         *llmpkg.ChatRequest
		description string
		assertion   func(*testing.T, string)
	}{
		{
			name: "单轮对话应生成 initial 键",
			req: &llmpkg.ChatRequest{
				TenantID: "tenant1",
				Model:    "gpt-4o-mini",
				Messages: []llmpkg.Message{
					{Role: llmpkg.RoleUser, Content: "Hello"},
				},
			},
			description: "只有一条消息时，应使用 :initial 后缀",
			assertion: func(t *testing.T, key string) {
				assert.Contains(t, key, ":initial", "应包含 :initial 后缀")
				assert.Contains(t, key, "tenant1", "应包含租户 ID")
				assert.Contains(t, key, "gpt-4o-mini", "应包含模型名称")
			},
		},
		{
			name: "多轮对话应生成层次化键",
			req: &llmpkg.ChatRequest{
				TenantID: "tenant1",
				Model:    "gpt-4o-mini",
				Messages: []llmpkg.Message{
					{Role: llmpkg.RoleSystem, Content: "You are a helpful assistant"},
					{Role: llmpkg.RoleUser, Content: "Hello"},
					{Role: llmpkg.RoleAssistant, Content: "Hi!"},
					{Role: llmpkg.RoleUser, Content: "How are you?"},
				},
			},
			description: "多轮对话应生成包含消息 Hash 的键",
			assertion: func(t *testing.T, key string) {
				assert.NotContains(t, key, ":initial", "不应包含 :initial 后缀")
				assert.Contains(t, key, "tenant1", "应包含租户 ID")
				assert.Contains(t, key, "gpt-4o-mini", "应包含模型名称")
				assert.Regexp(t, `llm:cache:tenant1:gpt-4o-mini:[0-9a-f]{24}`, key, "应匹配层次化键格式")
			},
		},
		{
			name: "不同租户应生成不同的键",
			req: &llmpkg.ChatRequest{
				TenantID: "tenant2",
				Model:    "gpt-4o-mini",
				Messages: []llmpkg.Message{
					{Role: llmpkg.RoleUser, Content: "Hello"},
				},
			},
			description: "不同租户应生成不同的键前缀",
			assertion: func(t *testing.T, key string) {
				assert.Contains(t, key, "tenant2", "应包含租户 ID")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := strategy.GenerateKey(tt.req)
			assert.NotEmpty(t, key, "缓存键不应为空")
			tt.assertion(t, key)
		})
	}
}

func TestHierarchicalKeyStrategy_PrefixSharing(t *testing.T) {
	strategy := NewHierarchicalKeyStrategy()

	// 模拟多轮对话
	baseMessages := []llmpkg.Message{
		{Role: llmpkg.RoleSystem, Content: "You are a helpful assistant"},
		{Role: llmpkg.RoleUser, Content: "Hello"},
		{Role: llmpkg.RoleAssistant, Content: "Hi! How can I help you?"},
	}

	req1 := &llmpkg.ChatRequest{
		TenantID: "tenant1",
		Model:    "gpt-4o-mini",
		Messages: append(baseMessages, llmpkg.Message{
			Role:    llmpkg.RoleUser,
			Content: "What's the weather?",
		}),
	}

	req2 := &llmpkg.ChatRequest{
		TenantID: "tenant1",
		Model:    "gpt-4o-mini",
		Messages: append(baseMessages, llmpkg.Message{
			Role:    llmpkg.RoleUser,
			Content: "Tell me a joke",
		}),
	}

	key1 := strategy.GenerateKey(req1)
	key2 := strategy.GenerateKey(req2)

	// 提取前缀部分（去掉消息 Hash）
	prefix1 := key1[:len(key1)-24] // 移除最后 24 个字符（msgHash）
	prefix2 := key2[:len(key2)-24]

	// 前缀应该相同（因为历史消息相同）
	assert.Equal(t, prefix1, prefix2, "相同历史消息应共享缓存前缀")

	// 但完整键应不同（因为最后一条用户消息不同）
	// 注意：层次化策略不包含最后一条消息，所以这里键应该相同！
	assert.Equal(t, key1, key2, "层次化策略：相同历史消息应生成相同的键")
}

func TestHierarchicalKeyStrategy_Name(t *testing.T) {
	strategy := NewHierarchicalKeyStrategy()
	assert.Equal(t, "hierarchical", strategy.Name())
}

func BenchmarkHashKeyStrategy_GenerateKey(b *testing.B) {
	strategy := NewHashKeyStrategy()
	req := &llmpkg.ChatRequest{
		TenantID: "tenant1",
		Model:    "gpt-4o-mini",
		Messages: []llmpkg.Message{
			{Role: llmpkg.RoleSystem, Content: "You are a helpful assistant"},
			{Role: llmpkg.RoleUser, Content: "Hello"},
			{Role: llmpkg.RoleAssistant, Content: "Hi!"},
			{Role: llmpkg.RoleUser, Content: "How are you?"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.GenerateKey(req)
	}
}

func BenchmarkHierarchicalKeyStrategy_GenerateKey(b *testing.B) {
	strategy := NewHierarchicalKeyStrategy()
	req := &llmpkg.ChatRequest{
		TenantID: "tenant1",
		Model:    "gpt-4o-mini",
		Messages: []llmpkg.Message{
			{Role: llmpkg.RoleSystem, Content: "You are a helpful assistant"},
			{Role: llmpkg.RoleUser, Content: "Hello"},
			{Role: llmpkg.RoleAssistant, Content: "Hi!"},
			{Role: llmpkg.RoleUser, Content: "How are you?"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.GenerateKey(req)
	}
}
