package tokenizer

import (
	"fmt"
	"sync"
)

// Tokenizer 是 LLM 层完整 tokenizer 接口。
//
// 跨包共享的最小契约位于 pkg/tokenizer.Tokenizer；本接口在共享契约
// CountTokens/Encode/Decode/MaxTokens/Name 之上补充 CountMessages，用于 LLM
// 消息级 token 估算。types.Tokenizer 与 rag.Tokenizer 仍保持各自无 error 的
// 层内形状，并通过 pkg/tokenizer 的 adapter 统一边界。
//
// 本接口返回 error 以支持真实 tokenizer（如 tiktoken）的错误处理。
// 使用 rag/runtime.NewSharedTokenizerAdapter() 可将
// 本接口适配为 RAG 分块 tokenizer。
type Tokenizer interface {
	// CountTokens 返回给定文本的 token 数.
	CountTokens(text string) (int, error)

	// CountMessages 返回消息列表的总 token 数,
	// 包括每条消息的开销（角色标记、分隔符等）。
	CountMessages(messages []Message) (int, error)

	// Encode 将文本转换为 token ID 列表.
	Encode(text string) ([]int, error)

	// Decode 将 token ID 转换回文本.
	Decode(tokens []int) (string, error)

	// MaxTokens 返回模型的最大上下文长度.
	MaxTokens() int

	// Name 返回分词器的名称.
	Name() string
}

// Message 是一个轻量级消息结构, 由 tokenizer 包使用
// 以避免与 llm 包的循环依赖。
type Message struct {
	Role    string
	Content string
}

// 全局分词器注册表.
var (
	modelTokenizers   = make(map[string]Tokenizer)
	modelTokenizersMu sync.RWMutex
)

// RegisterTokenizer 为给定的模型名称注册分词器.
func RegisterTokenizer(model string, t Tokenizer) {
	modelTokenizersMu.Lock()
	defer modelTokenizersMu.Unlock()
	modelTokenizers[model] = t
}

// GetTokenizer 返回为给定型号注册的标定器 。
// 它也尝试了前缀匹配(如"gpt-4o"匹配"gpt-4o-mini").
func GetTokenizer(model string) (Tokenizer, error) {
	modelTokenizersMu.RLock()
	defer modelTokenizersMu.RUnlock()

	if t, ok := modelTokenizers[model]; ok {
		return t, nil
	}

	// 尝试前缀匹配 。
	for prefix, t := range modelTokenizers {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return t, nil
		}
	}

	return nil, fmt.Errorf("no tokenizer registered for model: %s", model)
}

// GetTokenizer OrEstimator 返回该模型的注册代号器,
// 如果没有登记,则回到一般估计器。
func GetTokenizerOrEstimator(model string) Tokenizer {
	t, err := GetTokenizer(model)
	if err != nil {
		return NewEstimatorTokenizer(model, 0)
	}
	return t
}
