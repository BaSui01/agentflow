package evaluation

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecallAtKMetric_FromContract(t *testing.T) {
	m := NewRecallAtKMetric()
	out := NewEvalOutput("answer").WithRAGEvalMetrics(types.RAGEvalMetrics{RecallAtK: 0.75})
	score, err := m.Compute(context.Background(), nil, out)
	require.NoError(t, err)
	assert.Equal(t, 0.75, score)
}

func TestRecallAtKMetric_FromDocSets(t *testing.T) {
	m := NewRecallAtKMetric()
	in := NewEvalInput("q").WithRelevantDocIDs([]string{"d1", "d2", "d3"})
	out := NewEvalOutput("a").WithRetrievedDocIDs([]string{"d3", "d9", "d1"})
	score, err := m.Compute(context.Background(), in, out)
	require.NoError(t, err)
	assert.InDelta(t, 2.0/3.0, score, 1e-9)
}

func TestMRRMetric_FromContract(t *testing.T) {
	m := NewMRRMetric()
	out := NewEvalOutput("answer").WithRAGEvalMetrics(types.RAGEvalMetrics{MRR: 0.5})
	score, err := m.Compute(context.Background(), nil, out)
	require.NoError(t, err)
	assert.Equal(t, 0.5, score)
}

func TestMRRMetric_FromDocSets(t *testing.T) {
	m := NewMRRMetric()
	in := NewEvalInput("q").WithRelevantDocIDs([]string{"d2", "d8"})
	out := NewEvalOutput("a").WithRetrievedDocIDs([]string{"d1", "d2", "d3"})
	score, err := m.Compute(context.Background(), in, out)
	require.NoError(t, err)
	assert.Equal(t, 0.5, score)
}

func TestGroundednessMetric(t *testing.T) {
	m := NewGroundednessMetric()
	out := NewEvalOutput("answer").WithAnswerGroundedness(0.9)
	score, err := m.Compute(context.Background(), nil, out)
	require.NoError(t, err)
	assert.Equal(t, 0.9, score)

	out = NewEvalOutput("answer").WithRAGEvalMetrics(types.RAGEvalMetrics{Faithfulness: 0.66})
	score, err = m.Compute(context.Background(), nil, out)
	require.NoError(t, err)
	assert.Equal(t, 0.66, score)
}

func TestRAGMetricBuilders(t *testing.T) {
	input := NewEvalInput("q").WithRelevantDocIDs([]string{"a", "b"})
	require.NotNil(t, input.Context)
	assert.Equal(t, []string{"a", "b"}, input.Context[contextKeyRelevantDocIDs])

	output := NewEvalOutput("r").
		WithRetrievedDocIDs([]string{"c"}).
		WithAnswerGroundedness(0.8).
		WithRAGEvalMetrics(types.RAGEvalMetrics{RecallAtK: 0.2})
	require.NotNil(t, output.Metadata)
	assert.Equal(t, []string{"c"}, output.Metadata[metadataKeyRetrievedDocIDs])
	assert.Equal(t, 0.8, output.Metadata[metadataKeyAnswerGroundness])
	_, ok := output.Metadata[metadataKeyRAGEvalMetrics].(types.RAGEvalMetrics)
	assert.True(t, ok)
}
