package rag

import (
	"context"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// SimpleReranker
// ============================================================

func TestSimpleReranker_Rerank(t *testing.T) {
	reranker := NewSimpleReranker(zap.NewNop())

	results := []RetrievalResult{
		{Document: Document{ID: "d1", Content: "the quick brown fox"}, FinalScore: 0.5},
		{Document: Document{ID: "d2", Content: "quick fox jumps over"}, FinalScore: 0.3},
		{Document: Document{ID: "d3", Content: "unrelated content here"}, FinalScore: 0.8},
	}

	reranked, err := reranker.Rerank(context.Background(), "quick fox", results)
	require.NoError(t, err)
	assert.Len(t, reranked, 3)
	// Documents with "quick fox" should rank higher
	assert.True(t, reranked[0].RerankScore > 0)
}

func TestSimpleReranker_Rerank_Empty(t *testing.T) {
	reranker := NewSimpleReranker(zap.NewNop())
	results, err := reranker.Rerank(context.Background(), "query", nil)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ============================================================
// tokenize / abs
// ============================================================

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"  spaces  between  ", []string{"spaces", "between"}},
		{"single", []string{"single"}},
		{"", []string{}},
		{"tab\there", []string{"tab", "here"}},
		{"newline\nhere", []string{"newline", "here"}},
	}
	for _, tt := range tests {
		result := tokenize(tt.input)
		assert.Equal(t, tt.expected, result, "input: %q", tt.input)
	}
}

func TestAbs(t *testing.T) {
	assert.Equal(t, 5, abs(5))
	assert.Equal(t, 5, abs(-5))
	assert.Equal(t, 0, abs(0))
}

// ============================================================
// CrossEncoderReranker
// ============================================================

type mockCrossEncoderProvider struct {
	scores []float64
	err    error
}

func (m *mockCrossEncoderProvider) Score(_ context.Context, pairs []QueryDocPair) ([]float64, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.scores != nil {
		return m.scores[:len(pairs)], nil
	}
	scores := make([]float64, len(pairs))
	for i := range scores {
		scores[i] = float64(len(pairs) - i)
	}
	return scores, nil
}

func TestCrossEncoderReranker_Rerank(t *testing.T) {
	provider := &mockCrossEncoderProvider{scores: []float64{2.0, 0.5}}
	config := DefaultCrossEncoderConfig()
	config.BatchSize = 10
	reranker := NewCrossEncoderReranker(provider, config, zap.NewNop())

	results := []RetrievalResult{
		{Document: Document{ID: "d1", Content: "doc one"}, FinalScore: 0.3},
		{Document: Document{ID: "d2", Content: "doc two"}, FinalScore: 0.7},
	}

	reranked, err := reranker.Rerank(context.Background(), "query", results)
	require.NoError(t, err)
	assert.Len(t, reranked, 2)
	// d1 has higher cross-encoder score (2.0), so should rank first
	assert.Equal(t, "d1", reranked[0].Document.ID)
}

func TestCrossEncoderReranker_Rerank_Empty(t *testing.T) {
	provider := &mockCrossEncoderProvider{}
	reranker := NewCrossEncoderReranker(provider, DefaultCrossEncoderConfig(), zap.NewNop())
	results, err := reranker.Rerank(context.Background(), "query", nil)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ============================================================
// LLMReranker
// ============================================================

type mockLLMRerankerProvider struct {
	score float64
	err   error
}

func (m *mockLLMRerankerProvider) ScoreRelevance(_ context.Context, _, _ string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.score, nil
}

func TestLLMReranker_Rerank(t *testing.T) {
	provider := &mockLLMRerankerProvider{score: 8.0}
	reranker := NewLLMReranker(provider, DefaultLLMRerankerConfig(), zap.NewNop())

	results := []RetrievalResult{
		{Document: Document{ID: "d1", Content: "doc one"}, FinalScore: 0.5},
	}

	reranked, err := reranker.Rerank(context.Background(), "query", results)
	require.NoError(t, err)
	assert.Len(t, reranked, 1)
	assert.InDelta(t, 0.8, reranked[0].FinalScore, 0.01)
}

func TestLLMReranker_Rerank_Empty(t *testing.T) {
	provider := &mockLLMRerankerProvider{}
	reranker := NewLLMReranker(provider, DefaultLLMRerankerConfig(), zap.NewNop())
	results, err := reranker.Rerank(context.Background(), "query", nil)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestLLMReranker_Rerank_ProviderError(t *testing.T) {
	provider := &mockLLMRerankerProvider{err: assert.AnError}
	reranker := NewLLMReranker(provider, DefaultLLMRerankerConfig(), zap.NewNop())

	results := []RetrievalResult{
		{Document: Document{ID: "d1", Content: "doc"}, FinalScore: 0.5},
	}

	// Should not error, falls back to original score
	reranked, err := reranker.Rerank(context.Background(), "query", results)
	require.NoError(t, err)
	assert.Len(t, reranked, 1)
}

// ============================================================
// SimpleGraphEmbedder
// ============================================================

func TestSimpleGraphEmbedder_Embed(t *testing.T) {
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 64}, nil)

	vec, err := embedder.Embed(context.Background(), "hello world test")
	require.NoError(t, err)
	assert.Len(t, vec, 64)

	// Should be L2 normalized (norm ~= 1.0)
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	assert.InDelta(t, 1.0, norm, 0.001)
}

