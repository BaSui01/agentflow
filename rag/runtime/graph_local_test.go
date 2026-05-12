package runtime

import (
	"context"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestKnowledgeGraphNeighborsAndTypeQueries(t *testing.T) {
	graph := NewKnowledgeGraph(zap.NewNop())
	graph.AddNode(&Node{ID: "a", Type: "person", Label: "Alice"})
	graph.AddNode(&Node{ID: "b", Type: "person", Label: "Bob"})
	graph.AddNode(&Node{ID: "c", Type: "company", Label: "ACME"})
	graph.AddEdge(&Edge{ID: "ab", Source: "a", Target: "b", Type: "knows"})
	graph.AddEdge(&Edge{ID: "bc", Source: "b", Target: "c", Type: "works_at"})

	node, ok := graph.GetNode("a")
	require.True(t, ok)
	assert.Equal(t, "Alice", node.Label)

	depthOne := graph.GetNeighbors("a", 1)
	require.Len(t, depthOne, 1)
	assert.Equal(t, "b", depthOne[0].ID)

	depthTwo := graph.GetNeighbors("a", 2)
	assert.ElementsMatch(t, []string{"b", "c"}, nodeIDs(depthTwo))
	assert.ElementsMatch(t, []string{"a", "b"}, nodeIDs(graph.QueryByType("person")))
}

func TestKnowledgeGraphAssignsMissingIDsAndCreatedAt(t *testing.T) {
	graph := NewKnowledgeGraph(nil)
	node := &Node{Type: "entity", Label: "generated"}
	graph.AddNode(node)

	assert.NotEmpty(t, node.ID)
	assert.False(t, node.CreatedAt.IsZero())
	_, ok := graph.GetNode(node.ID)
	assert.True(t, ok)
}

func TestSimpleGraphEmbedderEmbedsDeterministicallyAndNormalizes(t *testing.T) {
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 4}, zap.NewNop())
	vec1, err := embedder.Embed(context.Background(), "Alpha beta alpha")
	require.NoError(t, err)
	vec2, err := embedder.Embed(context.Background(), "Alpha beta alpha")
	require.NoError(t, err)

	assert.Equal(t, vec1, vec2)
	assert.Len(t, vec1, 4)
	assert.InDelta(t, 1.0, l2Norm(vec1), 1e-9)

	empty, err := embedder.Embed(context.Background(), "   ")
	require.NoError(t, err)
	assert.Equal(t, []float64{0, 0, 0, 0}, empty)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = embedder.Embed(canceled, "alpha")
	assert.ErrorIs(t, err, context.Canceled)
}

func nodeIDs(nodes []*Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func l2Norm(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value * value
	}
	return math.Sqrt(sum)
}

func TestGraphRAGAddDocumentAndRetrieveHybridResults(t *testing.T) {
	ctx := context.Background()
	graph := NewKnowledgeGraph(zap.NewNop())
	store := &memoryLowLevelVectorStore{items: map[string]lowLevelVectorItem{}}
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, zap.NewNop())
	rag := NewGraphRAG(graph, store, embedder, GraphRAGConfig{
		GraphWeight:   0.4,
		VectorWeight:  0.6,
		MaxGraphDepth: 1,
		MaxResults:    10,
		MinScore:      0,
	}, zap.NewNop())

	require.NoError(t, rag.AddDocument(ctx, GraphDocument{
		ID:      "doc-1",
		Title:   "Go concurrency",
		Content: "goroutine channel scheduler",
		Metadata: map[string]any{
			"source": "unit",
		},
		Entities: []Entity{{ID: "entity-go", Name: "Go", Type: "language"}},
	}))

	results, err := rag.Retrieve(ctx, "goroutine channel")
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "doc-1", results[0].ID)
	assert.Equal(t, "vector", results[0].Source)
	assert.Contains(t, graphResultIDs(results), "entity-go")
}

func TestGraphRAGAutoExtractsEntitiesWhenConfigured(t *testing.T) {
	ctx := context.Background()
	graph := NewKnowledgeGraph(zap.NewNop())
	store := &memoryLowLevelVectorStore{items: map[string]lowLevelVectorItem{}}
	embedder := NewSimpleGraphEmbedder(SimpleGraphEmbedderConfig{Dimension: 8}, zap.NewNop())
	extractor := &stubEntityExtractor{entities: []Entity{{ID: "entity-rag", Name: "RAG", Type: "concept"}}}
	rag := NewGraphRAG(graph, store, embedder, GraphRAGConfig{
		AutoExtractEntities: true,
		MaxGraphDepth:       1,
		MaxResults:          5,
		MinScore:            0,
	}, zap.NewNop(), WithEntityExtractor(extractor))

	require.NoError(t, rag.AddDocument(ctx, GraphDocument{ID: "doc-2", Title: "RAG", Content: "retrieval augmented generation"}))
	neighbors := graph.GetNeighbors("doc-2", 1)
	require.Len(t, neighbors, 1)
	assert.Equal(t, "entity-rag", neighbors[0].ID)
	assert.Equal(t, 1, extractor.calls)
}

func graphResultIDs(results []GraphRetrievalResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
	}
	return ids
}

type stubEntityExtractor struct {
	entities []Entity
	calls    int
}

func (e *stubEntityExtractor) ExtractEntities(context.Context, string) ([]Entity, error) {
	e.calls++
	return e.entities, nil
}

type lowLevelVectorItem struct {
	vector   []float64
	metadata map[string]any
}

type memoryLowLevelVectorStore struct {
	items map[string]lowLevelVectorItem
}

func (s *memoryLowLevelVectorStore) Store(_ context.Context, id string, vector []float64, metadata map[string]any) error {
	s.items[id] = lowLevelVectorItem{vector: vector, metadata: metadata}
	return nil
}

func (s *memoryLowLevelVectorStore) Search(_ context.Context, query []float64, topK int, _ map[string]any) ([]LowLevelSearchResult, error) {
	results := make([]LowLevelSearchResult, 0, len(s.items))
	for id, item := range s.items {
		results = append(results, LowLevelSearchResult{ID: id, Score: cosineForTest(query, item.vector), Metadata: item.metadata})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (s *memoryLowLevelVectorStore) Delete(_ context.Context, id string) error {
	delete(s.items, id)
	return nil
}

func cosineForTest(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
