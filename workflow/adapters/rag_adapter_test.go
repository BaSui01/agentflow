package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/rag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRAGRetriever struct {
	results []rag.RetrievalResult
	err     error
	lastQ   string
}

func (m *mockRAGRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	m.lastQ = query
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestRAGStep_Execute(t *testing.T) {
	r := &mockRAGRetriever{results: []rag.RetrievalResult{{FinalScore: 0.9}}}
	step := NewRAGStep("rag-step", "", r)

	out, err := step.Execute(context.Background(), "what is agentflow")
	require.NoError(t, err)
	assert.Equal(t, "rag-step", step.Name())
	assert.Equal(t, "what is agentflow", r.lastQ)

	got, ok := out.([]rag.RetrievalResult)
	require.True(t, ok)
	assert.Len(t, got, 1)
}

func TestRAGStep_ErrorCases(t *testing.T) {
	step := NewRAGStep("", "", nil)
	_, err := step.Execute(context.Background(), "q")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retriever is nil")

	r := &mockRAGRetriever{}
	step = NewRAGStep("", "", r)
	_, err = step.Execute(context.Background(), map[string]any{"x": "y"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query is empty")

	r = &mockRAGRetriever{err: errors.New("upstream failed")}
	step = NewRAGStep("", "q", r)
	_, err = step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retrieve failed")
}
