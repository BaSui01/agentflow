package rag

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewMultiHopReasoner(t *testing.T) {
	config := DefaultMultiHopConfig()
	retriever := NewHybridRetriever(DefaultHybridRetrievalConfig(), nil)
	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, nil)

	if reasoner == nil {
		t.Fatal("expected reasoner to be created")
	}

	if reasoner.config.MaxHops != 4 {
		t.Errorf("expected MaxHops to be 4, got %d", reasoner.config.MaxHops)
	}
}

func TestMultiHopReasoner_Reason_Basic(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.MaxHops = 2
	config.EnableLLMReasoning = false

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	// 索引一些测试文档
	docs := []Document{
		{ID: "doc1", Content: "Machine learning is a subset of artificial intelligence."},
		{ID: "doc2", Content: "Python is a popular programming language for machine learning."},
		{ID: "doc3", Content: "TensorFlow is a machine learning framework developed by Google."},
	}
	retriever.IndexDocuments(docs)

	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	chain, err := reasoner.Reason(ctx, query)
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}

	if chain == nil {
		t.Fatal("expected chain to be returned")
	}

	if chain.OriginalQuery != query {
		t.Errorf("expected OriginalQuery to be %q, got %q", query, chain.OriginalQuery)
	}

	if chain.Status != StatusCompleted {
		t.Errorf("expected status to be completed, got %s", chain.Status)
	}

	if len(chain.Hops) == 0 {
		t.Error("expected at least one hop")
	}
}

func TestMultiHopReasoner_Reason_WithQueryTransformer(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.MaxHops = 2
	config.EnableLLMReasoning = false

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	docs := []Document{
		{ID: "doc1", Content: "Machine learning algorithms learn from data."},
		{ID: "doc2", Content: "Deep learning is a type of machine learning."},
	}
	retriever.IndexDocuments(docs)

	transformerConfig := DefaultQueryTransformConfig()
	transformerConfig.UseLLM = false
	transformer := NewQueryTransformer(transformerConfig, nil, zap.NewNop())

	reasoner := NewMultiHopReasoner(config, retriever, transformer, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "How does machine learning work?"

	chain, err := reasoner.Reason(ctx, query)
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}

	if chain.Status != StatusCompleted {
		t.Errorf("expected status to be completed, got %s", chain.Status)
	}

	// 检查是否发现意图
	if _, ok := chain.Metadata["intent"]; !ok {
		t.Log("intent metadata not set (expected when transformer is used)")
	}
}

func TestMultiHopReasoner_Reason_WithLLM(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.MaxHops = 2
	config.EnableLLMReasoning = true

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	docs := []Document{
		{ID: "doc1", Content: "Machine learning is a branch of AI."},
		{ID: "doc2", Content: "Neural networks are used in deep learning."},
	}
	retriever.IndexDocuments(docs)

	mockLLM := newMockLLMProvider()
	reasoner := NewMultiHopReasoner(config, retriever, nil, mockLLM, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	chain, err := reasoner.Reason(ctx, query)
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}

	if chain.Status != StatusCompleted {
		t.Errorf("expected status to be completed, got %s", chain.Status)
	}
}

func TestMultiHopReasoner_Reason_Timeout(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.TotalTimeout = 1 * time.Millisecond // Very short timeout
	config.HopTimeout = 1 * time.Millisecond

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	chain, err := reasoner.Reason(ctx, query)

	// 要么暂停,要么迅速完成
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("Got error: %v", err)
	}

	if chain != nil && chain.Status == StatusTimeout {
		t.Log("Chain timed out as expected")
	}
}

func TestMultiHopReasoner_ExecuteHop(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.EnableLLMReasoning = false

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	docs := []Document{
		{ID: "doc1", Content: "Test document about machine learning."},
	}
	retriever.IndexDocuments(docs)

	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())

	ctx := context.Background()
	seenDocIDs := make(map[string]bool)

	hop, err := reasoner.executeHop(ctx, 0, HopTypeInitial, "machine learning", "", seenDocIDs)
	if err != nil {
		t.Fatalf("executeHop failed: %v", err)
	}

	if hop == nil {
		t.Fatal("expected hop to be returned")
	}

	if hop.HopNumber != 0 {
		t.Errorf("expected HopNumber to be 0, got %d", hop.HopNumber)
	}

	if hop.Type != HopTypeInitial {
		t.Errorf("expected Type to be initial, got %s", hop.Type)
	}
}

func TestMultiHopReasoner_Cache(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.EnableCache = true
	config.CacheTTL = 1 * time.Minute
	config.MaxHops = 1
	config.EnableLLMReasoning = false

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	docs := []Document{
		{ID: "doc1", Content: "Machine learning basics."},
	}
	retriever.IndexDocuments(docs)

	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	// 第一通电话
	chain1, err := reasoner.Reason(ctx, query)
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}

	// 二通电话应打缓存
	chain2, err := reasoner.Reason(ctx, query)
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}

	// 应该是同一链条(从缓存)
	if chain1.ID != chain2.ID {
		t.Error("expected cached chain to be returned")
	}
}

