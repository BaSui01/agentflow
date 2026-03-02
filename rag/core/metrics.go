package core

import "time"

// RetrievalMetrics 统一检索观测字段。
type RetrievalMetrics struct {
	TraceID          string        `json:"trace_id"`
	RunID            string        `json:"run_id"`
	RetrievalLatency time.Duration `json:"retrieval_latency_ms"`
	RerankLatency    time.Duration `json:"rerank_latency_ms"`
	TopK             int           `json:"topk"`
	HitCount         int           `json:"hit_count"`
	ContextTokens    int           `json:"context_tokens"`
	Strategy         string        `json:"strategy"`
}

// EvalMetrics RAG 评估指标定义。
type EvalMetrics struct {
	ContextRelevance float64 `json:"context_relevance"`
	Faithfulness     float64 `json:"faithfulness"`
	AnswerRelevancy  float64 `json:"answer_relevancy"`
	RecallAtK        float64 `json:"recall_at_k"`
	MRR              float64 `json:"mrr"`
}
