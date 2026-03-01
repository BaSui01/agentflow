package rag

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// SimpleContextProvider
// ============================================================

func TestSimpleContextProvider_GenerateContext_WithTitleAndSection(t *testing.T) {
	t.Parallel()
	p := NewSimpleContextProvider(zap.NewNop())
	ctx := context.Background()

	doc := Document{
		ID:      "doc1",
		Content: "full content",
		Metadata: map[string]any{
			"title":   "My Document",
			"section": "Introduction",
		},
	}
	result, err := p.GenerateContext(ctx, doc, "This is a chunk of text about something important.")
	require.NoError(t, err)
	assert.Contains(t, result, "My Document")
	assert.Contains(t, result, "Introduction")
	assert.Contains(t, result, "covering:")
}

func TestSimpleContextProvider_GenerateContext_NoMetadata(t *testing.T) {
	t.Parallel()
	p := NewSimpleContextProvider(zap.NewNop())
	ctx := context.Background()

	doc := Document{ID: "doc2", Content: "content"}
	result, err := p.GenerateContext(ctx, doc, "short")
	require.NoError(t, err)
	// With no title/section and short chunk, should still produce something
	assert.NotEmpty(t, result)
}

func TestSimpleContextProvider_GenerateContext_EmptyChunk(t *testing.T) {
	t.Parallel()
	p := NewSimpleContextProvider(zap.NewNop())
	ctx := context.Background()

	doc := Document{ID: "doc3", Content: "content"}
	result, err := p.GenerateContext(ctx, doc, "")
	require.NoError(t, err)
	assert.Equal(t, "General content chunk", result)
}

func TestSimpleContextProvider_GenerateContext_CancelledContext(t *testing.T) {
	t.Parallel()
	p := NewSimpleContextProvider(zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	doc := Document{ID: "doc4", Content: "content"}
	_, err := p.GenerateContext(ctx, doc, "chunk")
	require.Error(t, err)
}

func TestSimpleContextProvider_GenerateContext_CachesResult(t *testing.T) {
	t.Parallel()
	p := NewSimpleContextProvider(zap.NewNop())
	ctx := context.Background()

	doc := Document{
		ID:       "doc5",
		Content:  "content",
		Metadata: map[string]any{"title": "Cached Doc"},
	}
	chunk := "some chunk text"

	r1, err := p.GenerateContext(ctx, doc, chunk)
	require.NoError(t, err)

	r2, err := p.GenerateContext(ctx, doc, chunk)
	require.NoError(t, err)
	assert.Equal(t, r1, r2)
}

func TestSimpleContextProvider_NilLogger(t *testing.T) {
	t.Parallel()
	p := NewSimpleContextProvider(nil)
	assert.NotNil(t, p)
	assert.NotNil(t, p.logger)
}

// ============================================================
// truncateText
// ============================================================

func TestTruncateText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		maxLen int
		check  func(t *testing.T, result string)
	}{
		{
			name:   "short text unchanged",
			input:  "hello world",
			maxLen: 50,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "hello world", result)
			},
		},
		{
			name:   "long text truncated with ellipsis",
			input:  "the quick brown fox jumps over the lazy dog and more words follow",
			maxLen: 30,
			check: func(t *testing.T, result string) {
				assert.True(t, len(result) <= 34) // 30 + "..."
				assert.Contains(t, result, "...")
			},
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "", result)
			},
		},
		{
			name:   "whitespace only",
			input:  "   ",
			maxLen: 10,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "", result)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := truncateText(tt.input, tt.maxLen)
			tt.check(t, result)
		})
	}
}

// ============================================================
// hashString
// ============================================================

func TestHashString_Deterministic(t *testing.T) {
	t.Parallel()
	h1 := hashString("hello")
	h2 := hashString("hello")
	assert.Equal(t, h1, h2)
}

func TestHashString_DifferentInputs(t *testing.T) {
	t.Parallel()
	h1 := hashString("hello")
	h2 := hashString("world")
	assert.NotEqual(t, h1, h2)
}

// ============================================================
// LLMContextProvider
// ============================================================

