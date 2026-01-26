package rag

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// ContextualRetrieval Anthropic 上下文检索（2025 最佳实践）
// 为每个 chunk 添加文档级上下文，提高检索准确率 50-60%
type ContextualRetrieval struct {
	retriever       *HybridRetriever
	contextProvider ContextProvider
	config          ContextualRetrievalConfig
	logger          *zap.Logger
}

// ContextualRetrievalConfig 上下文检索配置
type ContextualRetrievalConfig struct {
	// 上下文生成
	UseContextPrefix    bool   `json:"use_context_prefix"`     // 是否添加上下文前缀
	ContextTemplate     string `json:"context_template"`       // 上下文模板
	MaxContextLength    int    `json:"max_context_length"`     // 最大上下文长度
	
	// 检索增强
	UseReranking        bool    `json:"use_reranking"`          // 是否使用重排序
	ContextWeight       float64 `json:"context_weight"`         // 上下文权重（0.3-0.5）
	
	// 缓存
	CacheContexts       bool    `json:"cache_contexts"`         // 是否缓存上下文
}

// DefaultContextualRetrievalConfig 默认配置
func DefaultContextualRetrievalConfig() ContextualRetrievalConfig {
	return ContextualRetrievalConfig{
		UseContextPrefix:  true,
		ContextTemplate:   "Document: {{document_title}}\nSection: {{section_title}}\nContext: {{context}}\n\nContent: {{content}}",
		MaxContextLength:  200,
		UseReranking:      true,
		ContextWeight:     0.4,
		CacheContexts:     true,
	}
}

// ContextProvider 上下文提供器接口
type ContextProvider interface {
	// GenerateContext 为 chunk 生成上下文
	GenerateContext(ctx context.Context, doc Document, chunk string) (string, error)
}

// LLMContextProvider 基于 LLM 的上下文生成器
type LLMContextProvider struct {
	llmProvider func(context.Context, string) (string, error)
	logger      *zap.Logger
}

// NewLLMContextProvider 创建 LLM 上下文提供器
func NewLLMContextProvider(
	llmProvider func(context.Context, string) (string, error),
	logger *zap.Logger,
) *LLMContextProvider {
	return &LLMContextProvider{
		llmProvider: llmProvider,
		logger:      logger,
	}
}

// GenerateContext 生成上下文
func (p *LLMContextProvider) GenerateContext(ctx context.Context, doc Document, chunk string) (string, error) {
	prompt := fmt.Sprintf(`Given the following document and chunk, provide a brief context (1-2 sentences) that explains what this chunk is about in relation to the whole document.

Document Title: %s
Full Document: %s

Chunk: %s

Context:`, doc.Metadata["title"], doc.Content, chunk)

	context, err := p.llmProvider(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate context: %w", err)
	}

	return strings.TrimSpace(context), nil
}

// NewContextualRetrieval 创建上下文检索器
func NewContextualRetrieval(
	retriever *HybridRetriever,
	contextProvider ContextProvider,
	config ContextualRetrievalConfig,
	logger *zap.Logger,
) *ContextualRetrieval {
	return &ContextualRetrieval{
		retriever:       retriever,
		contextProvider: contextProvider,
		config:          config,
		logger:          logger,
	}
}

