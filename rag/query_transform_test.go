package rag

import (
	"context"
	"testing"
	"time"
)

// 模拟LLMProvider是用于测试的模拟LLM提供者
type mockLLMProvider struct {
	responses map[string]string
	callCount int
}

func newMockLLMProvider() *mockLLMProvider {
	return &mockLLMProvider{
		responses: make(map[string]string),
	}
}

func (m *mockLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	m.callCount++

	// 基于即时内容返回预定义的回复
	if contains(prompt, "alternative search queries") {
		return "1. What is machine learning\n2. ML basics explained\n3. Introduction to machine learning", nil
	}
	if contains(prompt, "Break down") {
		return "1. What is the capital of France?\n2. What is the population of Paris?", nil
	}
	if contains(prompt, "Rewrite") {
		return "machine learning fundamentals concepts", nil
	}
	if contains(prompt, "Classify the following query") {
		return "factual, 0.85", nil
	}
	if contains(prompt, "hypothetical document") {
		return "Machine learning is a subset of artificial intelligence that enables systems to learn from data.", nil
	}
	if contains(prompt, "step-back") {
		return "What are the fundamental concepts of artificial intelligence?", nil
	}

	return "default response", nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNewQueryTransformer(t *testing.T) {
	config := DefaultQueryTransformConfig()
	transformer := NewQueryTransformer(config, nil, nil)

	if transformer == nil {
		t.Fatal("expected transformer to be created")
	}

	if transformer.config.MaxExpansions != 3 {
		t.Errorf("expected MaxExpansions to be 3, got %d", transformer.config.MaxExpansions)
	}
}

func TestQueryTransformer_Transform(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false // Use rule-based for predictable testing
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	query := "How do I implement machine learning in Python?"

	result, err := transformer.Transform(ctx, query)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if result.Original != query {
		t.Errorf("expected Original to be %q, got %q", query, result.Original)
	}

	if result.Intent == "" {
		t.Error("expected Intent to be detected")
	}

	if len(result.Keywords) == 0 {
		t.Error("expected Keywords to be extracted")
	}
}

func TestQueryTransformer_Transform_WithLLM(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = true
	mockLLM := newMockLLMProvider()
	transformer := NewQueryTransformer(config, mockLLM, nil)

	ctx := context.Background()
	query := "What is machine learning?"

	result, err := transformer.Transform(ctx, query)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if result.Original != query {
		t.Errorf("expected Original to be %q, got %q", query, result.Original)
	}
}

func TestQueryTransformer_Expand(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	query := "how to implement authentication"

	expansions, err := transformer.Expand(ctx, query)
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(expansions) == 0 {
		t.Error("expected at least one expansion")
	}

	// 请列入原始查询
	found := false
	for _, exp := range expansions {
		if exp == query {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected original query to be in expansions")
	}
}

func TestQueryTransformer_Expand_WithLLM(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = true
	mockLLM := newMockLLMProvider()
	transformer := NewQueryTransformer(config, mockLLM, nil)

	ctx := context.Background()
	query := "What is machine learning?"

	expansions, err := transformer.Expand(ctx, query)
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(expansions) < 2 {
		t.Errorf("expected multiple expansions, got %d", len(expansions))
	}
}

func TestQueryTransformer_DetectIntent(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	tests := []struct {
		query    string
		expected []QueryIntent // Accept multiple valid intents due to map iteration order
	}{
		{"What is machine learning?", []QueryIntent{IntentFactual}},
		{"How to implement authentication?", []QueryIntent{IntentProcedural}},
		{"Compare Python and Java", []QueryIntent{IntentComparison}},
		{"Explain how neural networks work", []QueryIntent{IntentExplanation, IntentComparison}}, // Both valid
		{"Analyze the impact of AI on jobs", []QueryIntent{IntentAnalytical}},
		{"List all programming languages", []QueryIntent{IntentAggregation}},
		{"What if we had no internet?", []QueryIntent{IntentHypothetical}},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent, confidence := transformer.detectIntent(ctx, tt.query)
			found := false
			for _, expected := range tt.expected {
				if intent == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected intent in %v, got %s (confidence: %.2f)", tt.expected, intent, confidence)
			}
		})
	}
}

func TestQueryTransformer_ExtractKeywords(t *testing.T) {
	config := DefaultQueryTransformConfig()
	transformer := NewQueryTransformer(config, nil, nil)

	query := "How to implement machine learning algorithms in Python?"
	keywords := transformer.extractKeywords(query)

	if len(keywords) == 0 {
		t.Error("expected keywords to be extracted")
	}

	// 请检查停止的单词被删除
	for _, kw := range keywords {
		if kw == "how" || kw == "to" || kw == "in" {
			t.Errorf("stop word %q should not be in keywords", kw)
		}
	}
}

func TestQueryTransformer_ExtractEntities(t *testing.T) {
	config := DefaultQueryTransformConfig()
	transformer := NewQueryTransformer(config, nil, nil)

	query := "How does Google use TensorFlow for machine learning?"
	entities := transformer.extractEntities(query)

	// 应找到资本字( Google, TensorFlow)
	if len(entities) == 0 {
		t.Error("expected entities to be extracted")
	}
}

