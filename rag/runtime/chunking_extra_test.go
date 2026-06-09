package runtime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDocumentChunkerSemanticAndDocumentAwareStrategies(t *testing.T) {
	semanticCfg := ChunkingConfig{Strategy: ChunkingSemantic, ChunkSize: 40, ChunkOverlap: 4, MinChunkSize: 1, SimilarityThreshold: 0.95}
	semantic := NewDocumentChunker(semanticCfg, &EnhancedTokenizer{}, zap.NewNop())
	semanticChunks := semantic.ChunkDocument(Document{ID: "semantic", Content: "cats chase mice. cats like naps. databases store rows. sql queries tables."})
	require.NotEmpty(t, semanticChunks)
	assert.Contains(t, semanticChunks[0].Content, "cats")

	docCfg := ChunkingConfig{Strategy: ChunkingDocument, ChunkSize: 60, ChunkOverlap: 4, MinChunkSize: 1, PreserveTables: true, PreserveCodeBlocks: true, PreserveHeaders: true}
	documentAware := NewDocumentChunker(docCfg, &EnhancedTokenizer{}, zap.NewNop())
	docChunks := documentAware.ChunkDocument(Document{ID: "doc", Content: "# Heading\nintro text\n| a | b |\n| 1 | 2 |\n```go\nfmt.Println(1)\n```"})
	require.NotEmpty(t, docChunks)
	assert.True(t, chunksContainMetadataType(docChunks, "table") || chunksContainMetadataType(docChunks, "code"))
}

func TestChunkingHelpersAndTokenizers(t *testing.T) {
	simple := &SimpleTokenizer{}
	assert.Equal(t, 2, simple.CountTokens("12345678"))
	assert.Equal(t, []int{0, 1}, simple.Encode("12345678"))

	enhanced := &EnhancedTokenizer{}
	assert.Greater(t, enhanced.CountTokens("你好世界"), 0)
	assert.Len(t, enhanced.Encode("hello world"), enhanced.CountTokens("hello world"))

	assert.True(t, isCJKRune('你'))
	assert.True(t, isWhitespace(' '))
	assert.True(t, isSentenceBoundary('.', ' '))
	assert.False(t, isSentenceBoundary('.', 'x'))
	assert.True(t, isMarkdownTableLine("| a | b |"))
	assert.True(t, isHeaderLine("## Title"))

	chunker := NewDocumentChunker(ChunkingConfig{ChunkSize: 10, ChunkOverlap: 2, MinChunkSize: 1}, enhanced, zap.NewNop())
	sentences := chunker.splitIntoSentences("One sentence. Two sentence! 三句话？")
	assert.Len(t, sentences, 3)
	idf := sentenceIDF(sentences)
	assert.NotEmpty(t, idf)
	assert.NotEmpty(t, tokenizeForSimilarity("Hello, world!"))

	assert.Greater(t, chunker.tfidfCosineSimilarity("alpha beta", "alpha gamma", idf), 0.0)

	longWord := strings.Repeat("x", 80)
	split := chunker.splitByCharacters(longWord, 10)
	assert.NotEmpty(t, split)
	boundary := chunker.splitByCharactersWithBoundary("first sentence. second sentence. third sentence.", 5)
	assert.NotEmpty(t, boundary)
}

func chunksContainMetadataType(chunks []Chunk, typ string) bool {
	for _, chunk := range chunks {
		if chunk.Metadata != nil && chunk.Metadata["type"] == typ {
			return true
		}
	}
	return false
}
