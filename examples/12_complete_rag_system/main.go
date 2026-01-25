package main

import (
	"context"
	"fmt"
	"log"

	llmcontext "github.com/BaSui01/agentflow/llm/context"
	"github.com/BaSui01/agentflow/llm/retrieval"
	"go.uber.org/zap"
)

// 完整的 RAG 系统示例
// 展示：向量存储 + 混合检索 + 语义缓存 + 上下文压缩

func main() {
	// 初始化日志
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== 完整 RAG 系统示例 ===\n")

	// 1. 创建向量存储
	vectorStore := retrieval.NewInMemoryVectorStore(logger)

	// 2. 准备文档
	docs := []retrieval.Document{
		{
			ID:      "doc1",
			Content: "Go is a statically typed, compiled programming language designed at Google.",
			Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		},
		{
			ID:      "doc2",
			Content: "Python is an interpreted, high-level programming language with dynamic typing.",
			Embedding: []float64{0.2, 0.3, 0.4, 0.5, 0.6},
		},
		{
			ID:      "doc3",
			Content: "Rust is a systems programming language focused on safety and performance.",
			Embedding: []float64{0.15, 0.25, 0.35, 0.45, 0.55},
		},
	}

	// 3. 索引文档
	if err := vectorStore.AddDocuments(context.Background(), docs); err != nil {
		log.Fatal(err)
	}

	// 4. 创建混合检索器
	config := retrieval.DefaultHybridRetrievalConfig()
	retriever := retrieval.NewHybridRetrieverWithVectorStore(config, vectorStore, logger)

	if err := retriever.IndexDocuments(docs); err != nil {
		log.Fatal(err)
	}

	// 5. 执行检索
	query := "programming language"
	queryEmbedding := []float64{0.12, 0.22, 0.32, 0.42, 0.52}

	results, err := retriever.Retrieve(context.Background(), query, queryEmbedding)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("检索结果:")
	for i, result := range results {
		fmt.Printf("%d. [Score: %.3f] %s\n", i+1, result.FinalScore, result.Document.Content)
	}

	// 6. 语义缓存示例
	fmt.Println("\n=== 语义缓存 ===")
	cacheConfig := retrieval.SemanticCacheConfig{
		SimilarityThreshold: 0.9,
	}
	cache := retrieval.NewSemanticCache(vectorStore, cacheConfig, logger)

	// 设置缓存
	cacheDoc := retrieval.Document{
		ID:      "cache1",
		Content: "Cached response for programming language query",
		Embedding: queryEmbedding,
	}
	if err := cache.Set(context.Background(), cacheDoc); err != nil {
		log.Fatal(err)
	}

	// 获取缓存
	if doc, hit := cache.Get(context.Background(), queryEmbedding); hit {
		fmt.Printf("缓存命中: %s\n", doc.Content)
	} else {
		fmt.Println("缓存未命中")
	}

	// 7. 上下文压缩示例
	fmt.Println("\n=== 上下文压缩 ===")

	// 创建 tokenizer（简化版）
	tokenizer := &SimpleTokenizer{}

	// Create summary compressor
	compressorConfig := llmcontext.DefaultSummaryCompressionConfig()
	compressor := llmcontext.NewSummaryCompressor(nil, compressorConfig, logger)

	// Create enhanced context manager
	contextManager := llmcontext.NewDefaultContextManagerWithCompression(tokenizer, compressor, logger)

	// Prepare messages
	messages := []llmcontext.Message{
		{Role: llmcontext.RoleSystem, Content: "You are a helpful assistant."},
		{Role: llmcontext.RoleUser, Content: "What is Go?"},
		{Role: llmcontext.RoleAssistant, Content: "Go is a programming language."},
		{Role: llmcontext.RoleUser, Content: "What about Python?"},
		{Role: llmcontext.RoleAssistant, Content: "Python is also a programming language."},
	}

	// 裁剪消息
	trimmed, err := contextManager.TrimMessages(messages, 100)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("原始消息数: %d\n", len(messages))
	fmt.Printf("裁剪后消息数: %d\n", len(trimmed))

	fmt.Println("\n=== 完成 ===")
}

// SimpleTokenizer 简单的 tokenizer 实现
type SimpleTokenizer struct{}

func (t *SimpleTokenizer) CountTokens(text string) int {
	return len(text) / 4 // 简化估算
}

func (t *SimpleTokenizer) CountMessageTokens(msg llmcontext.Message) int {
	return t.CountTokens(msg.Content)
}

func (t *SimpleTokenizer) CountMessagesTokens(msgs []llmcontext.Message) int {
	total := 0
	for _, msg := range msgs {
		total += t.CountMessageTokens(msg)
	}
	return total
}

func (t *SimpleTokenizer) EstimateToolTokens(tools []llmcontext.ToolSchema) int {
	return len(tools) * 50 // Simple estimation
}
