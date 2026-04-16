package rag

import (
	llmtokenizer "github.com/BaSui01/agentflow/llm/tokenizer"
	"go.uber.org/zap"
)

// LLMTokenizerAdapter 将 llm/tokenizer.Tokenizer 适配为 rag.Tokenizer 接口。
// 当底层 tokenizer 返回 error 时，回退到字符估算并记录警告日志。
type LLMTokenizerAdapter struct {
	inner  llmtokenizer.Tokenizer
	logger *zap.Logger
}

// NewLLMTokenizerAdapter 创建适配器。
func NewLLMTokenizerAdapter(inner llmtokenizer.Tokenizer, logger *zap.Logger) *LLMTokenizerAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LLMTokenizerAdapter{inner: inner, logger: logger}
}

// CountTokens 返回文本的 token 数。
// 底层 tokenizer 出错时回退到 len(text)/4 估算。
func (a *LLMTokenizerAdapter) CountTokens(text string) int {
	count, err := a.inner.CountTokens(text)
	if err != nil {
		a.logger.Warn("tokenizer CountTokens failed, falling back to estimate", zap.Error(err))
		return len(text) / 4
	}
	return count
}

// Encode 将文本转换为 token ID 列表。
// 底层 tokenizer 出错时回退到伪 token ID 序列。
func (a *LLMTokenizerAdapter) Encode(text string) []int {
	tokens, err := a.inner.Encode(text)
	if err != nil {
		a.logger.Warn("tokenizer Encode failed, falling back to estimate", zap.Error(err))
		result := make([]int, len(text)/4)
		for i := range result {
			result[i] = i
		}
		return result
	}
	return tokens
}
