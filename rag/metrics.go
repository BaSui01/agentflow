package rag

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// collectRetrievalMetrics 在 RAG 检索出口统一采集度量。
// 使用 core.RetrievalMetrics（通过 facade 别名 RetrievalMetrics）。
func collectRetrievalMetrics(
	ctx context.Context,
	retrievalStart time.Time,
	rerankDuration time.Duration,
	topK int,
	hitCount int,
	contextTokens int,
) RetrievalMetrics {
	m := RetrievalMetrics{
		RetrievalLatency: time.Since(retrievalStart),
		RerankLatency:    rerankDuration,
		TopK:             topK,
		HitCount:         hitCount,
		ContextTokens:    contextTokens,
	}
	if traceID, ok := types.TraceID(ctx); ok {
		m.TraceID = traceID
	}
	if runID, ok := types.RunID(ctx); ok {
		m.RunID = runID
	}
	return m
}

// estimateTokens 粗略估算文本 token 数（英文约 4 字符/token）。
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4
}
