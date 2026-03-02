package evaluation

import (
	"context"
	"fmt"
	"math"

	"github.com/BaSui01/agentflow/types"
)

const (
	metadataKeyRAGEvalMetrics   = "rag_eval_metrics"
	metadataKeyRetrievedDocIDs  = "retrieved_doc_ids"
	metadataKeyAnswerGroundness = "answer_groundedness"
	contextKeyRelevantDocIDs    = "relevant_doc_ids"
)

// RecallAtKMetric computes recall@k for RAG evaluation.
// Priority:
// 1) direct value from types.RAGEvalMetrics
// 2) derive from relevant_doc_ids + retrieved_doc_ids
type RecallAtKMetric struct{}

func NewRecallAtKMetric() *RecallAtKMetric { return &RecallAtKMetric{} }

func (m *RecallAtKMetric) Name() string { return "recall_at_k" }

func (m *RecallAtKMetric) Compute(_ context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if metrics, ok := extractRAGEvalMetrics(output); ok {
		return clamp01(metrics.RecallAtK), nil
	}

	relevant := toStringSet(extractRelevantDocIDs(input))
	if len(relevant) == 0 {
		return 0, nil
	}
	retrieved := extractRetrievedDocIDs(output)
	if len(retrieved) == 0 {
		return 0, nil
	}

	hit := 0
	for _, id := range uniqueStrings(retrieved) {
		if _, ok := relevant[id]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(relevant)), nil
}

// MRRMetric computes Mean Reciprocal Rank for RAG retrieval.
// Priority:
// 1) direct value from types.RAGEvalMetrics
// 2) derive from relevant_doc_ids + retrieved_doc_ids
type MRRMetric struct{}

func NewMRRMetric() *MRRMetric { return &MRRMetric{} }

func (m *MRRMetric) Name() string { return "mrr" }

func (m *MRRMetric) Compute(_ context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if metrics, ok := extractRAGEvalMetrics(output); ok {
		return clamp01(metrics.MRR), nil
	}

	relevant := toStringSet(extractRelevantDocIDs(input))
	if len(relevant) == 0 {
		return 0, nil
	}
	retrieved := extractRetrievedDocIDs(output)
	for idx, id := range retrieved {
		if _, ok := relevant[id]; ok {
			return 1.0 / float64(idx+1), nil
		}
	}
	return 0, nil
}

// GroundednessMetric computes answer groundedness for RAG answers.
// Priority:
// 1) answer_groundedness metadata
// 2) types.RAGEvalMetrics.Faithfulness
type GroundednessMetric struct{}

func NewGroundednessMetric() *GroundednessMetric { return &GroundednessMetric{} }

func (m *GroundednessMetric) Name() string { return "groundedness" }

func (m *GroundednessMetric) Compute(_ context.Context, _ *EvalInput, output *EvalOutput) (float64, error) {
	if output == nil || output.Metadata == nil {
		return 0, nil
	}
	if v, ok := toFloat64(output.Metadata[metadataKeyAnswerGroundness]); ok {
		return clamp01(v), nil
	}
	if metrics, ok := extractRAGEvalMetrics(output); ok {
		return clamp01(metrics.Faithfulness), nil
	}
	return 0, nil
}

// WithRelevantDocIDs writes relevant document IDs into EvalInput context.
func (e *EvalInput) WithRelevantDocIDs(docIDs []string) *EvalInput {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[contextKeyRelevantDocIDs] = append([]string(nil), docIDs...)
	return e
}

// WithRetrievedDocIDs writes retrieved document IDs into EvalOutput metadata.
func (e *EvalOutput) WithRetrievedDocIDs(docIDs []string) *EvalOutput {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[metadataKeyRetrievedDocIDs] = append([]string(nil), docIDs...)
	return e
}

// WithAnswerGroundedness writes groundedness score into EvalOutput metadata.
func (e *EvalOutput) WithAnswerGroundedness(v float64) *EvalOutput {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[metadataKeyAnswerGroundness] = clamp01(v)
	return e
}

// WithRAGEvalMetrics writes types.RAGEvalMetrics contract into EvalOutput metadata.
func (e *EvalOutput) WithRAGEvalMetrics(metrics types.RAGEvalMetrics) *EvalOutput {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[metadataKeyRAGEvalMetrics] = metrics
	return e
}

func extractRAGEvalMetrics(output *EvalOutput) (types.RAGEvalMetrics, bool) {
	if output == nil || output.Metadata == nil {
		return types.RAGEvalMetrics{}, false
	}
	raw, ok := output.Metadata[metadataKeyRAGEvalMetrics]
	if !ok || raw == nil {
		return types.RAGEvalMetrics{}, false
	}
	switch v := raw.(type) {
	case types.RAGEvalMetrics:
		return v, true
	case *types.RAGEvalMetrics:
		if v == nil {
			return types.RAGEvalMetrics{}, false
		}
		return *v, true
	case map[string]any:
		return decodeRAGEvalMetricsMap(v)
	default:
		return types.RAGEvalMetrics{}, false
	}
}

func decodeRAGEvalMetricsMap(v map[string]any) (types.RAGEvalMetrics, bool) {
	recall, okRecall := toFloat64(v["recall_at_k"])
	mrr, okMRR := toFloat64(v["mrr"])
	faithfulness, okFaith := toFloat64(v["faithfulness"])
	if !okRecall && !okMRR && !okFaith {
		return types.RAGEvalMetrics{}, false
	}
	out := types.RAGEvalMetrics{}
	if okRecall {
		out.RecallAtK = recall
	}
	if okMRR {
		out.MRR = mrr
	}
	if okFaith {
		out.Faithfulness = faithfulness
	}
	return out, true
}

func extractRelevantDocIDs(input *EvalInput) []string {
	if input == nil || input.Context == nil {
		return nil
	}
	return toStringSlice(input.Context[contextKeyRelevantDocIDs])
}

func extractRetrievedDocIDs(output *EvalOutput) []string {
	if output == nil || output.Metadata == nil {
		return nil
	}
	return toStringSlice(output.Metadata[metadataKeyRetrievedDocIDs])
}

func toStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok || s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func toStringSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range uniqueStrings(items) {
		if item == "" {
			continue
		}
		out[item] = struct{}{}
	}
	return out
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func validateRAGMetricsForDebug(output *EvalOutput) error {
	if output == nil || output.Metadata == nil {
		return nil
	}
	if _, ok := output.Metadata[metadataKeyRAGEvalMetrics]; ok {
		if _, ok := extractRAGEvalMetrics(output); !ok {
			return fmt.Errorf("invalid %s payload", metadataKeyRAGEvalMetrics)
		}
	}
	return nil
}