func TestSimpleGraphEmbedder_Embed_Empty(t *testing.T) {
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{}, nil)
	vec, err := embedder.Embed(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, vec, 128) // default dimension
	// All zeros for empty text
	for _, v := range vec {
		assert.Equal(t, 0.0, v)
	}
}

func TestSimpleGraphEmbedder_Embed_CancelledContext(t *testing.T) {
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := embedder.Embed(ctx, "hello")
	assert.Error(t, err)
}

func TestNormalize64(t *testing.T) {
	vec := []float64{3.0, 4.0}
	normalize64(vec)
	assert.InDelta(t, 0.6, vec[0], 0.001)
	assert.InDelta(t, 0.8, vec[1], 0.001)
}

func TestNormalize64_ZeroVector(t *testing.T) {
	vec := []float64{0.0, 0.0}
	normalize64(vec)
	assert.Equal(t, 0.0, vec[0])
	assert.Equal(t, 0.0, vec[1])
}

// ============================================================
// InMemoryVectorStore — DeleteDocuments, UpdateDocument
// ============================================================

func TestInMemoryVectorStore_DeleteDocuments(t *testing.T) {
	store := NewInMemoryVectorStore(zap.NewNop())
	ctx := context.Background()

	docs := []Document{
		{ID: "d1", Content: "doc one", Embedding: []float64{1, 0, 0}},
		{ID: "d2", Content: "doc two", Embedding: []float64{0, 1, 0}},
		{ID: "d3", Content: "doc three", Embedding: []float64{0, 0, 1}},
	}
	require.NoError(t, store.AddDocuments(ctx, docs))

	err := store.DeleteDocuments(ctx, []string{"d1", "d3"})
	require.NoError(t, err)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestInMemoryVectorStore_UpdateDocument(t *testing.T) {
	store := NewInMemoryVectorStore(zap.NewNop())
	ctx := context.Background()

	docs := []Document{
		{ID: "d1", Content: "original", Embedding: []float64{1, 0, 0}},
	}
	require.NoError(t, store.AddDocuments(ctx, docs))

	err := store.UpdateDocument(ctx, Document{ID: "d1", Content: "updated", Embedding: []float64{0, 1, 0}})
	require.NoError(t, err)

	// Search should find the updated content
	results, err := store.Search(ctx, []float64{0, 1, 0}, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "updated", results[0].Document.Content)
}

func TestInMemoryVectorStore_UpdateDocument_NotFound(t *testing.T) {
	store := NewInMemoryVectorStore(zap.NewNop())
	err := store.UpdateDocument(context.Background(), Document{ID: "nonexistent"})
	assert.Error(t, err)
}

// ============================================================
// SemanticCache — Get / Set
// ============================================================

func TestSemanticCache_GetSet(t *testing.T) {
	store := NewInMemoryVectorStore(zap.NewNop())
	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())
	ctx := context.Background()

	doc := Document{ID: "cached-1", Content: "cached result", Embedding: []float64{1, 0, 0}}
	require.NoError(t, cache.Set(ctx, doc))

	// Query with same embedding should hit cache
	result, ok := cache.Get(ctx, []float64{1, 0, 0})
	assert.True(t, ok)
	assert.Equal(t, "cached result", result.Content)
}

func TestSemanticCache_Get_Miss(t *testing.T) {
	store := NewInMemoryVectorStore(zap.NewNop())
	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.99}, zap.NewNop())
	ctx := context.Background()

	// Empty store, should miss
	_, ok := cache.Get(ctx, []float64{1, 0, 0})
	assert.False(t, ok)
}