func TestLLMContextProvider_GenerateContext_Success(t *testing.T) {
	t.Parallel()
	provider := NewLLMContextProvider(
		func(ctx context.Context, prompt string) (string, error) {
			return "  generated context  ", nil
		},
		zap.NewNop(),
	)

	doc := Document{
		ID:      "d1",
		Content: "full doc content",
		Metadata: map[string]any{
			"title": "Test Doc",
		},
	}
	result, err := provider.GenerateContext(context.Background(), doc, "chunk text")
	require.NoError(t, err)
	assert.Equal(t, "generated context", result)
}

func TestLLMContextProvider_GenerateContext_Error(t *testing.T) {
	t.Parallel()
	provider := NewLLMContextProvider(
		func(ctx context.Context, prompt string) (string, error) {
			return "", fmt.Errorf("llm error")
		},
		zap.NewNop(),
	)

	doc := Document{ID: "d1", Content: "content"}
	_, err := provider.GenerateContext(context.Background(), doc, "chunk")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate context")
}

// ============================================================
// ContextualRetrieval — chunkDocument
// ============================================================

func newTestContextualRetrieval(t *testing.T) *ContextualRetrieval {
	t.Helper()
	cfg := DefaultContextualRetrievalConfig()
	retriever := NewHybridRetriever(DefaultHybridRetrievalConfig(), zap.NewNop())
	provider := NewSimpleContextProvider(zap.NewNop())
	return NewContextualRetrieval(retriever, provider, cfg, zap.NewNop())
}

func TestContextualRetrieval_ChunkDocument_EmptyContent(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	doc := Document{ID: "d1", Content: ""}
	chunks := cr.chunkDocument(doc)
	assert.Nil(t, chunks)
}

func TestContextualRetrieval_ChunkDocument_ShortContent(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	doc := Document{ID: "d1", Content: "Short paragraph."}
	chunks := cr.chunkDocument(doc)
	require.Len(t, chunks, 1)
	assert.Equal(t, "Short paragraph.", chunks[0])
}

func TestContextualRetrieval_ChunkDocument_MultipleParagraphs(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	cr.config.ChunkSize = 50
	cr.config.ChunkOverlap = 10

	content := "First paragraph here.\n\nSecond paragraph here.\n\nThird paragraph here."
	doc := Document{ID: "d1", Content: content}
	chunks := cr.chunkDocument(doc)
	require.NotEmpty(t, chunks)
	// Each chunk should be non-empty
	for _, c := range chunks {
		assert.NotEmpty(t, c)
	}
}

// ============================================================
// ContextualRetrieval — cache operations
// ============================================================

func TestContextualRetrieval_CacheOperations(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	cr.config.CacheTTL = 1 * time.Hour

	key := cr.buildCacheKey("doc1", "chunk text")
	assert.NotEmpty(t, key)

	// Initially empty
	_, ok := cr.getFromCache(key)
	assert.False(t, ok)

	// Put and get
	cr.putToCache(key, "cached context")
	val, ok := cr.getFromCache(key)
	assert.True(t, ok)
	assert.Equal(t, "cached context", val)
}

func TestContextualRetrieval_CacheExpiry(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	cr.config.CacheTTL = 1 * time.Nanosecond // expire immediately

	key := cr.buildCacheKey("doc1", "chunk")
	cr.putToCache(key, "value")

	time.Sleep(2 * time.Millisecond)
	_, ok := cr.getFromCache(key)
	assert.False(t, ok)
}

func TestContextualRetrieval_CleanExpiredCache(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	cr.config.CacheTTL = 1 * time.Nanosecond

	cr.putToCache("k1", "v1")
	cr.putToCache("k2", "v2")

	time.Sleep(2 * time.Millisecond)
	cleaned := cr.CleanExpiredCache()
	assert.Equal(t, 2, cleaned)
}

// ============================================================
// ContextualRetrieval — BM25 / scoring
// ============================================================

func TestContextualRetrieval_CalculateContextRelevance_EmptyInputs(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	assert.Equal(t, 0.0, cr.calculateContextRelevance("", "some context"))
	assert.Equal(t, 0.0, cr.calculateContextRelevance("query", ""))
	assert.Equal(t, 0.0, cr.calculateContextRelevance("", ""))
}

func TestContextualRetrieval_CalculateContextRelevance_MatchingTerms(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	cr.config.BM25K1 = 1.2
	cr.config.BM25B = 0.75

	score := cr.calculateContextRelevance("machine learning", "this document covers machine learning algorithms")
	assert.Greater(t, score, 0.0)
	assert.LessOrEqual(t, score, 1.0)
}

