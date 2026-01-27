package rag

import (
	"strings"
	"unicode"

	"go.uber.org/zap"
)

// ChunkingStrategy 分块策略
type ChunkingStrategy string

const (
	ChunkingFixed     ChunkingStrategy = "fixed"     // 固定大小
	ChunkingRecursive ChunkingStrategy = "recursive" // 递归分块
	ChunkingSemantic  ChunkingStrategy = "semantic"  // 语义分块
	ChunkingDocument  ChunkingStrategy = "document"  // 文档感知
)

// ChunkingConfig 分块配置（基于 2025 最佳实践）
type ChunkingConfig struct {
	Strategy     ChunkingStrategy `json:"strategy"`       // 分块策略
	ChunkSize    int              `json:"chunk_size"`     // 块大小（tokens）
	ChunkOverlap int              `json:"chunk_overlap"`  // 重叠大小（tokens）
	MinChunkSize int              `json:"min_chunk_size"` // 最小块大小

	// 语义分块参数
	SimilarityThreshold float64 `json:"similarity_threshold"` // 语义相似度阈值

	// 文档感知参数
	PreserveTables     bool `json:"preserve_tables"`      // 保留表格
	PreserveCodeBlocks bool `json:"preserve_code_blocks"` // 保留代码块
	PreserveHeaders    bool `json:"preserve_headers"`     // 保留标题
}

// DefaultChunkingConfig 默认分块配置（生产级）
func DefaultChunkingConfig() ChunkingConfig {
	return ChunkingConfig{
		Strategy:            ChunkingRecursive,
		ChunkSize:           512, // 400-800 tokens 最佳
		ChunkOverlap:        102, // 20% overlap
		MinChunkSize:        50,
		SimilarityThreshold: 0.8,
		PreserveTables:      true,
		PreserveCodeBlocks:  true,
		PreserveHeaders:     true,
	}
}

// Chunk 文档块
type Chunk struct {
	Content    string                 `json:"content"`
	StartPos   int                    `json:"start_pos"`
	EndPos     int                    `json:"end_pos"`
	Metadata   map[string]interface{} `json:"metadata"`
	TokenCount int                    `json:"token_count"`
}

// DocumentChunker 文档分块器
type DocumentChunker struct {
	config    ChunkingConfig
	tokenizer Tokenizer
	logger    *zap.Logger
}

// Tokenizer 分词器接口
type Tokenizer interface {
	CountTokens(text string) int
	Encode(text string) []int
}

// NewDocumentChunker 创建文档分块器
func NewDocumentChunker(config ChunkingConfig, tokenizer Tokenizer, logger *zap.Logger) *DocumentChunker {
	return &DocumentChunker{
		config:    config,
		tokenizer: tokenizer,
		logger:    logger,
	}
}

// ChunkDocument 分块文档
func (c *DocumentChunker) ChunkDocument(doc Document) []Chunk {
	switch c.config.Strategy {
	case ChunkingFixed:
		return c.fixedSizeChunking(doc)
	case ChunkingRecursive:
		return c.recursiveChunking(doc)
	case ChunkingSemantic:
		return c.semanticChunking(doc)
	case ChunkingDocument:
		return c.documentAwareChunking(doc)
	default:
		return c.recursiveChunking(doc)
	}
}

// recursiveChunking 递归分块（推荐用于生产环境）
// 在句子/段落边界分割，保持语义完整性
func (c *DocumentChunker) recursiveChunking(doc Document) []Chunk {
	content := doc.Content

	// 分隔符优先级：段落 > 句子 > 单词
	separators := []string{"\n\n", "\n", ". ", "。", "! ", "！", "? ", "？", " "}

	chunks := c.recursiveSplit(content, separators, 0, 0)

	// 添加重叠
	if c.config.ChunkOverlap > 0 {
		chunks = c.addOverlap(chunks, content)
	}

	c.logger.Info("recursive chunking completed",
		zap.Int("chunks", len(chunks)),
		zap.Int("chunk_size", c.config.ChunkSize),
		zap.Int("overlap", c.config.ChunkOverlap))

	return chunks
}

