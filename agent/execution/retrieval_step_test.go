package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRetriever struct {
	records []types.RetrievalRecord
	err     error
}

func (m *mockRetriever) Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.records, nil
}

type mockReranker struct {
	records []types.RetrievalRecord
	err     error
}

func (m *mockReranker) Rerank(ctx context.Context, query string, records []types.RetrievalRecord) ([]types.RetrievalRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.records != nil {
		return m.records, nil
	}
	return records, nil
}

func TestRetrievalStep_Execute_Success(t *testing.T) {
	step := NewRetrievalStep(
		&mockRetriever{
			records: []types.RetrievalRecord{
				{DocID: "d1", Content: "hello world", Score: 0.9},
				{DocID: "d2", Content: "another context block", Score: 0.8},
			},
		},
		&mockReranker{
			records: []types.RetrievalRecord{
				{DocID: "d2", Content: "another context block", Score: 0.95},
				{DocID: "d1", Content: "hello world", Score: 0.9},
			},
		},
		nil,
	)

	ctx := context.Background()
	ctx = types.WithTraceID(ctx, "trace-1")
	ctx = types.WithRunID(ctx, "run-1")
	ctx = types.WithSpanID(ctx, "span-1")

	result, err := step.Execute(ctx, RetrievalStepRequest{Query: "hello", TopK: 2})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Records, 2)
	assert.Equal(t, "d2", result.Records[0].DocID)
	assert.Equal(t, 2, result.Metrics.TopK)
	assert.Equal(t, 2, result.Metrics.HitCount)
	assert.Equal(t, "trace-1", result.Metrics.Trace.TraceID)
	assert.Equal(t, "run-1", result.Metrics.Trace.RunID)
	assert.Equal(t, "span-1", result.Metrics.Trace.SpanID)
	assert.GreaterOrEqual(t, result.Metrics.ContextTokens, 1)
}

func TestRetrievalStep_Execute_InvalidQuery(t *testing.T) {
	step := NewRetrievalStep(&mockRetriever{}, nil, nil)
	_, err := step.Execute(context.Background(), RetrievalStepRequest{Query: "   "})
	require.Error(t, err)
}

func TestRetrievalStep_Execute_RetrieveError(t *testing.T) {
	step := NewRetrievalStep(&mockRetriever{err: errors.New("retrieve failed")}, nil, nil)
	_, err := step.Execute(context.Background(), RetrievalStepRequest{Query: "hello"})
	require.Error(t, err)
}

func TestRetrievalStep_Execute_RerankError(t *testing.T) {
	step := NewRetrievalStep(
		&mockRetriever{records: []types.RetrievalRecord{{DocID: "d1", Content: "hello", Score: 0.9}}},
		&mockReranker{err: errors.New("rerank failed")},
		nil,
	)
	_, err := step.Execute(context.Background(), RetrievalStepRequest{Query: "hello"})
	require.Error(t, err)
}
