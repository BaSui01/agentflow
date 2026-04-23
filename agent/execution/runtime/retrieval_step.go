package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Retriever defines the retrieval capability required by RetrievalStep.
type Retriever interface {
	Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error)
}

// Reranker defines an optional rerank stage for retrieval results.
type Reranker interface {
	Rerank(ctx context.Context, query string, records []types.RetrievalRecord) ([]types.RetrievalRecord, error)
}

// RetrievalStepRequest represents retrieval step input.
type RetrievalStepRequest struct {
	Query string
	TopK  int
}

// RetrievalStepResult carries retrieval output and observability metrics.
type RetrievalStepResult struct {
	Records            []types.RetrievalRecord
	Metrics            types.RetrievalMetricsContract
	Eval               types.RAGEvalMetrics
	AnswerGroundedness float64 `json:"answer_groundedness"`
}

// RetrievalStep makes retrieval a standard execution step.
type RetrievalStep struct {
	retriever Retriever
	reranker  Reranker
	logger    *zap.Logger
}

// NewRetrievalStep creates a retrieval step with optional reranker.
func NewRetrievalStep(retriever Retriever, reranker Reranker, logger *zap.Logger) *RetrievalStep {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetrievalStep{
		retriever: retriever,
		reranker:  reranker,
		logger:    logger,
	}
}

// Execute runs retrieval + optional rerank and returns standardized metrics.
func (s *RetrievalStep) Execute(ctx context.Context, req RetrievalStepRequest) (*RetrievalStepResult, error) {
	if s == nil || s.retriever == nil {
		return nil, types.NewInvalidRequestError("retriever is not configured")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, types.NewInvalidRequestError("query is required")
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	retrievalStart := time.Now()
	records, err := s.retriever.Retrieve(ctx, query, topK)
	retrievalLatency := time.Since(retrievalStart).Milliseconds()
	if err != nil {
		return nil, err
	}

	rerankLatency := int64(0)
	if s.reranker != nil {
		rerankStart := time.Now()
		records, err = s.reranker.Rerank(ctx, query, records)
		rerankLatency = time.Since(rerankStart).Milliseconds()
		if err != nil {
			return nil, err
		}
	}

	metrics := types.RetrievalMetricsContract{
		Trace: types.RetrievalTrace{
			TraceID: mustContext(types.TraceID(ctx)),
			RunID:   mustContext(types.RunID(ctx)),
			SpanID:  mustContext(types.SpanID(ctx)),
		},
		RetrievalLatencyMS: retrievalLatency,
		RerankLatencyMS:    rerankLatency,
		TopK:               topK,
		HitCount:           len(records),
		ContextTokens:      estimateContextTokens(records),
	}

	// Keep eval metrics in the result contract so upper layers can incrementally fill them.
	result := &RetrievalStepResult{
		Records: records,
		Metrics: metrics,
		Eval: types.RAGEvalMetrics{
			RecallAtK: 0,
			MRR:       0,
		},
		AnswerGroundedness: 0,
	}
	s.logger.Debug("retrieval step completed",
		zap.Int("topk", topK),
		zap.Int("hits", len(records)),
		zap.Int64("retrieval_latency_ms", retrievalLatency),
		zap.Int64("rerank_latency_ms", rerankLatency),
		zap.Int("context_tokens", result.Metrics.ContextTokens),
	)
	return result, nil
}

func estimateContextTokens(records []types.RetrievalRecord) int {
	total := 0
	for _, rec := range records {
		total += len(strings.Fields(rec.Content))
	}
	return total
}

func mustContext(value string, ok bool) string {
	if !ok {
		return ""
	}
	return value
}