func TestQueryTransformer_ShouldDecompose(t *testing.T) {
	config := DefaultQueryTransformConfig()
	transformer := NewQueryTransformer(config, nil, nil)

	tests := []struct {
		query    string
		intent   QueryIntent
		expected bool
	}{
		{"What is Python?", IntentFactual, false},
		{"Compare Python and Java and explain their differences", IntentComparison, true},
		{"What is the capital of France and what is its population?", IntentFactual, true},
		{"Analyze the impact of AI on healthcare", IntentAnalytical, true},
		{"This is a very long query that contains many words and should be decomposed because it exceeds the threshold", IntentFactual, true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := transformer.shouldDecompose(tt.query, tt.intent)
			if result != tt.expected {
				t.Errorf("expected shouldDecompose to be %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestQueryTransformer_Decompose(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	query := "What is Python and how does it compare to Java?"

	subQueries, err := transformer.decompose(ctx, query)
	if err != nil {
		t.Fatalf("decompose failed: %v", err)
	}

	if len(subQueries) < 2 {
		t.Errorf("expected at least 2 sub-queries, got %d", len(subQueries))
	}
}

func TestQueryTransformer_Rewrite(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	query := "Can you tell me how to implement authentication?"

	rewritten, err := transformer.rewrite(ctx, query, IntentProcedural)
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	// 应删除填充词
	if rewritten == query {
		t.Error("expected query to be rewritten")
	}

	if contains(rewritten, "can you tell me") {
		t.Error("expected filler words to be removed")
	}
}

func TestQueryTransformer_GenerateHyDE(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.EnableHyDE = true
	mockLLM := newMockLLMProvider()
	transformer := NewQueryTransformer(config, mockLLM, nil)

	ctx := context.Background()
	query := "What is machine learning?"

	hydeDoc, err := transformer.generateHyDE(ctx, query)
	if err != nil {
		t.Fatalf("generateHyDE failed: %v", err)
	}

	if hydeDoc == "" {
		t.Error("expected HyDE document to be generated")
	}
}

func TestQueryTransformer_StepBack(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.EnableStepBack = true
	mockLLM := newMockLLMProvider()
	transformer := NewQueryTransformer(config, mockLLM, nil)

	ctx := context.Background()
	query := "How does backpropagation work in neural networks?"

	stepBackQuery, err := transformer.stepBack(ctx, query)
	if err != nil {
		t.Fatalf("stepBack failed: %v", err)
	}

	if stepBackQuery == "" {
		t.Error("expected step-back query to be generated")
	}
}

func TestQueryTransformer_Cache(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.EnableCache = true
	config.CacheTTL = 1 * time.Minute
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	query := "What is machine learning?"

	// 第一通电话
	result1, err := transformer.Transform(ctx, query)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	// 二通电话应打缓存
	result2, err := transformer.Transform(ctx, query)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	// 结果应该是一样的
	if result1.Transformed != result2.Transformed {
		t.Error("expected cached result to match")
	}
}

func TestQueryTransformer_TransformBatch(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	queries := []string{
		"What is Python?",
		"How to implement authentication?",
		"Compare SQL and NoSQL databases",
	}

	results, err := transformer.TransformBatch(ctx, queries)
	if err != nil {
		t.Fatalf("TransformBatch failed: %v", err)
	}

	if len(results) != len(queries) {
		t.Errorf("expected %d results, got %d", len(queries), len(results))
	}

	for i, result := range results {
		if result == nil {
			t.Errorf("result %d is nil", i)
			continue
		}
		if result.Original != queries[i] {
			t.Errorf("result %d: expected Original %q, got %q", i, queries[i], result.Original)
		}
	}
}

func TestQueryTransformer_ExpandWithMetadata(t *testing.T) {
	config := DefaultQueryTransformConfig()
	config.UseLLM = false
	transformer := NewQueryTransformer(config, nil, nil)

	ctx := context.Background()
	query := "How to implement machine learning?"

	result, err := transformer.ExpandWithMetadata(ctx, query)
	if err != nil {
		t.Fatalf("ExpandWithMetadata failed: %v", err)
	}

	if result.Original != query {
		t.Errorf("expected Original to be %q, got %q", query, result.Original)
	}

	if len(result.Expansions) == 0 {
		t.Error("expected expansions")
	}

	if len(result.Keywords) == 0 {
		t.Error("expected keywords")
	}
}

func TestTransformedQuery_JSON(t *testing.T) {
	tq := &TransformedQuery{
		Original:    "test query",
		Transformed: "transformed query",
		Type:        TransformRewrite,
		Intent:      IntentFactual,
		Confidence:  0.85,
		Keywords:    []string{"test", "query"},
	}

	// 序列化
	data, err := tq.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// 淡化
	tq2 := &TransformedQuery{}
	err = tq2.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if tq2.Original != tq.Original {
		t.Errorf("expected Original %q, got %q", tq.Original, tq2.Original)
	}

	if tq2.Intent != tq.Intent {
		t.Errorf("expected Intent %s, got %s", tq.Intent, tq2.Intent)
	}
}

func TestDefaultQueryTransformConfig(t *testing.T) {
	config := DefaultQueryTransformConfig()

	if !config.EnableExpansion {
		t.Error("expected EnableExpansion to be true")
	}

	if config.MaxExpansions != 3 {
		t.Errorf("expected MaxExpansions to be 3, got %d", config.MaxExpansions)
	}

	if !config.EnableRewriting {
		t.Error("expected EnableRewriting to be true")
	}

	if !config.EnableDecomposition {
		t.Error("expected EnableDecomposition to be true")
	}

	if !config.EnableIntentDetection {
		t.Error("expected EnableIntentDetection to be true")
	}

	if config.EnableHyDE {
		t.Error("expected EnableHyDE to be false by default")
	}

	if config.EnableStepBack {
		t.Error("expected EnableStepBack to be false by default")
	}
}

func TestTransformCache(t *testing.T) {
	cache := newTransformCache(100 * time.Millisecond)

	tq := &TransformedQuery{
		Original:    "test",
		Transformed: "transformed",
	}

	// 准备就绪
	cache.set("key1", tq)
	result, ok := cache.get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if result.Original != tq.Original {
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
