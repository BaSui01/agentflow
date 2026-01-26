package main

import (
	"context"
	"fmt"
	"log"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// 完整的 RAG 系统示例
// 展示：向量存储 + 混合检索 + 语义缓存 + 上下文压缩

func main() {
	// 初始化日志
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== 完整 RAG 系统示例 ===")

	// 1. 创建向量存储
	vectorStore := rag.NewInMemoryVectorStore(logger)

	// 2. 准备文档
	docs := []rag.Document{
		{
			ID:        "doc1",
			Content:   "Go is a statically typed, compiled programming language designed at Google.",
			Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		},
		{
			ID:        "doc2",
			Content:   "Python is an interpreted, high-level programming language with dynamic typing.",
			Embedding: []float64{0.2, 0.3, 0.4, 0.5, 0.6},
		},
		{
			ID:        "doc3",
			Content:   "Rust is a systems programming language focused on safety and performance.",
			Embedding: []float64{0.15, 0.25, 0.35, 0.45, 0.55},
		},
	}

	// 3. 索引文档
	if err := vectorStore.AddDocuments(context.Background(), docs); err != nil {
		log.Fatal(err)
	}

	// 4. 创建混合检索器
	config := rag.DefaultHybridRetrievalConfig()
	retriever := rag.NewHybridRetrieverWithVectorStore(config, vectorStore, logger)

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
	cacheConfig := rag.SemanticCacheConfig{
		SimilarityThreshold: 0.9,
	}
	cache := rag.NewSemanticCache(vectorStore, cacheConfig, logger)

	// 设置缓存
	cacheDoc := rag.Document{
		ID:        "cache1",
		Content:   "Cached response for programming language query",
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

	// 使用新的 agent/context.Engineer 进行上下文管理
	engineerConfig := agentcontext.DefaultConfig()
	engineer := agentcontext.New(engineerConfig, logger)

	// 准备消息
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "What is Go?"},
		{Role: types.RoleAssistant, Content: "Go is a programming language."},
		{Role: types.RoleUser, Content: "What about Python?"},
		{Role: types.RoleAssistant, Content: "Python is also a programming language."},
	}

	// 获取上下文状态
	status := engineer.GetStatus(messages)
	fmt.Printf("当前 token 数: %d\n", status.CurrentTokens)
	fmt.Printf("使用率: %.2f%%\n", status.UsageRatio*100)
	fmt.Printf("建议: %s\n", status.Recommendation)

	// 管理上下文（如果需要压缩）
	managed, err := engineer.Manage(context.Background(), messages, "What about Python?")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("原始消息数: %d\n", len(messages))
	fmt.Printf("管理后消息数: %d\n", len(managed))

	fmt.Println("\n=== 完成 ===")
}