// ============================================================
// Chunking — splitByCharacters, adjustToSentenceBoundary, isWhitespace
// ============================================================

func TestIsWhitespace(t *testing.T) {
	assert.True(t, isWhitespace(' '))
	assert.True(t, isWhitespace('\t'))
	assert.True(t, isWhitespace('\n'))
	assert.False(t, isWhitespace('a'))
	assert.False(t, isWhitespace('1'))
	_ = unicode.IsSpace // ensure import is used
}

func TestSimpleTokenizer_Encode(t *testing.T) {
	tok := &SimpleTokenizer{}
	tokens := tok.Encode("hello world test data")
	assert.Len(t, tokens, 5) // 20 chars / 4 = 5
}

func TestDocumentChunker_SplitByCharacters(t *testing.T) {
	chunker := NewDocumentChunker(ChunkingConfig{
		ChunkSize:    10, // 10 tokens * 4 chars = 40 chars per chunk
		ChunkOverlap: 0,
		Strategy:     ChunkingFixed,
	}, &SimpleTokenizer{}, zap.NewNop())

	text := "This is a test document that should be split into multiple chunks by character count."
	chunks := chunker.splitByCharacters(text, 0)
	assert.Greater(t, len(chunks), 1)
	for _, c := range chunks {
		assert.NotEmpty(t, c.Content)
	}
}

func TestDocumentChunker_SplitByCharactersWithBoundary(t *testing.T) {
	chunker := NewDocumentChunker(ChunkingConfig{
		ChunkSize:    10,
		ChunkOverlap: 0,
		Strategy:     ChunkingFixed,
	}, &SimpleTokenizer{}, zap.NewNop())

	text := "First sentence. Second sentence. Third sentence. Fourth sentence."
	chunks := chunker.splitByCharactersWithBoundary(text, 0)
	assert.Greater(t, len(chunks), 0)
}

func TestDocumentChunker_AdjustToSentenceBoundary(t *testing.T) {
	chunker := NewDocumentChunker(ChunkingConfig{ChunkSize: 10}, &SimpleTokenizer{}, zap.NewNop())

	tests := []struct {
		name  string
		input string
	}{
		{"with period", "Hello world. This is a test sentence. More text here"},
		{"with question", "What is this? Another sentence here"},
		{"no boundary", "just some text without any sentence ending markers"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chunker.adjustToSentenceBoundary(tt.input)
			if tt.input == "" {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)
			}
		})
	}
}

// ============================================================
// webResultCache — get / set
// ============================================================

func TestWebResultCache_GetSet(t *testing.T) {
	cache := newWebResultCache(5 * 60 * 1e9) // 5 minutes

	results := []WebRetrievalResult{
		{Title: "Result 1", URL: "http://example.com"},
	}

	// Miss
	_, ok := cache.get("test query")
	assert.False(t, ok)

	// Set
	cache.set("test query", results)

	// Hit
	got, ok := cache.get("test query")
	assert.True(t, ok)
	assert.Len(t, got, 1)
	assert.Equal(t, "Result 1", got[0].Title)
}

func TestWebResultCache_CaseInsensitive(t *testing.T) {
	cache := newWebResultCache(5 * 60 * 1e9)
	results := []WebRetrievalResult{{Title: "R1"}}

	cache.set("Test Query", results)
	got, ok := cache.get("test query")
	assert.True(t, ok)
	assert.Len(t, got, 1)
}

// ============================================================
// reasoningCache — evictOldest
// ============================================================

func TestReasoningCache_EvictOldest(t *testing.T) {
	cache := newReasoningCache(5 * time.Minute)
	cache.maxSize = 2

	cache.set("key1", &ReasoningChain{OriginalQuery: "q1", CreatedAt: time.Now().Add(-2 * time.Minute)})
	cache.set("key2", &ReasoningChain{OriginalQuery: "q2", CreatedAt: time.Now().Add(-1 * time.Minute)})
	// This should evict key1 (oldest)
	cache.set("key3", &ReasoningChain{OriginalQuery: "q3", CreatedAt: time.Now()})

	_, ok := cache.get("key1")
	assert.False(t, ok)
	_, ok = cache.get("key3")
	assert.True(t, ok)
}

// ============================================================
// GraphRAG — NewGraphRAG, Retrieve, AddDocument, min
// ============================================================

