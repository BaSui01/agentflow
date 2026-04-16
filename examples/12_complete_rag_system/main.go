package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	llmrerank "github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	ragruntime "github.com/BaSui01/agentflow/rag/runtime"
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
	docs := []core.Document{
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
	cacheDoc := core.Document{
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

	// 8. RAG runtime Builder 集成示例
	demoRuntimeBuilder(logger)

	// 9. Context manager 集成示例
	demoContextManagers(logger)

	fmt.Println("\n=== 完成 ===")
}

func demoRuntimeBuilder(logger *zap.Logger) {
	fmt.Println("\n=== Runtime Builder 集成 ===")
	ctx := context.Background()

	hybridCfg := rag.DefaultHybridRetrievalConfig()
	hybridCfg.TopK = 3
	store := rag.NewInMemoryVectorStore(logger)

	builder := ragruntime.NewBuilder(nil, logger).
		WithLogger(logger).
		WithVectorStore(store).
		WithEmbeddingProvider(exampleEmbeddingProvider{}).
		WithRerankProvider(exampleRerankProvider{}).
		WithHybridConfig(hybridCfg)

	if _, err := builder.BuildEnhancedRetriever(); err != nil {
		log.Fatalf("BuildEnhancedRetriever failed: %v", err)
	}
	if _, err := builder.BuildHybridRetriever(); err != nil {
		log.Fatalf("BuildHybridRetriever failed: %v", err)
	}
	if _, err := builder.BuildHybridRetrieverWithVectorStore(); err != nil {
		log.Fatalf("BuildHybridRetrieverWithVectorStore failed: %v", err)
	}

	runtimeCache, err := ragruntime.NewSemanticCache(store, ragruntime.SemanticCacheConfig{
		SimilarityThreshold: 0.7,
	}, logger)
	if err != nil {
		log.Fatalf("NewSemanticCache failed: %v", err)
	}
	if err := runtimeCache.Set(ctx, core.Document{
		ID:        "runtime-cache-doc",
		Content:   "runtime semantic cache demo",
		Embedding: []float64{0.1, 0.2, 0.3},
	}); err != nil {
		log.Fatalf("runtime cache set failed: %v", err)
	}
	if _, hit := runtimeCache.Get(ctx, []float64{0.1, 0.2, 0.3}); !hit {
		log.Fatal("runtime cache get expected hit")
	}
	if err := runtimeCache.Clear(ctx); err != nil {
		log.Fatalf("runtime cache clear failed: %v", err)
	}

	fmt.Println("runtime.Builder 已接入 provider/store 注入与 hybrid 检索构建")
}

func demoContextManagers(logger *zap.Logger) {
	fmt.Println("\n=== Context Managers 集成 ===")
	ctx := context.Background()

	overflowMessages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a strict assistant. " + strings.Repeat("rule ", 80)},
		{Role: types.RoleUser, Content: strings.Repeat("very long user message ", 120)},
		{Role: types.RoleAssistant, Content: strings.Repeat("very long assistant message ", 120)},
	}

	engineer := agentcontext.New(agentcontext.Config{
		MaxContextTokens: 80,
		ReserveForOutput: 0,
		SoftLimit:        0.2,
		WarnLimit:        0.4,
		HardLimit:        0.6,
		TargetUsage:      0.3,
		Strategy:         agentcontext.StrategyAdaptive,
	}, logger)
	status := engineer.GetStatus(overflowMessages)
	fmt.Printf("engineer level=%s usage=%.2f\n", status.Level.String(), status.UsageRatio)
	_, _ = engineer.MustFit(ctx, overflowMessages, "compress")
	_ = engineer.GetStats()
	_ = engineer.EstimateTokens(overflowMessages)
	_ = engineer.CanAddMessage(overflowMessages, types.Message{Role: types.RoleUser, Content: "next"})

	agentCfg := agentcontext.DefaultAgentContextConfig("gpt-4o")
	agentCfg.MaxContextTokens = 80
	agentCfg.ReserveForOutput = 0
	manager := agentcontext.NewAgentContextManager(agentCfg, logger)
	manager.SetSummaryProvider(func(_ context.Context, _ []types.Message) (string, error) {
		return "summary", nil
	})
	_, _ = manager.PrepareMessages(ctx, overflowMessages, "prepare")
	_ = manager.GetStatus(overflowMessages)
	_ = manager.CanAddMessage(overflowMessages, types.Message{Role: types.RoleAssistant, Content: "ok"})
	_ = manager.EstimateTokens(overflowMessages)
	_ = manager.GetStats()
	_ = manager.ShouldCompress(overflowMessages)
	_ = manager.GetRecommendation(overflowMessages)

	slide := agentcontext.NewWindowManager(agentcontext.WindowConfig{
		Strategy:      agentcontext.StrategySlidingWindow,
		MaxTokens:     120,
		MaxMessages:   2,
		ReserveTokens: 10,
		KeepSystemMsg: true,
		KeepLastN:     1,
	}, nil, nil)
	_, _ = slide.PrepareMessages(ctx, overflowMessages, "slide")
	_ = slide.GetStatus(overflowMessages)
	_ = slide.EstimateTokens(overflowMessages)

	budget := agentcontext.NewWindowManager(agentcontext.WindowConfig{
		Strategy:      agentcontext.StrategyTokenBudget,
		MaxTokens:     120,
		ReserveTokens: 10,
		KeepSystemMsg: true,
		KeepLastN:     1,
	}, nil, nil)
	_, _ = budget.PrepareMessages(ctx, overflowMessages, "budget")

	summary := agentcontext.NewWindowManager(agentcontext.WindowConfig{
		Strategy:      agentcontext.StrategySummarize,
		MaxTokens:     120,
		ReserveTokens: 10,
		KeepSystemMsg: true,
		KeepLastN:     1,
	}, nil, simpleSummarizer{})
	_, _ = summary.PrepareMessages(ctx, overflowMessages, "summary")

	fmt.Println("agent/context 的 Engineer/AgentContextManager/WindowManager 已接入示例链路")
}

type exampleEmbeddingProvider struct{}

func (exampleEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float64, error) {
	return []float64{0.1, 0.2, 0.3}, nil
}

func (exampleEmbeddingProvider) EmbedDocuments(_ context.Context, docs []string) ([][]float64, error) {
	out := make([][]float64, len(docs))
	for i := range docs {
		out[i] = []float64{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (exampleEmbeddingProvider) Name() string {
	return "example-embedding-provider"
}

type exampleRerankProvider struct{}

func (exampleRerankProvider) RerankSimple(_ context.Context, _ string, docs []string, topN int) ([]llmrerank.RerankResult, error) {
	if topN > len(docs) {
		topN = len(docs)
	}
	results := make([]llmrerank.RerankResult, 0, topN)
	for i := 0; i < topN; i++ {
		results = append(results, llmrerank.RerankResult{
			Index:          i,
			RelevanceScore: float64(topN - i),
		})
	}
	return results, nil
}

func (exampleRerankProvider) Name() string {
	return "example-rerank-provider"
}

type simpleSummarizer struct{}

func (simpleSummarizer) Summarize(_ context.Context, messages []types.Message) (string, error) {
	return fmt.Sprintf("summary(%d)", len(messages)), nil
}
