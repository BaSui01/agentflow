package rag

import (
	"testing"

	"go.uber.org/zap"
)

// 模拟器工具 用于测试的切换器接口
type mockTokenizer struct{}

func (m *mockTokenizer) CountTokens(text string) int {
	// 简单近似 : ~ 4 个字符/ 每个符号
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

	// 校验块有内容
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

	// 校验块保持一些结构
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

	// 空文档应返回空或单个空块
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

	// 视执行情况,小文件可能导致0或1个块
	if len(chunks) > 1 {
		t.Errorf("expected at most 1 chunk for small document, got %d", len(chunks))
	}

	// 如果有块,请核对内容
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

	// 创建大文档
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