// recursiveSplit 递归分割
func (c *DocumentChunker) recursiveSplit(text string, separators []string, startPos int, depth int) []Chunk {
	if len(separators) == 0 {
		// 最后一级：按字符分割（句子边界感知）
		return c.splitByCharactersWithBoundary(text, startPos)
	}

	separator := separators[0]
	parts := strings.Split(text, separator)

	chunks := []Chunk{}
	currentChunk := ""
	currentStart := startPos

	for i, part := range parts {
		// 恢复分隔符（除了最后一个）
		if i < len(parts)-1 {
			part += separator
		}

		testChunk := currentChunk + part
		tokenCount := c.tokenizer.CountTokens(testChunk)

		if tokenCount <= c.config.ChunkSize {
			currentChunk = testChunk
		} else {
			// 当前块已满
			if currentChunk != "" {
				// 句子边界检测：确保不在句子中间分割
				finalChunk := c.adjustToSentenceBoundary(currentChunk)
				chunks = append(chunks, Chunk{
					Content:    strings.TrimSpace(finalChunk),
					StartPos:   currentStart,
					EndPos:     currentStart + len(finalChunk),
					TokenCount: c.tokenizer.CountTokens(finalChunk),
				})
				currentStart += len(finalChunk)

				// 将剩余部分添加到下一个块
				remainder := currentChunk[len(finalChunk):]
				currentChunk = remainder + part
			}

			// 如果单个 part 超过限制，递归使用下一级分隔符
			if c.tokenizer.CountTokens(part) > c.config.ChunkSize {
				subChunks := c.recursiveSplit(part, separators[1:], currentStart, depth+1)
				chunks = append(chunks, subChunks...)
				currentStart += len(part)
				currentChunk = ""
			} else if currentChunk == "" {
				currentChunk = part
			}
		}
	}

	// 添加最后一个块
	if currentChunk != "" && c.tokenizer.CountTokens(currentChunk) >= c.config.MinChunkSize {
		chunks = append(chunks, Chunk{
			Content:    strings.TrimSpace(currentChunk),
			StartPos:   currentStart,
			EndPos:     currentStart + len(currentChunk),
			TokenCount: c.tokenizer.CountTokens(currentChunk),
		})
	}

	return chunks
}

// splitByCharacters 按字符分割（最后手段）
func (c *DocumentChunker) splitByCharacters(text string, startPos int) []Chunk {
	chunks := []Chunk{}
	runes := []rune(text)

	// 估算每个 token 约 4 个字符
	charsPerChunk := c.config.ChunkSize * 4

	for i := 0; i < len(runes); i += charsPerChunk {
		end := i + charsPerChunk
		if end > len(runes) {
			end = len(runes)
		}

		chunkText := string(runes[i:end])
		chunks = append(chunks, Chunk{
			Content:    chunkText,
			StartPos:   startPos + i,
			EndPos:     startPos + end,
			TokenCount: c.tokenizer.CountTokens(chunkText),
		})
	}

	return chunks
}

// splitByCharactersWithBoundary 按字符分割（句子边界感知）
func (c *DocumentChunker) splitByCharactersWithBoundary(text string, startPos int) []Chunk {
	chunks := []Chunk{}
	runes := []rune(text)

	// 估算每个 token 约 4 个字符
	charsPerChunk := c.config.ChunkSize * 4

	for i := 0; i < len(runes); i += charsPerChunk {
		end := i + charsPerChunk
		if end > len(runes) {
			end = len(runes)
		}

		// 调整到句子边界
		chunkText := string(runes[i:end])
		adjustedText := c.adjustToSentenceBoundary(chunkText)

		chunks = append(chunks, Chunk{
			Content:    adjustedText,
			StartPos:   startPos + i,
			EndPos:     startPos + i + len([]rune(adjustedText)),
			TokenCount: c.tokenizer.CountTokens(adjustedText),
		})
	}

	return chunks
}

// adjustToSentenceBoundary 调整到句子边界（避免在句子中间分割）
func (c *DocumentChunker) adjustToSentenceBoundary(text string) string {
	if len(text) == 0 {
		return text
	}

	// 句子结束标记
	sentenceEnders := []rune{'.', '。', '!', '！', '?', '？', '\n'}

	// 从后往前查找最近的句子边界
	runes := []rune(text)
	for i := len(runes) - 1; i >= len(runes)/2; i-- { // 只在后半部分查找
		for _, ender := range sentenceEnders {
			if runes[i] == ender {
				// 找到句子边界，包含标点符号
				return string(runes[:i+1])
			}
		}
	}

	// 如果找不到句子边界，查找空格
	for i := len(runes) - 1; i >= len(runes)/2; i-- {
		if runes[i] == ' ' || runes[i] == '\t' {
			return string(runes[:i])
		}
	}

	// 实在找不到，返回原文
	return text
}