// mockLowLevelVectorStore implements LowLevelVectorStore for testing.
type mockLowLevelVectorStore struct {
	storeFn  func(ctx context.Context, id string, vector []float64, metadata map[string]any) error
	searchFn func(ctx context.Context, query []float64, topK int, filter map[string]any) ([]LowLevelSearchResult, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockLowLevelVectorStore) Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error {
	if m.storeFn != nil {
		return m.storeFn(ctx, id, vector, metadata)
	}
	return nil
}

func (m *mockLowLevelVectorStore) Search(ctx context.Context, query []float64, topK int, filter map[string]any) ([]LowLevelSearchResult, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, query, topK, filter)
	}
	return nil, nil
}

func (m *mockLowLevelVectorStore) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func TestNewGraphRAG(t *testing.T) {
	graph := NewKnowledgeGraph(zap.NewNop())
	store := &mockLowLevelVectorStore{}
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, nil)
	config := DefaultGraphRAGConfig()

	gr := NewGraphRAG(graph, store, embedder, config, nil)
	assert.NotNil(t, gr)
	assert.Equal(t, graph, gr.graph)
	assert.Equal(t, config.MaxResults, gr.config.MaxResults)
}

func TestGraphRAG_Retrieve_Empty(t *testing.T) {
	graph := NewKnowledgeGraph(zap.NewNop())
	store := &mockLowLevelVectorStore{}
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, nil)
	config := DefaultGraphRAGConfig()
	config.MinScore = 0.0

	gr := NewGraphRAG(graph, store, embedder, config, zap.NewNop())
	results, err := gr.Retrieve(context.Background(), "test query")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestGraphRAG_Retrieve_WithVectorResults(t *testing.T) {
	graph := NewKnowledgeGraph(zap.NewNop())
	// Add a node so graph expansion can find neighbors
	graph.AddNode(&Node{ID: "doc-1", Type: "document", Label: "Test Doc"})
	graph.AddNode(&Node{ID: "doc-2", Type: "document", Label: "Related Doc"})
	graph.AddEdge(&Edge{Source: "doc-1", Target: "doc-2", Type: "related"})

	store := &mockLowLevelVectorStore{
		searchFn: func(_ context.Context, _ []float64, _ int, _ map[string]any) ([]LowLevelSearchResult, error) {
			return []LowLevelSearchResult{
				{ID: "doc-1", Score: 0.9, Metadata: map[string]any{"title": "Test"}},
			}, nil
		},
	}
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, nil)
	config := DefaultGraphRAGConfig()
	config.MinScore = 0.0

	gr := NewGraphRAG(graph, store, embedder, config, zap.NewNop())
	results, err := gr.Retrieve(context.Background(), "test query")
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestGraphRAG_AddDocument(t *testing.T) {
	graph := NewKnowledgeGraph(zap.NewNop())
	var storedIDs []string
	store := &mockLowLevelVectorStore{
		storeFn: func(_ context.Context, id string, _ []float64, _ map[string]any) error {
			storedIDs = append(storedIDs, id)
			return nil
		},
	}
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, nil)
	config := DefaultGraphRAGConfig()

	gr := NewGraphRAG(graph, store, embedder, config, zap.NewNop())

	doc := GraphDocument{
		ID:      "doc-1",
		Title:   "Test Document",
		Content: "This is test content",
		Entities: []Entity{
			{ID: "ent-1", Name: "Entity One", Type: "person"},
		},
	}

	err := gr.AddDocument(context.Background(), doc)
	require.NoError(t, err)
	assert.Contains(t, storedIDs, "doc-1")

	// Verify node was added to graph
	node, ok := graph.GetNode("doc-1")
	assert.True(t, ok)
	assert.Equal(t, "Test Document", node.Label)

	// Verify entity node was added
	entNode, ok := graph.GetNode("ent-1")
	assert.True(t, ok)
	assert.Equal(t, "person", entNode.Type)
}

func TestGraphRAG_AddDocument_EmbedError(t *testing.T) {
	graph := NewKnowledgeGraph(zap.NewNop())
	store := &mockLowLevelVectorStore{}
	// Use an embedder with cancelled context to trigger error
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, nil)
	config := DefaultGraphRAGConfig()

	gr := NewGraphRAG(graph, store, embedder, config, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := gr.AddDocument(ctx, GraphDocument{ID: "doc-1", Content: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embed document")
}

func TestMin(t *testing.T) {
	assert.Equal(t, 3, min(3, 5))
	assert.Equal(t, 3, min(5, 3))
	assert.Equal(t, 4, min(4, 4))
}

