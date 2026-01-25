package context_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/llm/context"
	"go.uber.org/zap"
)

// 示例：使用 Tokenizer 计算 token 数
func ExampleTokenizer() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 创建 Tokenizer
	tokenizer := context.NewEstimateTokenizer()

	// 计算文本 tokens
	text := "Hello, how are you today?"
	tokens := tokenizer.CountTokens(text)
	fmt.Printf("Text tokens: %d\n", tokens)

	// 计算消息 tokens
	msg := context.Message{
		Role:    context.RoleUser,
		Content: "What's the weather like in San Francisco?",
	}
	msgTokens := tokenizer.CountMessageTokens(msg)
	fmt.Printf("Message tokens: %d\n", msgTokens)

	// 计算消息列表 tokens
	msgs := []context.Message{
		{
			Role:    context.RoleSystem,
			Content: "You are a helpful assistant.",
		},
		{
			Role:    context.RoleUser,
			Content: "Tell me about the Golden Gate Bridge.",
		},
		{
			Role:    context.RoleAssistant,
			Content: "The Golden Gate Bridge is a suspension bridge...",
		},
	}
	totalTokens := tokenizer.CountMessagesTokens(msgs)
	fmt.Printf("Total tokens: %d\n", totalTokens)
}

// 示例：使用 ContextManager 裁剪消息
func ExampleContextManager() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tokenizer := context.NewEstimateTokenizer()
	manager := context.NewDefaultContextManager(tokenizer, logger)

	// 创建一个长消息列表
	msgs := []context.Message{
		{Role: context.RoleSystem, Content: "You are a helpful assistant."},
		{Role: context.RoleUser, Content: "Message 1"},
		{Role: context.RoleAssistant, Content: "Response 1"},
		{Role: context.RoleUser, Content: "Message 2"},
		{Role: context.RoleAssistant, Content: "Response 2"},
		{Role: context.RoleUser, Content: "Message 3"},
		{Role: context.RoleAssistant, Content: "Response 3"},
		{Role: context.RoleUser, Content: "Message 4"},
	}

	// 估算当前 tokens
	currentTokens := manager.EstimateTokens(msgs)
	fmt.Printf("Current tokens: %d\n", currentTokens)

	// 裁剪到 50 tokens
	trimmed, err := manager.TrimMessages(msgs, 50)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	fmt.Printf("Trimmed to %d messages\n", len(trimmed))
	fmt.Printf("Trimmed tokens: %d\n", manager.EstimateTokens(trimmed))
}

// 测试：Tokenizer 基本功能
func TestTokenizer(t *testing.T) {
	tokenizer := context.NewEstimateTokenizer()

	// 测试空文本
	if tokens := tokenizer.CountTokens(""); tokens != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", tokens)
	}

	// 测试英文文本
	english := "Hello, world!"
	englishTokens := tokenizer.CountTokens(english)
	if englishTokens <= 0 {
		t.Errorf("expected positive tokens for English text, got %d", englishTokens)
	}

	// 测试中文文本
	chinese := "你好，世界！"
	chineseTokens := tokenizer.CountTokens(chinese)
	if chineseTokens <= 0 {
		t.Errorf("expected positive tokens for Chinese text, got %d", chineseTokens)
	}

	// 中文应该比英文更密集（每个字符更多 tokens）
	if chineseTokens <= englishTokens {
		t.Logf("Chinese tokens: %d, English tokens: %d", chineseTokens, englishTokens)
	}
}

// 测试：Context Manager 裁剪策略
func TestContextManagerPruneStrategies(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tokenizer := context.NewEstimateTokenizer()
	manager := context.NewDefaultContextManager(tokenizer, logger)

	msgs := []context.Message{
		{Role: context.RoleSystem, Content: "System message"},
		{Role: context.RoleUser, Content: "User message 1"},
		{Role: context.RoleAssistant, Content: "Assistant response 1"},
		{Role: context.RoleUser, Content: "User message 2"},
		{Role: context.RoleTool, Name: "tool1", Content: "Tool result"},
		{Role: context.RoleUser, Content: "User message 3"},
	}

	maxTokens := 30

	strategies := []context.PruneStrategy{
		context.PruneOldest,
		context.PruneByRole,
		context.PruneSlidingWindow,
		context.PruneToolCalls,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			trimmed, err := manager.PruneByStrategy(msgs, maxTokens, strategy)
			if err != nil {
				t.Fatalf("strategy %s failed: %v", strategy, err)
			}

			trimmedTokens := manager.EstimateTokens(trimmed)
			if trimmedTokens > maxTokens {
				t.Errorf("strategy %s exceeded token limit: %d > %d", strategy, trimmedTokens, maxTokens)
			}

			t.Logf("Strategy %s: %d messages, %d tokens", strategy, len(trimmed), trimmedTokens)
		})
	}
}

