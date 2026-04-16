package types

// RerankDocument 表示待重排序的文档（RerankResult 的子结构）。
type RerankDocument struct {
	Text  string `json:"text"`
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// RerankResult 表示单个重排序结果（从 LLM 层下沉到 types 层）。
type RerankResult struct {
	Index          int     `json:"index"`           // 输入中的原始索引
	RelevanceScore float64 `json:"relevance_score"` // 0-1 归一化分数
	Document       string  `json:"document,omitempty"`
}

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

type ToolStateSnapshot struct {
	ToolName   string            `json:"tool_name"`
	Summary    string            `json:"summary"`
	ArtifactID string            `json:"artifact_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
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