// addOverlap 添加重叠
func (c *DocumentChunker) addOverlap(chunks []Chunk, fullText string) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}

	overlapped := make([]Chunk, len(chunks))
	overlapChars := c.config.ChunkOverlap * 4 // 估算字符数

	for i := range chunks {
		chunk := chunks[i]

		// 从前一个块获取重叠内容
		if i > 0 {
			prevChunk := chunks[i-1]
			overlapStart := prevChunk.EndPos - overlapChars
			if overlapStart < prevChunk.StartPos {
				overlapStart = prevChunk.StartPos
			}

			if overlapStart < chunk.StartPos {
				overlapText := fullText[overlapStart:chunk.StartPos]
				chunk.Content = overlapText + chunk.Content
				chunk.StartPos = overlapStart
			}
		}

		overlapped[i] = chunk
	}

	return overlapped
}

// semanticChunking 语义分块（基于句子嵌入相似度）
func (c *DocumentChunker) semanticChunking(doc Document) []Chunk {
	// 1. 按句子分割
	sentences := c.splitIntoSentences(doc.Content)

	if len(sentences) == 0 {
		return []Chunk{}
	}

	// 2. 计算句子嵌入（简化版：使用词重叠作为相似度）
	// 生产环境应使用真实的句子嵌入模型
	similarities := c.calculateSentenceSimilarities(sentences)

	// 3. 在相似度低的地方分割
	chunks := []Chunk{}
	currentChunk := sentences[0]
	currentStart := 0

	for i := 1; i < len(sentences); i++ {
		similarity := similarities[i-1]

		// 如果相似度低于阈值，或块太大，则分割
		testChunk := currentChunk + " " + sentences[i]
		tokenCount := c.tokenizer.CountTokens(testChunk)

		if similarity < c.config.SimilarityThreshold || tokenCount > c.config.ChunkSize {
			// 创建新块
			chunks = append(chunks, Chunk{
				Content:    strings.TrimSpace(currentChunk),
				StartPos:   currentStart,
				EndPos:     currentStart + len(currentChunk),
				TokenCount: c.tokenizer.CountTokens(currentChunk),
			})
			currentStart += len(currentChunk) + 1
			currentChunk = sentences[i]
		} else {
			currentChunk = testChunk
		}
	}

	// 添加最后一个块
	if currentChunk != "" {
		chunks = append(chunks, Chunk{
			Content:    strings.TrimSpace(currentChunk),
			StartPos:   currentStart,
			EndPos:     currentStart + len(currentChunk),
			TokenCount: c.tokenizer.CountTokens(currentChunk),
		})
	}

	return chunks
}

// documentAwareChunking 文档感知分块（保留结构）
func (c *DocumentChunker) documentAwareChunking(doc Document) []Chunk {
	content := doc.Content
	chunks := []Chunk{}

	// 1. 识别特殊结构
	blocks := c.identifyStructuralBlocks(content)

	// 2. 处理每个块
	for _, block := range blocks {
		if block.Type == "code" && c.config.PreserveCodeBlocks {
			// 代码块不分割
			chunks = append(chunks, Chunk{
				Content:    block.Content,
				StartPos:   block.StartPos,
				EndPos:     block.EndPos,
				TokenCount: c.tokenizer.CountTokens(block.Content),
				Metadata: map[string]interface{}{
					"type": "code",
				},
			})
		} else if block.Type == "table" && c.config.PreserveTables {
			// 表格不分割
			chunks = append(chunks, Chunk{
				Content:    block.Content,
				StartPos:   block.StartPos,
				EndPos:     block.EndPos,
				TokenCount: c.tokenizer.CountTokens(block.Content),
				Metadata: map[string]interface{}{
					"type": "table",
				},
			})
		} else {
			// 普通文本使用递归分块
			subDoc := Document{Content: block.Content}
			subChunks := c.recursiveChunking(subDoc)

			// 调整位置
			for i := range subChunks {
				subChunks[i].StartPos += block.StartPos
				subChunks[i].EndPos += block.StartPos
			}

			chunks = append(chunks, subChunks...)
		}
	}

	return chunks
}

// fixedSizeChunking 固定大小分块（不推荐）
func (c *DocumentChunker) fixedSizeChunking(doc Document) []Chunk {
	content := doc.Content
	chunks := []Chunk{}

	charsPerChunk := c.config.ChunkSize * 4
	overlapChars := c.config.ChunkOverlap * 4

	for i := 0; i < len(content); i += (charsPerChunk - overlapChars) {
		end := i + charsPerChunk
		if end > len(content) {
			end = len(content)
		}

		chunkText := content[i:end]
		chunks = append(chunks, Chunk{
			Content:    chunkText,
			StartPos:   i,
			EndPos:     end,
			TokenCount: c.tokenizer.CountTokens(chunkText),
		})

		if end >= len(content) {
			break
		}
	}

	return chunks
}

// ====== 辅助方法 ======