// 测试：保留 System 消息
func TestContextManagerPreservesSystem(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tokenizer := context.NewEstimateTokenizer()
	manager := context.NewDefaultContextManager(tokenizer, logger)

	msgs := []context.Message{
		{Role: context.RoleSystem, Content: "You are a helpful assistant."},
		{Role: context.RoleUser, Content: "Tell me a long story about..."},
		{Role: context.RoleAssistant, Content: "Once upon a time..."},
	}

	// 裁剪到很小的 token 数
	trimmed, err := manager.TrimMessages(msgs, 20)
	if err != nil {
		t.Fatalf("trim failed: %v", err)
	}

	// 应该保留 System 消息
	hasSystem := false
	for _, msg := range trimmed {
		if msg.Role == context.RoleSystem {
			hasSystem = true
			break
		}
	}

	if !hasSystem {
		t.Error("System message should be preserved")
	}
}

// 测试：Tool tokens 估算
func TestEstimateToolTokens(t *testing.T) {
	tokenizer := context.NewEstimateTokenizer()

	tools := []context.ToolSchema{
		{
			Name:        "get_weather",
			Description: "Get the current weather",
			Parameters:  json.RawMessage(`{"type": "object", "properties": {"location": {"type": "string"}}}`),
		},
		{
			Name:        "search",
			Description: "Search the web",
			Parameters:  json.RawMessage(`{"type": "object", "properties": {"query": {"type": "string"}}}`),
		},
	}

	tokens := tokenizer.EstimateToolTokens(tools)
	if tokens <= 0 {
		t.Errorf("expected positive tool tokens, got %d", tokens)
	}

	t.Logf("Tool tokens: %d", tokens)
}

// 测试：完整请求 token 计算
func TestCountRequestTokens(t *testing.T) {
	tokenizer := context.NewEstimateTokenizer()

	req := &context.ChatRequest{
		Messages: []context.Message{
			{Role: context.RoleSystem, Content: "You are a helpful assistant."},
			{Role: context.RoleUser, Content: "What's the weather?"},
		},
		Tools: []context.ToolSchema{
			{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  json.RawMessage(`{"type": "object"}`),
			},
		},
		MaxTokens: 500,
	}

	// 计算输入 tokens
	inputTokens := context.CountRequestTokens(req, tokenizer)
	if inputTokens <= 0 {
		t.Errorf("expected positive input tokens, got %d", inputTokens)
	}

	// 计算总 tokens（输入 + 输出）
	totalTokens := context.TotalRequestTokens(req, tokenizer)
	if totalTokens <= inputTokens {
		t.Errorf("total tokens should be greater than input tokens")
	}

	t.Logf("Input tokens: %d, Total tokens: %d", inputTokens, totalTokens)
}

// 基准测试：Tokenizer 性能
func BenchmarkTokenizer(b *testing.B) {
	tokenizer := context.NewEstimateTokenizer()
	text := "This is a sample text for benchmarking the tokenizer performance."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.CountTokens(text)
	}
}

// 基准测试：Context Manager 性能
func BenchmarkContextManager(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	tokenizer := context.NewEstimateTokenizer()
	manager := context.NewDefaultContextManager(tokenizer, logger)

	msgs := make([]context.Message, 100)
	for i := 0; i < 100; i++ {
		msgs[i] = context.Message{
			Role:    context.RoleUser,
			Content: fmt.Sprintf("Message %d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.TrimMessages(msgs, 1000)
	}
}

// 示例：使用滑动窗口策略
func ExamplePruneSlidingWindow() {
	logger, _ := zap.NewDevelopment()
	tokenizer := context.NewEstimateTokenizer()
	manager := context.NewDefaultContextManager(tokenizer, logger)

	msgs := []context.Message{
		{Role: context.RoleUser, Content: "Oldest message"},
		{Role: context.RoleAssistant, Content: "Response 1"},
		{Role: context.RoleUser, Content: "Middle message"},
		{Role: context.RoleAssistant, Content: "Response 2"},
		{Role: context.RoleUser, Content: "Recent message"},
		{Role: context.RoleAssistant, Content: "Response 3"},
	}

	// 使用滑动窗口策略保留最近的消息
	trimmed, _ := manager.PruneByStrategy(msgs, 30, context.PruneSlidingWindow)

	fmt.Printf("Original messages: %d\n", len(msgs))
	fmt.Printf("Trimmed messages: %d\n", len(trimmed))
	fmt.Printf("First message: %s\n", trimmed[0].Content)
	fmt.Printf("Last message: %s\n", trimmed[len(trimmed)-1].Content)
}
