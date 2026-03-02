package core

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
)

func TestBuildSharedRetrievalRecords(t *testing.T) {
	trace := types.RetrievalTrace{TraceID: "t1", RunID: "r1", SpanID: "s1"}
	in := []RetrievalResult{
		{
			Document: Document{
				ID:      "doc-1",
				Content: "hello",
				Metadata: map[string]any{
					"source": "kb",
				},
			},
			FinalScore: 0.98,
		},
	}
	out := BuildSharedRetrievalRecords(in, trace)
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	if out[0].DocID != "doc-1" || out[0].Source != "kb" || out[0].Score != 0.98 {
		t.Fatalf("unexpected mapped record: %+v", out[0])
	}
	if out[0].Trace.TraceID != "t1" || out[0].Trace.RunID != "r1" || out[0].Trace.SpanID != "s1" {
		t.Fatalf("unexpected trace mapping: %+v", out[0].Trace)
	}
}

func TestBuildSharedEvalMetrics(t *testing.T) {
	in := EvalMetrics{
		ContextRelevance: 0.9,
		Faithfulness:     0.8,
		AnswerRelevancy:  0.85,
		RecallAtK:        0.7,
		MRR:              0.6,
	}
	out := BuildSharedEvalMetrics(in)
	if out.ContextRelevance != in.ContextRelevance ||
		out.Faithfulness != in.Faithfulness ||
		out.AnswerRelevancy != in.AnswerRelevancy ||
		out.RecallAtK != in.RecallAtK ||
		out.MRR != in.MRR {
		t.Fatalf("unexpected eval mapping: %+v", out)
	}
}