// IndexDocumentsWithContext 索引文档（添加上下文）
func (r *ContextualRetrieval) IndexDocumentsWithContext(ctx context.Context, docs []Document) error {
	if !r.config.UseContextPrefix {
		return r.retriever.IndexDocuments(docs)
	}

	// 为每个文档的 chunk 添加上下文
	enrichedDocs := make([]Document, 0)

	for _, doc := range docs {
		// 分块
		chunks := r.chunkDocument(doc)

		for i, chunk := range chunks {
			// 生成上下文
			contextStr, err := r.contextProvider.GenerateContext(ctx, doc, chunk)
			if err != nil {
				r.logger.Warn("failed to generate context, using original chunk",
					zap.String("doc_id", doc.ID),
					zap.Int("chunk_idx", i),
					zap.Error(err))
				contextStr = ""
			}

			// 创建增强的文档
			enrichedContent := r.renderContextTemplate(doc, chunk, contextStr)

			enrichedDoc := Document{
				ID:        fmt.Sprintf("%s_chunk_%d", doc.ID, i),
				Content:   enrichedContent,
				Embedding: nil, // 需要重新生成 embedding
				Metadata: map[string]interface{}{
					"original_doc_id": doc.ID,
					"chunk_index":     i,
					"context":         contextStr,
					"original_chunk":  chunk,
				},
			}

			enrichedDocs = append(enrichedDocs, enrichedDoc)
		}
	}

	r.logger.Info("indexed documents with context",
		zap.Int("original_docs", len(docs)),
		zap.Int("enriched_chunks", len(enrichedDocs)))

	return r.retriever.IndexDocuments(enrichedDocs)
}

// Retrieve 检索（使用上下文增强）
func (r *ContextualRetrieval) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]RetrievalResult, error) {
	// 使用底层检索器
	results, err := r.retriever.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, err
	}

	// 如果启用重排序，考虑上下文相关性
	if r.config.UseReranking {
		results = r.rerankWithContext(query, results)
	}

	return results, nil
}

// chunkDocument 分块文档
func (r *ContextualRetrieval) chunkDocument(doc Document) []string {
	// 简化实现：按段落分块
	content := doc.Content
	paragraphs := strings.Split(content, "\n\n")

	chunks := make([]string, 0)
	currentChunk := ""

	for _, para := range paragraphs {
		if len(currentChunk)+len(para) > 500 { // 500 字符一个 chunk
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
			}
			currentChunk = para
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += para
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

// renderContextTemplate 渲染上下文模板
func (r *ContextualRetrieval) renderContextTemplate(doc Document, chunk, context string) string {
	template := r.config.ContextTemplate

	// 替换变量
	template = strings.ReplaceAll(template, "{{document_title}}", getMetadataString(doc.Metadata, "title"))
	template = strings.ReplaceAll(template, "{{section_title}}", getMetadataString(doc.Metadata, "section"))
	template = strings.ReplaceAll(template, "{{context}}", context)
	template = strings.ReplaceAll(template, "{{content}}", chunk)

	return template
}

// rerankWithContext 基于上下文重排序
func (r *ContextualRetrieval) rerankWithContext(query string, results []RetrievalResult) []RetrievalResult {
	for i := range results {
		// 提取上下文
		context := getMetadataString(results[i].Document.Metadata, "context")

		// 计算上下文相关性
		contextScore := r.calculateContextRelevance(query, context)

		// 混合分数
		results[i].FinalScore = results[i].FinalScore*(1-r.config.ContextWeight) +
			contextScore*r.config.ContextWeight
	}

	// 重新排序
	sortResultsByFinalScore(results)

	return results
}

// calculateContextRelevance 计算上下文相关性
func (r *ContextualRetrieval) calculateContextRelevance(query, context string) float64 {
	// 简化实现：词重叠
	queryWords := strings.Fields(strings.ToLower(query))
	contextWords := strings.Fields(strings.ToLower(context))

	if len(queryWords) == 0 {
		return 0.0
	}

	matchCount := 0
	for _, qw := range queryWords {
		for _, cw := range contextWords {
			if qw == cw {
				matchCount++
				break
			}
		}
	}

	return float64(matchCount) / float64(len(queryWords))
}

// getMetadataString 获取元数据字符串
func getMetadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}

	val, ok := metadata[key]
	if !ok {
		return ""
	}

	str, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%v", val)
	}

	return str
}

// sortResultsByFinalScore 按最终分数排序
func sortResultsByFinalScore(results []RetrievalResult) {
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].FinalScore < results[j+1].FinalScore {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
