package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSimpleRerankerRanksByQueryOverlap(t *testing.T) {
	reranker := NewSimpleReranker(zap.NewNop())
	results := []RetrievalResult{
		{Document: Document{ID: "low", Content: "unrelated content"}, FinalScore: 0.2},
		{Document: Document{ID: "high", Content: "agentflow retrieval retrieval query"}, FinalScore: 0.2},
	}

	reranked, err := reranker.Rerank(context.Background(), "retrieval query", results)
	require.NoError(t, err)

	assert.Equal(t, "high", reranked[0].Document.ID)
	assert.Greater(t, reranked[0].RerankScore, reranked[1].RerankScore)
	assert.Equal(t, 1.0, reranker.proximityScore([]string{"single"}, []string{"single"}))
	assert.Equal(t, []string{"a", "b"}, tokenize("a\n\tb"))
	assert.Equal(t, 3, abs(-3))
}

func TestCrossEncoderRerankerScoresInBatchesAndLimitsCandidates(t *testing.T) {
	provider := &fakeCrossEncoderProvider{scores: []float64{2, 1, 0}}
	reranker := NewCrossEncoderReranker(provider, CrossEncoderConfig{BatchSize: 2, MaxLength: 4, ScoreWeight: 1, OriginalWeight: 0}, zap.NewNop())
	results := []RetrievalResult{
		{Document: Document{ID: "a", Content: "aaaa bbbb cccc dddd eeee"}, FinalScore: 0.1},
		{Document: Document{ID: "b", Content: "bbbb"}, FinalScore: 0.1},
		{Document: Document{ID: "c", Content: "cccc"}, FinalScore: 0.1},
	}

	reranked, err := reranker.Rerank(context.Background(), "query", results)
	require.NoError(t, err)

	assert.Equal(t, "a", reranked[0].Document.ID)
	assert.Equal(t, 2, provider.calls)
	require.NotEmpty(t, provider.seen)
	assert.LessOrEqual(t, len(provider.seen[0][0].Document), 16)
}

func TestCrossEncoderRerankerReturnsProviderError(t *testing.T) {
	reranker := NewCrossEncoderReranker(&fakeCrossEncoderProvider{err: errors.New("score failed")}, CrossEncoderConfig{BatchSize: 1, MaxLength: 8}, zap.NewNop())
	_, err := reranker.Rerank(context.Background(), "query", []RetrievalResult{{Document: Document{ID: "a", Content: "doc"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to score pairs")
}

func TestLLMRerankerUsesProviderAndFallsBackOnError(t *testing.T) {
	provider := &fakeLLMRerankerProvider{scores: map[string]float64{"good": 9}, errFor: "bad"}
	reranker := NewLLMReranker(provider, LLMRerankerConfig{MaxCandidates: 2}, zap.NewNop())
	results := []RetrievalResult{
		{Document: Document{ID: "bad", Content: "bad"}, FinalScore: 0.6},
		{Document: Document{ID: "good", Content: "good"}, FinalScore: 0.1},
		{Document: Document{ID: "ignored", Content: "ignored"}, FinalScore: 1.0},
	}

	reranked, err := reranker.Rerank(context.Background(), "query", results)
	require.NoError(t, err)
	require.Len(t, reranked, 2)
	assert.Equal(t, "good", reranked[0].Document.ID)
	assert.Equal(t, 2, provider.calls)
}

type fakeCrossEncoderProvider struct {
	scores []float64
	err    error
	calls  int
	seen   [][]QueryDocPair
}

func (p *fakeCrossEncoderProvider) Score(_ context.Context, pairs []QueryDocPair) ([]float64, error) {
	p.calls++
	p.seen = append(p.seen, append([]QueryDocPair(nil), pairs...))
	if p.err != nil {
		return nil, p.err
	}
	out := make([]float64, len(pairs))
	for i := range pairs {
		out[i] = p.scores[0]
		p.scores = p.scores[1:]
	}
	return out, nil
}

type fakeLLMRerankerProvider struct {
	scores map[string]float64
	errFor string
	calls  int
}

func (p *fakeLLMRerankerProvider) ScoreRelevance(_ context.Context, _, document string) (float64, error) {
	p.calls++
	if document == p.errFor {
		return 0, errors.New("boom")
	}
	return p.scores[document], nil
}
