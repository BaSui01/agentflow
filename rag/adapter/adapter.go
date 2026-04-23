// Package adapter 提供 RAG 层到 agent/workflow 层的适配器
package adapter

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/rag/retrieval"
	rag "github.com/BaSui01/agentflow/rag/runtime"
	"github.com/BaSui01/agentflow/types"
)

// RetrievalToolAdapter 将 RAG Pipeline 适配为 agent 层可用的检索接口
type RetrievalToolAdapter struct {
	pipeline   *retrieval.Pipeline
	embedder   rag.EmbeddingProvider
	collection string
}

// NewRetrievalToolAdapter 从 Pipeline 创建适配器
func NewRetrievalToolAdapter(pipeline *retrieval.Pipeline, embedder rag.EmbeddingProvider, collection string) *RetrievalToolAdapter {
	return &RetrievalToolAdapter{
		pipeline:   pipeline,
		embedder:   embedder,
		collection: collection,
	}
}

// Retrieve 实现 agent 层需要的检索接口，返回 types.RetrievalRecord
func (a *RetrievalToolAdapter) Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
	if a.pipeline == nil {
		return nil, fmt.Errorf("pipeline is nil")
	}

	// 1. 生成 query embedding（如果有 embedder）
	var queryEmbedding []float64
	if a.embedder != nil {
		embedding, err := a.embedder.EmbedQuery(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryEmbedding = embedding
	}

	// 2. 调用 pipeline.Execute
	input := retrieval.PipelineInput{
		Query:          query,
		QueryEmbedding: queryEmbedding,
	}

	output, err := a.pipeline.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("execute pipeline: %w", err)
	}

	// 3. 转换 rag.RetrievalResult 为 types.RetrievalRecord
	trace := types.RetrievalTrace{
		TraceID: "",
		RunID:   "",
		SpanID:  "",
	}
	// 从 context 中提取 trace 信息
	if tid, ok := types.TraceID(ctx); ok {
		trace.TraceID = tid
	}
	if rid, ok := types.RunID(ctx); ok {
		trace.RunID = rid
	}
	if sid, ok := types.SpanID(ctx); ok {
		trace.SpanID = sid
	}

	records := core.BuildSharedRetrievalRecords(output.Results, trace)

	// 4. 根据 topK 裁剪结果
	if topK > 0 && len(records) > topK {
		records = records[:topK]
	}

	return records, nil
}

// Pipeline 返回底层的 Pipeline 实例
func (a *RetrievalToolAdapter) Pipeline() *retrieval.Pipeline {
	return a.pipeline
}

// Embedder 返回 embedding provider
func (a *RetrievalToolAdapter) Embedder() rag.EmbeddingProvider {
	return a.embedder
}

// Collection 返回集合名称
func (a *RetrievalToolAdapter) Collection() string {
	return a.collection
}

// =============================================================================
// Hybrid Retriever Adapter (for workflow/steps)
// =============================================================================

// HybridRetrieverAdapter 将 HybridRetriever 适配为 workflow/steps 需要的接口
type HybridRetrieverAdapter struct {
	retriever *rag.HybridRetriever
}

// NewHybridRetrieverAdapter 创建适配器
func NewHybridRetrieverAdapter(retriever *rag.HybridRetriever) *HybridRetrieverAdapter {
	return &HybridRetrieverAdapter{retriever: retriever}
}

// Retrieve 实现 workflow/steps.HybridRetriever 接口
func (a *HybridRetrieverAdapter) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]types.RetrievalRecord, error) {
	if a.retriever == nil {
		return nil, fmt.Errorf("retriever is nil")
	}

	// 调用底层 retriever
	results, err := a.retriever.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}

	// 构建 trace
	trace := types.RetrievalTrace{}
	if tid, ok := types.TraceID(ctx); ok {
		trace.TraceID = tid
	}
	if rid, ok := types.RunID(ctx); ok {
		trace.RunID = rid
	}
	if sid, ok := types.SpanID(ctx); ok {
		trace.SpanID = sid
	}

	// 转换为 types.RetrievalRecord
	records := make([]types.RetrievalRecord, 0, len(results))
	for _, r := range results {
		source := ""
		if r.Document.Metadata != nil {
			if v, ok := r.Document.Metadata["source"].(string); ok {
				source = v
			}
		}
		records = append(records, types.RetrievalRecord{
			DocID:   r.Document.ID,
			Content: r.Document.Content,
			Source:  source,
			Score:   r.FinalScore,
			Trace:   trace,
		})
	}

	return records, nil
}

// Retriever 返回底层的 HybridRetriever 实例
func (a *HybridRetrieverAdapter) Retriever() *rag.HybridRetriever {
	return a.retriever
}
