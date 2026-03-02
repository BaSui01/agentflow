package types

// RetrievalTrace defines minimal cross-layer trace fields for retrieval flows.
type RetrievalTrace struct {
	TraceID string `json:"trace_id,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

// RetrievalRecord defines the minimal cross-layer retrieval result contract.
type RetrievalRecord struct {
	DocID   string         `json:"doc_id"`
	Content string         `json:"content"`
	Source  string         `json:"source,omitempty"`
	Score   float64        `json:"score"`
	Trace   RetrievalTrace `json:"trace,omitempty"`
}

// RetrievalMetricsContract defines minimal retrieval observability fields.
type RetrievalMetricsContract struct {
	Trace              RetrievalTrace `json:"trace,omitempty"`
	RetrievalLatencyMS int64          `json:"retrieval_latency_ms"`
	RerankLatencyMS    int64          `json:"rerank_latency_ms"`
	TopK               int            `json:"topk"`
	HitCount           int            `json:"hit_count"`
	ContextTokens      int            `json:"context_tokens"`
}

// RAGEvalMetrics defines the shared RAG evaluation metric contract.
type RAGEvalMetrics struct {
	ContextRelevance float64 `json:"context_relevance"`
	Faithfulness     float64 `json:"faithfulness"`
	AnswerRelevancy  float64 `json:"answer_relevancy"`
	RecallAtK        float64 `json:"recall_at_k"`
	MRR              float64 `json:"mrr"`
}
