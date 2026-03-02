package core

import "github.com/BaSui01/agentflow/types"

// BuildSharedRetrievalRecords maps rag retrieval results into types-level minimal shared records.
func BuildSharedRetrievalRecords(results []RetrievalResult, trace types.RetrievalTrace) []types.RetrievalRecord {
	out := make([]types.RetrievalRecord, 0, len(results))
	for _, r := range results {
		source := ""
		if r.Document.Metadata != nil {
			if v, ok := r.Document.Metadata["source"].(string); ok {
				source = v
			}
		}
		out = append(out, types.RetrievalRecord{
			DocID:   r.Document.ID,
			Content: r.Document.Content,
			Source:  source,
			Score:   r.FinalScore,
			Trace:   trace,
		})
	}
	return out
}

// BuildSharedEvalMetrics maps rag eval metrics into types-level eval contract.
func BuildSharedEvalMetrics(m EvalMetrics) types.RAGEvalMetrics {
	return types.RAGEvalMetrics{
		ContextRelevance: m.ContextRelevance,
		Faithfulness:     m.Faithfulness,
		AnswerRelevancy:  m.AnswerRelevancy,
		RecallAtK:        m.RecallAtK,
		MRR:              m.MRR,
	}
}
