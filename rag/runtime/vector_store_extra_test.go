package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInMemoryVectorStoreCRUDPaginationAndErrors(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore(zap.NewNop())

	require.Error(t, store.AddDocuments(ctx, []Document{{ID: "missing-embedding"}}))
	require.NoError(t, store.AddDocuments(ctx, []Document{
		{ID: "a", Content: "alpha", Embedding: []float64{1, 0}},
		{ID: "b", Content: "beta", Embedding: []float64{0, 1}},
		{ID: "c", Content: "gamma", Embedding: []float64{0.5, 0.5}},
	}))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	results, err := store.Search(ctx, []float64{1, 0}, 2)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "a", results[0].Document.ID)
	assert.GreaterOrEqual(t, results[0].Score, results[1].Score)

	ids, err := store.ListDocumentIDs(ctx, 2, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "c"}, ids)
	ids, err = store.ListDocumentIDs(ctx, 2, 99)
	require.NoError(t, err)
	assert.Empty(t, ids)

	require.NoError(t, store.UpdateDocument(ctx, Document{ID: "b", Content: "updated", Embedding: []float64{0, 2}}))
	assert.Error(t, store.UpdateDocument(ctx, Document{ID: "missing", Embedding: []float64{1}}))

	require.NoError(t, store.DeleteDocuments(ctx, []string{"a", "missing"}))
	count, err = store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	require.NoError(t, store.ClearAll(ctx))
	count, err = store.Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestVectorConversionHelpersAndCosineEdgeCases(t *testing.T) {
	assert.Nil(t, Float32ToFloat64(nil))
	assert.Nil(t, Float64ToFloat32(nil))
	assert.Equal(t, []float64{1.5, -2}, Float32ToFloat64([]float32{1.5, -2}))
	assert.Equal(t, []float32{1.5, -2}, Float64ToFloat32([]float64{1.5, -2}))

	assert.Equal(t, 0.0, cosineSimilarity([]float64{1}, []float64{1, 2}))
	assert.Equal(t, 0.0, cosineSimilarity([]float64{0, 0}, []float64{1, 0}))
	assert.InDelta(t, 1.0, cosineSimilarity([]float64{1, 0}, []float64{1, 0}), 1e-9)
}
