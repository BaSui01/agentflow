package runtime

import (
	"testing"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockTokenizer struct{}

func (m *mockTokenizer) CountTokens(text string) int {
	return len(text) / 4
}

func TestDefaultChunkingConfig(t *testing.T) {
	cfg := DefaultChunkingConfig()
	assert.Equal(t, ChunkingRecursive, cfg.Strategy)
	assert.Equal(t, 512, cfg.ChunkSize)
	assert.Equal(t, 102, cfg.ChunkOverlap)
	assert.True(t, cfg.PreserveTables)
}

func TestNewDocumentChunker(t *testing.T) {
	cfg := DefaultChunkingConfig()
	chunker := NewDocumentChunker(cfg, &mockTokenizer{}, zap.NewNop())
	require.NotNil(t, chunker)
}

func TestDocumentChunker_FixedStrategy(t *testing.T) {
	cfg := ChunkingConfig{
		Strategy:     ChunkingFixed,
		ChunkSize:    100,
		ChunkOverlap: 20,
		MinChunkSize: 10,
	}
	chunker := NewDocumentChunker(cfg, &mockTokenizer{}, zap.NewNop())

	doc := Document{
		ID:      "test",
		Content: "This is a test document with some content that should be chunked into pieces.",
	}
	chunks := chunker.ChunkDocument(doc)
	assert.NotEmpty(t, chunks)
}

func TestDocumentChunker_RecursiveStrategy(t *testing.T) {
	cfg := ChunkingConfig{
		Strategy:     ChunkingRecursive,
		ChunkSize:    100,
		ChunkOverlap: 20,
		MinChunkSize: 10,
	}
	chunker := NewDocumentChunker(cfg, &mockTokenizer{}, zap.NewNop())

	doc := Document{
		ID:      "test",
		Content: "Paragraph one.\n\nParagraph two.\n\nParagraph three.",
	}
	chunks := chunker.ChunkDocument(doc)
	assert.NotEmpty(t, chunks)
}

func TestDocumentChunker_EmptyContent(t *testing.T) {
	cfg := DefaultChunkingConfig()
	chunker := NewDocumentChunker(cfg, &mockTokenizer{}, zap.NewNop())

	doc := Document{ID: "empty", Content: ""}
	chunks := chunker.ChunkDocument(doc)
	assert.Empty(t, chunks)
}

func TestBuildHybridRetrieverWithVectorStore(t *testing.T) {
	cfg := config.DefaultConfig()
	retriever, err := NewBuilder(cfg, zap.NewNop()).BuildHybridRetrieverWithVectorStore()
	require.NoError(t, err)
	require.NotNil(t, retriever)
}

func TestBuilder_WithEmbeddingType(t *testing.T) {
	b := NewBuilder(nil, zap.NewNop()).WithEmbeddingType(core.EmbeddingProviderType("openai"))
	assert.NotNil(t, b)
}

func TestBuilder_WithRerankType(t *testing.T) {
	b := NewBuilder(nil, zap.NewNop()).WithRerankType(core.RerankProviderType("cohere"))
	assert.NotNil(t, b)
}

func TestBuilder_WithHybridConfig(t *testing.T) {
	cfg := DefaultHybridRetrievalConfig()
	b := NewBuilder(nil, zap.NewNop()).WithHybridConfig(cfg)
	assert.NotNil(t, b)
}

func TestBuilder_WithAPIKey(t *testing.T) {
	b := NewBuilder(nil, zap.NewNop()).WithAPIKey("sk-test")
	assert.NotNil(t, b)
}

func TestBuilder_WithLogger(t *testing.T) {
	b := NewBuilder(nil, zap.NewNop()).WithLogger(zap.NewNop())
	assert.NotNil(t, b)
}