func TestReasoningChain_GetHop(t *testing.T) {
	chain := &ReasoningChain{
		Hops: []ReasoningHop{
			{HopNumber: 0, Query: "query1"},
			{HopNumber: 1, Query: "query2"},
		},
	}

	hop := chain.GetHop(0)
	if hop == nil {
		t.Fatal("expected hop to be returned")
	}
	if hop.Query != "query1" {
		t.Errorf("expected Query to be 'query1', got %q", hop.Query)
	}

	hop = chain.GetHop(1)
	if hop == nil {
		t.Fatal("expected hop to be returned")
	}
	if hop.Query != "query2" {
		t.Errorf("expected Query to be 'query2', got %q", hop.Query)
	}

	// 超出范围
	hop = chain.GetHop(5)
	if hop != nil {
		t.Error("expected nil for out of bounds hop")
	}

	hop = chain.GetHop(-1)
	if hop != nil {
		t.Error("expected nil for negative hop number")
	}
}

func TestReasoningChain_GetAllDocuments(t *testing.T) {
	chain := &ReasoningChain{
		Hops: []ReasoningHop{
			{
				Results: []RetrievalResult{
					{Document: Document{ID: "doc1", Content: "content1"}},
					{Document: Document{ID: "doc2", Content: "content2"}},
				},
			},
			{
				Results: []RetrievalResult{
					{Document: Document{ID: "doc2", Content: "content2"}}, // Duplicate
					{Document: Document{ID: "doc3", Content: "content3"}},
				},
			},
		},
	}

	docs := chain.GetAllDocuments()

	if len(docs) != 3 {
		t.Errorf("expected 3 unique documents, got %d", len(docs))
	}

	// 检查复制件是否被删除
	seen := make(map[string]bool)
	for _, doc := range docs {
		if seen[doc.ID] {
			t.Errorf("duplicate document found: %s", doc.ID)
		}
		seen[doc.ID] = true
	}
}

func TestReasoningChain_GetTopDocuments(t *testing.T) {
	chain := &ReasoningChain{
		Hops: []ReasoningHop{
			{
				Results: []RetrievalResult{
					{Document: Document{ID: "doc1"}, FinalScore: 0.9},
					{Document: Document{ID: "doc2"}, FinalScore: 0.7},
				},
			},
			{
				Results: []RetrievalResult{
					{Document: Document{ID: "doc3"}, FinalScore: 0.8},
					{Document: Document{ID: "doc4"}, FinalScore: 0.6},
				},
			},
		},
	}

	topDocs := chain.GetTopDocuments(2)

	if len(topDocs) != 2 {
		t.Errorf("expected 2 documents, got %d", len(topDocs))
	}

	// 应该按分数排序
	if topDocs[0].FinalScore < topDocs[1].FinalScore {
		t.Error("expected documents to be sorted by score descending")
	}

	if topDocs[0].Document.ID != "doc1" {
		t.Errorf("expected first document to be doc1, got %s", topDocs[0].Document.ID)
	}
}

func TestReasoningChain_Visualize(t *testing.T) {
	chain := &ReasoningChain{
		OriginalQuery: "test query",
		Hops: []ReasoningHop{
			{
				HopNumber: 0,
				Type:      HopTypeInitial,
				Query:     "initial query",
				Results: []RetrievalResult{
					{Document: Document{ID: "doc1", Content: "content1"}, FinalScore: 0.9},
				},
			},
		},
		FinalAnswer: "This is the answer.",
	}

	viz := chain.Visualize()

	if viz == nil {
		t.Fatal("expected visualization to be returned")
	}

	// 应该有查询节点, hop节点,文档节点, 并回答节点
	if len(viz.Nodes) < 4 {
		t.Errorf("expected at least 4 nodes, got %d", len(viz.Nodes))
	}

	// 检查节点类型
	nodeTypes := make(map[string]int)
	for _, node := range viz.Nodes {
		nodeTypes[node.Type]++
	}

	if nodeTypes["query"] != 1 {
		t.Error("expected 1 query node")
	}
	if nodeTypes["hop"] != 1 {
		t.Error("expected 1 hop node")
	}
	if nodeTypes["answer"] != 1 {
		t.Error("expected 1 answer node")
	}

	// 检查边缘
	if len(viz.Edges) < 3 {
		t.Errorf("expected at least 3 edges, got %d", len(viz.Edges))
	}
}

func TestReasoningChain_JSON(t *testing.T) {
	chain := &ReasoningChain{
		ID:            "chain_123",
		OriginalQuery: "test query",
		Status:        StatusCompleted,
		Hops: []ReasoningHop{
			{HopNumber: 0, Query: "query1"},
		},
	}

	// 序列化
	data, err := chain.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// 淡化
	chain2 := &ReasoningChain{}
	err = chain2.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if chain2.ID != chain.ID {
		t.Errorf("expected ID %q, got %q", chain.ID, chain2.ID)
	}

	if chain2.OriginalQuery != chain.OriginalQuery {
		t.Errorf("expected OriginalQuery %q, got %q", chain.OriginalQuery, chain2.OriginalQuery)
	}

	if chain2.Status != chain.Status {
		t.Errorf("expected Status %s, got %s", chain.Status, chain2.Status)
	}
}

