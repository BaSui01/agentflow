package runtime

import (
	pkgtokenizer "github.com/BaSui01/agentflow/pkg/tokenizer"
	"go.uber.org/zap"
)

// SharedTokenizerAdapter 将共享 tokenizer contract 适配为 rag.Tokenizer 接口。
// 当底层 tokenizer 返回 error 时，回退到字符估算并记录警告日志。
type SharedTokenizerAdapter struct {
	inner  pkgtokenizer.Tokenizer
	logger *zap.Logger
}

// NewSharedTokenizerAdapter 创建共享 tokenizer contract 到 RAG tokenizer 的适配器。
func NewSharedTokenizerAdapter(inner pkgtokenizer.Tokenizer, logger *zap.Logger) *SharedTokenizerAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SharedTokenizerAdapter{inner: inner, logger: logger}
}

// CountTokens 返回文本的 token 数。
// 底层 tokenizer 出错时回退到 len(text)/4 估算。
func (a *SharedTokenizerAdapter) CountTokens(text string) int {
	count, err := a.inner.CountTokens(text)
	if err != nil {
		a.logger.Warn("tokenizer CountTokens failed, falling back to estimate", zap.Error(err))
		return pkgtokenizer.NewRAGAdapter(a.inner).CountTokens(text)
	}
	return count
}

// Encode 将文本转换为 token ID 列表。
// 底层 tokenizer 出错时回退到伪 token ID 序列。
func (a *SharedTokenizerAdapter) Encode(text string) []int {
	tokens, err := a.inner.Encode(text)
	if err != nil {
		a.logger.Warn("tokenizer Encode failed, falling back to estimate", zap.Error(err))
		return pkgtokenizer.NewRAGAdapter(a.inner).Encode(text)
	}
	return tokens
}