func TestContextualRetrieval_GetIDF_CachesValue(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)

	idf1 := cr.getIDF("testterm", 100)
	idf2 := cr.getIDF("testterm", 100)
	assert.Equal(t, idf1, idf2)
	assert.Greater(t, idf1, 0.0)
}

// ============================================================
// EmbeddingSimilarity
// ============================================================

func TestEmbeddingSimilarity_Identical(t *testing.T) {
	t.Parallel()
	sim := EmbeddingSimilarity([]float64{1, 0, 0}, []float64{1, 0, 0})
	assert.InDelta(t, 1.0, sim, 1e-9)
}

func TestEmbeddingSimilarity_Orthogonal(t *testing.T) {
	t.Parallel()
	sim := EmbeddingSimilarity([]float64{1, 0}, []float64{0, 1})
	assert.InDelta(t, 0.0, sim, 1e-9)
}

func TestEmbeddingSimilarity_DifferentLengths(t *testing.T) {
	t.Parallel()
	sim := EmbeddingSimilarity([]float64{1, 0}, []float64{1, 0, 0})
	assert.Equal(t, 0.0, sim)
}

func TestEmbeddingSimilarity_EmptyVectors(t *testing.T) {
	t.Parallel()
	sim := EmbeddingSimilarity([]float64{}, []float64{})
	assert.Equal(t, 0.0, sim)
}

func TestEmbeddingSimilarity_ZeroVector(t *testing.T) {
	t.Parallel()
	sim := EmbeddingSimilarity([]float64{0, 0}, []float64{1, 0})
	assert.Equal(t, 0.0, sim)
}

// ============================================================
// contextualTokenize
// ============================================================

func TestContextualTokenize_English(t *testing.T) {
	t.Parallel()
	tokens := contextualTokenize("Hello World Test")
	assert.Contains(t, tokens, "hello")
	assert.Contains(t, tokens, "world")
	assert.Contains(t, tokens, "test")
}

func TestContextualTokenize_FiltersSingleChars(t *testing.T) {
	t.Parallel()
	tokens := contextualTokenize("a b c hello")
	// Single ASCII chars should be filtered out
	assert.NotContains(t, tokens, "a")
	assert.NotContains(t, tokens, "b")
	assert.Contains(t, tokens, "hello")
}

func TestContextualTokenize_Empty(t *testing.T) {
	t.Parallel()
	tokens := contextualTokenize("")
	assert.Empty(t, tokens)
}

// ============================================================
// getMetadataString
// ============================================================

func TestGetMetadataString_NilMap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", getMetadataString(nil, "key"))
}

func TestGetMetadataString_MissingKey(t *testing.T) {
	t.Parallel()
	m := map[string]any{"other": "value"}
	assert.Equal(t, "", getMetadataString(m, "key"))
}

func TestGetMetadataString_StringValue(t *testing.T) {
	t.Parallel()
	m := map[string]any{"key": "value"}
	assert.Equal(t, "value", getMetadataString(m, "key"))
}

func TestGetMetadataString_NonStringValue(t *testing.T) {
	t.Parallel()
	m := map[string]any{"key": 42}
	assert.Equal(t, "42", getMetadataString(m, "key"))
}

// ============================================================
// sortResultsByFinalScore
// ============================================================

func TestSortResultsByFinalScore(t *testing.T) {
	t.Parallel()
	results := []RetrievalResult{
		{FinalScore: 0.3},
		{FinalScore: 0.9},
		{FinalScore: 0.1},
		{FinalScore: 0.7},
	}
	sortResultsByFinalScore(results)
	assert.Equal(t, 0.9, results[0].FinalScore)
	assert.Equal(t, 0.7, results[1].FinalScore)
	assert.Equal(t, 0.3, results[2].FinalScore)
	assert.Equal(t, 0.1, results[3].FinalScore)
}

// ============================================================
// splitSentences
// ============================================================

func TestSplitSentences(t *testing.T) {
	t.Parallel()
	sentences := splitSentences("Hello world. How are you? I am fine!")
	assert.Len(t, sentences, 3)
	assert.Equal(t, "Hello world.", sentences[0])
	assert.Equal(t, "How are you?", sentences[1])
	assert.Equal(t, "I am fine!", sentences[2])
}

func TestSplitSentences_NoTerminator(t *testing.T) {
	t.Parallel()
	sentences := splitSentences("No sentence terminator here")
	assert.Len(t, sentences, 1)
	assert.Equal(t, "No sentence terminator here", sentences[0])
}