// splitIntoSentences 分割成句子
func (c *DocumentChunker) splitIntoSentences(text string) []string {
	sentences := []string{}

	// 简化实现：按标点符号分割
	delimiters := []rune{'.', '。', '!', '！', '?', '？', '\n'}

	currentSentence := ""
	for _, char := range text {
		currentSentence += string(char)

		isDelimiter := false
		for _, delim := range delimiters {
			if char == delim {
				isDelimiter = true
				break
			}
		}

		if isDelimiter {
			trimmed := strings.TrimSpace(currentSentence)
			if trimmed != "" {
				sentences = append(sentences, trimmed)
			}
			currentSentence = ""
		}
	}

	// 添加最后一个句子
	if strings.TrimSpace(currentSentence) != "" {
		sentences = append(sentences, strings.TrimSpace(currentSentence))
	}

	return sentences
}

// calculateSentenceSimilarities 计算句子相似度
func (c *DocumentChunker) calculateSentenceSimilarities(sentences []string) []float64 {
	if len(sentences) <= 1 {
		return []float64{}
	}

	similarities := make([]float64, len(sentences)-1)

	for i := 0; i < len(sentences)-1; i++ {
		// 简化实现：词重叠相似度
		// 生产环境应使用句子嵌入模型
		similarities[i] = c.wordOverlapSimilarity(sentences[i], sentences[i+1])
	}

	return similarities
}

// wordOverlapSimilarity 词重叠相似度
func (c *DocumentChunker) wordOverlapSimilarity(s1, s2 string) float64 {
	words1 := strings.Fields(strings.ToLower(s1))
	words2 := strings.Fields(strings.ToLower(s2))

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// 计算交集
	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	overlap := 0
	for _, w := range words2 {
		if set1[w] {
			overlap++
		}
	}

	// Jaccard 相似度
	union := len(words1) + len(words2) - overlap
	if union == 0 {
		return 0.0
	}

	return float64(overlap) / float64(union)
}

// StructuralBlock 结构块
type StructuralBlock struct {
	Type     string // code, table, text, header
	Content  string
	StartPos int
	EndPos   int
}

// identifyStructuralBlocks 识别结构块
func (c *DocumentChunker) identifyStructuralBlocks(content string) []StructuralBlock {
	blocks := []StructuralBlock{}

	// 简化实现：识别代码块和表格
	lines := strings.Split(content, "\n")

	currentBlock := StructuralBlock{Type: "text"}
	currentPos := 0
	inCodeBlock := false
	inTable := false

	for _, line := range lines {
		lineLen := len(line) + 1 // +1 for newline

		// 检测代码块
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// 代码块结束
				currentBlock.Content += line + "\n"
				currentBlock.EndPos = currentPos + lineLen
				blocks = append(blocks, currentBlock)

				currentBlock = StructuralBlock{
					Type:     "text",
					StartPos: currentPos + lineLen,
				}
				inCodeBlock = false
			} else {
				// 代码块开始
				if currentBlock.Content != "" {
					currentBlock.EndPos = currentPos
					blocks = append(blocks, currentBlock)
				}

				currentBlock = StructuralBlock{
					Type:     "code",
					Content:  line + "\n",
					StartPos: currentPos,
				}
				inCodeBlock = true
			}
		} else if strings.Contains(line, "|") && strings.Count(line, "|") >= 2 {
			// 可能是表格
			if !inTable {
				if currentBlock.Content != "" {
					currentBlock.EndPos = currentPos
					blocks = append(blocks, currentBlock)
				}

				currentBlock = StructuralBlock{
					Type:     "table",
					Content:  line + "\n",
					StartPos: currentPos,
				}
				inTable = true
			} else {
				currentBlock.Content += line + "\n"
			}
		} else {
			if inTable {
				// 表格结束
				currentBlock.EndPos = currentPos
				blocks = append(blocks, currentBlock)

				currentBlock = StructuralBlock{
					Type:     "text",
					Content:  line + "\n",
					StartPos: currentPos,
				}
				inTable = false
			} else {
				currentBlock.Content += line + "\n"
			}
		}

		currentPos += lineLen
	}

	// 添加最后一个块
	if currentBlock.Content != "" {
		currentBlock.EndPos = currentPos
		blocks = append(blocks, currentBlock)
	}

	return blocks
}

// SimpleTokenizer 简单分词器（用于测试）
type SimpleTokenizer struct{}

func (t *SimpleTokenizer) CountTokens(text string) int {
	// 简化估算：1 token ≈ 4 字符
	return len(text) / 4
}

func (t *SimpleTokenizer) Encode(text string) []int {
	// 简化实现
	tokens := make([]int, len(text)/4)
	for i := range tokens {
		tokens[i] = i
	}
	return tokens
}

// isWhitespace 检查是否为空白字符
func isWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}
