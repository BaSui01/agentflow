package tokenizer

import (
	"fmt"

	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

// TokenizerAdapter 将 llm/tokenizer.Tokenizer 适配为 rag.Tokenizer 接口。
// 当底层 tokenizer 返回 error 时，回退到字符估算并记录警告日志。
type TokenizerAdapter struct {
	inner  Tokenizer
	logger *zap.Logger
}

// NewTokenizerAdapter 创建适配器。
func NewTokenizerAdapter(inner Tokenizer, logger *zap.Logger) *TokenizerAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TokenizerAdapter{inner: inner, logger: logger}
}

// CountTokens 返回文本的 token 数。
// 底层 tokenizer 出错时回退到 len(text)/4 估算。
func (a *TokenizerAdapter) CountTokens(text string) int {
	count, err := a.inner.CountTokens(text)
	if err != nil {
		a.logger.Warn("tokenizer CountTokens failed, falling back to estimate",
			zap.Error(err))
		return len(text) / 4
	}
	return count
}

// Encode 将文本转换为 token ID 列表。
// 底层 tokenizer 出错时回退到伪 token ID 序列。
func (a *TokenizerAdapter) Encode(text string) []int {
	tokens, err := a.inner.Encode(text)
	if err != nil {
		a.logger.Warn("tokenizer Encode failed, falling back to estimate",
			zap.Error(err))
		result := make([]int, len(text)/4)
		for i := range result {
			result[i] = i
		}
		return result
	}
	return tokens
}

// 确保实现接口
var _ core.Tokenizer = (*TokenizerAdapter)(nil)

// NewTiktokenAdapter 创建一个基于 tiktoken 的 rag.Tokenizer 适配器。
// model 参数指定 tiktoken 模型（如 "gpt-4o", "gpt-4", "gpt-3.5-turbo"）。
func NewTiktokenAdapter(model string, logger *zap.Logger) (core.Tokenizer, error) {
	tok, err := NewTiktokenTokenizer(model)
	if err != nil {
		return nil, fmt.Errorf("create tiktoken tokenizer: %w", err)
	}
	return NewTokenizerAdapter(tok, logger), nil
}

// NewEstimatorAdapter 创建一个基于 EstimatorTokenizer 的 rag.Tokenizer 适配器。
// 比 SimpleTokenizer 更精确（CJK 感知），且不需要外部编码数据下载。
// model 参数仅用于标识，maxTokens 指定模型上下文长度（0 使用默认值 4096）。
func NewEstimatorAdapter(model string, maxTokens int, logger *zap.Logger) core.Tokenizer {
	est := NewEstimatorTokenizer(model, maxTokens)
	return NewTokenizerAdapter(est, logger)
}