// ============================================================
// renderContextTemplate
// ============================================================

func TestContextualRetrieval_RenderContextTemplate(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	doc := Document{
		ID: "d1",
		Metadata: map[string]any{
			"title":   "Doc Title",
			"section": "Section A",
		},
	}
	result := cr.renderContextTemplate(doc, "chunk content", "generated context")
	assert.Contains(t, result, "Doc Title")
	assert.Contains(t, result, "Section A")
	assert.Contains(t, result, "generated context")
	assert.Contains(t, result, "chunk content")
}

// ============================================================
// IndexDocumentsWithContext
// ============================================================

func TestContextualRetrieval_IndexDocumentsWithContext_NoContextPrefix(t *testing.T) {
	t.Parallel()
	cfg := DefaultContextualRetrievalConfig()
	cfg.UseContextPrefix = false
	retriever := NewHybridRetriever(DefaultHybridRetrievalConfig(), zap.NewNop())
	provider := NewSimpleContextProvider(zap.NewNop())
	cr := NewContextualRetrieval(retriever, provider, cfg, zap.NewNop())

	docs := []Document{
		{ID: "d1", Content: "Hello world"},
	}
	err := cr.IndexDocumentsWithContext(context.Background(), docs)
	require.NoError(t, err)
}

func TestContextualRetrieval_IndexDocumentsWithContext_WithContext(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	cr.config.ChunkSize = 500

	docs := []Document{
		{
			ID:      "d1",
			Content: "This is a test document with enough content to be indexed.",
			Metadata: map[string]any{
				"title":   "Test",
				"section": "Intro",
			},
		},
	}
	err := cr.IndexDocumentsWithContext(context.Background(), docs)
	require.NoError(t, err)
}

func TestContextualRetrieval_IndexDocumentsWithContext_ProviderError(t *testing.T) {
	t.Parallel()
	cfg := DefaultContextualRetrievalConfig()
	retriever := NewHybridRetriever(DefaultHybridRetrievalConfig(), zap.NewNop())
	failProvider := NewLLMContextProvider(
		func(ctx context.Context, prompt string) (string, error) {
			return "", fmt.Errorf("provider failure")
		},
		zap.NewNop(),
	)
	cr := NewContextualRetrieval(retriever, failProvider, cfg, zap.NewNop())

	docs := []Document{
		{ID: "d1", Content: "Some content here."},
	}
	// Should not fail — provider errors are logged and skipped
	err := cr.IndexDocumentsWithContext(context.Background(), docs)
	require.NoError(t, err)
}

// ============================================================
// rerankWithEmbedding
// ============================================================

func TestContextualRetrieval_RerankWithEmbedding_EmptyQuery(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	results := []RetrievalResult{
		{FinalScore: 0.5, Document: Document{Embedding: []float64{1, 0}}},
	}
	reranked := cr.rerankWithEmbedding([]float64{}, results)
	assert.Equal(t, 0.5, reranked[0].FinalScore)
}

func TestContextualRetrieval_RerankWithEmbedding_WithEmbeddings(t *testing.T) {
	t.Parallel()
	cr := newTestContextualRetrieval(t)
	results := []RetrievalResult{
		{FinalScore: 0.5, Document: Document{Embedding: []float64{1, 0}}},
		{FinalScore: 0.3, Document: Document{Embedding: []float64{0, 1}}},
	}
	reranked := cr.rerankWithEmbedding([]float64{1, 0}, results)
	// First result should have higher score since embedding matches query
	assert.Greater(t, reranked[0].FinalScore, reranked[1].FinalScore)
}

// ============================================================
// DefaultContextualRetrievalConfig
// ============================================================

func TestDefaultContextualRetrievalConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultContextualRetrievalConfig()
	assert.True(t, cfg.UseContextPrefix)
	assert.True(t, cfg.UseReranking)
	assert.True(t, cfg.CacheContexts)
	assert.Equal(t, 500, cfg.ChunkSize)
	assert.Equal(t, 50, cfg.ChunkOverlap)
	assert.Equal(t, time.Hour, cfg.CacheTTL)
	assert.InDelta(t, 1.2, cfg.BM25K1, 1e-9)
	assert.InDelta(t, 0.75, cfg.BM25B, 1e-9)
}



