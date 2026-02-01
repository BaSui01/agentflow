package rag

import (
	"testing"

	"go.uber.org/zap"
)

// mockTokenizer implements Tokenizer interface for testing
type mockTokenizer struct{}

func (m *mockTokenizer) CountTokens(text string) int {
	// Simple approximation: ~4 chars per token
	return len(text) / 4
}

func (m *mockTokenizer) Encode(text string) []int {
	tokens := make([]int, m.CountTokens(text))
	for i := range tokens {
		tokens[i] = i
	}
	return tokens
}

func TestDefaultChunkingConfig(t *testing.T) {
	config := DefaultChunkingConfig()

	if config.Strategy != ChunkingRecursive {
		t.Errorf("expected strategy to be recursive, got %s", config.Strategy)
	}

	if config.ChunkSize != 512 {
		t.Errorf("expected chunk size to be 512, got %d", config.ChunkSize)
	}

	if config.ChunkOverlap != 102 {
		t.Errorf("expected chunk overlap to be 102, got %d", config.ChunkOverlap)
	}

	if config.MinChunkSize != 50 {
		t.Errorf("expected min chunk size to be 50, got %d", config.MinChunkSize)
	}
}

func TestNewDocumentChunker(t *testing.T) {
	config := DefaultChunkingConfig()
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	if chunker == nil {
		t.Fatal("expected chunker to be created")
	}

	if chunker.config.Strategy != ChunkingRecursive {
		t.Errorf("expected strategy to be recursive, got %s", chunker.config.Strategy)
	}
}

func TestDocumentChunker_FixedSizeChunking(t *testing.T) {
	config := ChunkingConfig{
		Strategy:     ChunkingFixed,
		ChunkSize:    100,
		ChunkOverlap: 20,
		MinChunkSize: 10,
	}
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	doc := Document{
		ID:      "test-doc",
		Content: "This is a test document with enough content to be split into multiple chunks. " +
			"We need to make sure the chunking works correctly with fixed size strategy. " +
			"Adding more content here to ensure we have multiple chunks. " +
			"The chunker should split this into several pieces based on the chunk size configuration.",
	}

	chunks := chunker.ChunkDocument(doc)

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}

	// Verify chunks have content
	for i, chunk := range chunks {
		if chunk.Content == "" {
			t.Errorf("chunk %d has empty content", i)
		}
	}
}

func TestDocumentChunker_RecursiveChunking(t *testing.T) {
	config := ChunkingConfig{
		Strategy:     ChunkingRecursive,
		ChunkSize:    100,
		ChunkOverlap: 20,
		MinChunkSize: 10,
	}
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	doc := Document{
		ID: "test-doc",
		Content: `First paragraph with some content.

Second paragraph with different content.

Third paragraph to ensure multiple chunks are created.

Fourth paragraph with even more content to test the recursive chunking algorithm.`,
	}

	chunks := chunker.ChunkDocument(doc)

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}

	// Verify chunks maintain some structure
	for i, chunk := range chunks {
		if chunk.Content == "" {
			t.Errorf("chunk %d has empty content", i)
		}
		if chunk.StartPos < 0 {
			t.Errorf("chunk %d has invalid start position: %d", i, chunk.StartPos)
		}
	}
}

func TestDocumentChunker_SemanticChunking(t *testing.T) {
	config := ChunkingConfig{
		Strategy:            ChunkingSemantic,
		ChunkSize:           100,
		ChunkOverlap:        20,
		MinChunkSize:        10,
		SimilarityThreshold: 0.8,
	}
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	doc := Document{
		ID:      "test-doc",
		Content: "Machine learning is a subset of AI. Deep learning uses neural networks. Natural language processing handles text.",
	}

	chunks := chunker.ChunkDocument(doc)

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

func TestDocumentChunker_DocumentAwareChunking(t *testing.T) {
	config := ChunkingConfig{
		Strategy:           ChunkingDocument,
		ChunkSize:          200,
		ChunkOverlap:       40,
		MinChunkSize:       20,
		PreserveTables:     true,
		PreserveCodeBlocks: true,
		PreserveHeaders:    true,
	}
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	doc := Document{
		ID: "test-doc",
		Content: `# Header 1

This is the first section with some content.

## Header 2

This is the second section.

| Column 1 | Column 2 |
|----------|----------|
| Value 1  | Value 2  |

` + "```python\nprint('Hello World')\n```" + `

More content after the code block.`,
	}

	chunks := chunker.ChunkDocument(doc)

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

func TestDocumentChunker_EmptyDocument(t *testing.T) {
	config := DefaultChunkingConfig()
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	doc := Document{
		ID:      "empty-doc",
		Content: "",
	}

	chunks := chunker.ChunkDocument(doc)

	// Empty document should return empty or single empty chunk
	if len(chunks) > 1 {
		t.Errorf("expected at most 1 chunk for empty document, got %d", len(chunks))
	}
}

func TestDocumentChunker_SmallDocument(t *testing.T) {
	config := ChunkingConfig{
		Strategy:     ChunkingRecursive,
		ChunkSize:    1000,
		ChunkOverlap: 200,
		MinChunkSize: 50,
	}
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	doc := Document{
		ID:      "small-doc",
		Content: "This is a small document.",
	}

	chunks := chunker.ChunkDocument(doc)

	// Small document may result in 0 or 1 chunk depending on implementation
	if len(chunks) > 1 {
		t.Errorf("expected at most 1 chunk for small document, got %d", len(chunks))
	}

	// If there's a chunk, verify content
	if len(chunks) == 1 && chunks[0].Content != doc.Content {
		t.Errorf("expected chunk content to match document content")
	}
}

func TestChunk_Metadata(t *testing.T) {
	chunk := Chunk{
		Content:    "Test content",
		StartPos:   0,
		EndPos:     12,
		TokenCount: 3,
		Metadata: map[string]interface{}{
			"source": "test",
			"index":  0,
		},
	}

	if chunk.Metadata["source"] != "test" {
		t.Error("expected metadata source to be 'test'")
	}

	if chunk.Metadata["index"] != 0 {
		t.Error("expected metadata index to be 0")
	}
}

func BenchmarkDocumentChunker_RecursiveChunking(b *testing.B) {
	config := DefaultChunkingConfig()
	tokenizer := &mockTokenizer{}
	logger := zap.NewNop()

	chunker := NewDocumentChunker(config, tokenizer, logger)

	// Create a large document
	content := ""
	for i := 0; i < 100; i++ {
		content += "This is paragraph number " + string(rune('0'+i%10)) + ". It contains some text for testing. "
	}

	doc := Document{
		ID:      "benchmark-doc",
		Content: content,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunker.ChunkDocument(doc)
	}
}
