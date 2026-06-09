package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestQueryTransformerTransformWithRulesExtractsIntentKeywordsEntities(t *testing.T) {
	cfg := DefaultQueryTransformConfig()
	cfg.UseLLM = false
	cfg.EnableCache = false
	transformer := NewQueryTransformer(cfg, nil, zap.NewNop())

	result, err := transformer.Transform(context.Background(), "compare Go and Rust concurrency?")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, TransformDecomposition, result.Type)
	assert.Equal(t, IntentComparison, result.Intent)
	assert.Equal(t, "Compare go and rust concurrency", result.Transformed)
	assert.Contains(t, result.Keywords, "compare")
	assert.Contains(t, result.Entities, "Go")
	assert.Contains(t, result.Entities, "Rust")
	assert.NotEmpty(t, result.SubQueries)
	assert.Equal(t, 0.8, result.Metadata["intent_confidence"])
}

func TestQueryTransformerExpandWithRulesAndMetadata(t *testing.T) {
	cfg := DefaultQueryTransformConfig()
	cfg.UseLLM = false
	cfg.EnableCache = false
	cfg.MaxExpansions = 2
	transformer := NewQueryTransformer(cfg, nil, zap.NewNop())

	result, err := transformer.ExpandWithMetadata(context.Background(), "explain best cache example")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "explain best cache example", result.Original)
	assert.Len(t, result.Expansions, 3)
	assert.Equal(t, IntentExplanation, result.Intent)
	assert.Contains(t, result.Keywords, "explain")
	assert.Contains(t, result.Keywords, "cache")
}

func TestQueryTransformerTransformUsesCache(t *testing.T) {
	cfg := DefaultQueryTransformConfig()
	cfg.UseLLM = false
	cfg.EnableCache = true
	transformer := NewQueryTransformer(cfg, nil, zap.NewNop())

	first, err := transformer.Transform(context.Background(), "what is semantic cache")
	require.NoError(t, err)
	second, err := transformer.Transform(context.Background(), "what is semantic cache")
	require.NoError(t, err)
	assert.Same(t, first, second)
}

func TestQueryTransformerHyDEAndStepBackRequireLLM(t *testing.T) {
	cfg := DefaultQueryTransformConfig()
	cfg.UseLLM = false
	cfg.EnableCache = false
	cfg.EnableHyDE = true
	cfg.EnableStepBack = true
	transformer := NewQueryTransformer(cfg, nil, zap.NewNop())

	result, err := transformer.Transform(context.Background(), "what is retrieval augmented generation")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotContains(t, result.Metadata, "hyde_document")
	assert.NotContains(t, result.Metadata, "step_back_query")
}

func TestTransformedQueryJSONRoundTrip(t *testing.T) {
	query := &TransformedQuery{
		Original:    "original",
		Transformed: "rewritten",
		Type:        TransformRewrite,
		Intent:      IntentFactual,
		Confidence:  0.7,
		SubQueries:  []string{"one", "two"},
		Keywords:    []string{"one"},
		Entities:    []string{"Entity"},
	}

	data, err := query.ToJSON()
	require.NoError(t, err)
	var decoded TransformedQuery
	require.NoError(t, decoded.FromJSON(data))
	assert.Equal(t, query.Original, decoded.Original)
	assert.Equal(t, query.Transformed, decoded.Transformed)
	assert.Equal(t, query.Type, decoded.Type)
	assert.Equal(t, query.Intent, decoded.Intent)
	assert.Equal(t, query.SubQueries, decoded.SubQueries)
}