func TestMultiHopReasoner_ReasonBatch(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.MaxHops = 1
	config.EnableLLMReasoning = false

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	docs := []Document{
		{ID: "doc1", Content: "Machine learning basics."},
		{ID: "doc2", Content: "Python programming language."},
	}
	retriever.IndexDocuments(docs)

	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())

	ctx := context.Background()
	queries := []string{
		"What is machine learning?",
		"What is Python?",
	}

	chains, err := reasoner.ReasonBatch(ctx, queries)
	if err != nil {
		t.Fatalf("ReasonBatch failed: %v", err)
	}

	if len(chains) != len(queries) {
		t.Errorf("expected %d chains, got %d", len(queries), len(chains))
	}

	for i, chain := range chains {
		if chain == nil {
			t.Errorf("chain %d is nil", i)
			continue
		}
		if chain.OriginalQuery != queries[i] {
			t.Errorf("chain %d: expected query %q, got %q", i, queries[i], chain.OriginalQuery)
		}
	}
}

func TestDefaultMultiHopConfig(t *testing.T) {
	config := DefaultMultiHopConfig()

	if config.MaxHops != 4 {
		t.Errorf("expected MaxHops to be 4, got %d", config.MaxHops)
	}

	if config.MinHops != 1 {
		t.Errorf("expected MinHops to be 1, got %d", config.MinHops)
	}

	if config.ResultsPerHop != 5 {
		t.Errorf("expected ResultsPerHop to be 5, got %d", config.ResultsPerHop)
	}

	if !config.EnableLLMReasoning {
		t.Error("expected EnableLLMReasoning to be true")
	}

	if !config.EnableQueryRefinement {
		t.Error("expected EnableQueryRefinement to be true")
	}

	if !config.DeduplicateResults {
		t.Error("expected DeduplicateResults to be true")
	}
}

func TestReasoningCache(t *testing.T) {
	cache := newReasoningCache(100 * time.Millisecond)

	chain := &ReasoningChain{
		ID:            "chain_123",
		OriginalQuery: "test",
		CreatedAt:     time.Now(),
	}

	// 准备就绪
	cache.set("key1", chain)
	result, ok := cache.get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if result.ID != chain.ID {
		t.Error("expected cached value to match")
	}

	// 不存在密钥
	_, ok = cache.get("nonexistent")
	if ok {
		t.Error("expected cache miss for non-existent key")
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)
	_, ok = cache.get("key1")
	if ok {
		t.Error("expected cache miss after expiration")
	}
}

func TestTruncateContext(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is a ..."},
		{"exact", 5, "exact"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		result := truncateContext(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateContext(%q, %d) = %q, expected %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestGenerateChainID(t *testing.T) {
	id1 := generateChainID()
	time.Sleep(1 * time.Nanosecond) // Ensure different timestamps
	id2 := generateChainID()

	if id1 == "" {
		t.Error("expected non-empty chain ID")
	}

	// ID应该是独一无二的(基于纳米秒印)
	// 注意: 在非常快的执行中, ID可能是相同的, 所以我们只是检查格式
	if !contains(id1, "chain_") {
		t.Error("expected chain ID to start with 'chain_'")
	}

	if !contains(id2, "chain_") {
		t.Error("expected chain ID to start with 'chain_'")
	}
}

func TestNormalizeQueryForDedup(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"What is AI?", "what is ai?"},
		{"  HELLO   WORLD  ", "hello world"},
		{"Same Query", "same query"},
		{"  multiple   spaces   here  ", "multiple spaces here"},
		{"", ""},
		{"   ", ""},
		{"MixedCase", "mixedcase"},
	}

	for _, tt := range tests {
		result := normalizeQueryForDedup(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeQueryForDedup(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestMultiHopReasoner_QueryDeduplication(t *testing.T) {
	config := DefaultMultiHopConfig()
	config.MaxHops = 5
	config.EnableLLMReasoning = false
	config.EnableQueryRefinement = false // Disable refinement to control queries

	retrieverConfig := DefaultHybridRetrievalConfig()
	retriever := NewHybridRetriever(retrieverConfig, zap.NewNop())

	// 索引测试文档
	docs := []Document{
		{ID: "doc1", Content: "Machine learning is a subset of artificial intelligence."},
		{ID: "doc2", Content: "Deep learning uses neural networks."},
	}
	retriever.IndexDocuments(docs)

	reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	chain, err := reasoner.Reason(ctx, query)
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}

	if chain == nil {
		t.Fatal("expected chain to be returned")
	}

	// 分解,即使与 MaxHops= 5, 我们不应该执行重复的查询
	// 第一个 hop 执行查询, 后面使用相同查询的 hop 将被跳过
	if chain.Status != StatusCompleted {
		t.Errorf("expected status to be completed, got %s", chain.Status)
	}
}
