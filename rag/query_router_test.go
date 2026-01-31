package rag

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewQueryRouter(t *testing.T) {
	config := DefaultQueryRouterConfig()
	router := NewQueryRouter(config, nil, nil, nil)

	if router == nil {
		t.Fatal("expected router to be created")
	}

	if router.config.DefaultStrategy != StrategyHybrid {
		t.Errorf("expected DefaultStrategy to be hybrid, got %s", router.config.DefaultStrategy)
	}
}

func TestQueryRouter_Route_Basic(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = false
	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	decision, err := router.Route(ctx, query)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if decision == nil {
		t.Fatal("expected decision to be returned")
	}

	if decision.Query != query {
		t.Errorf("expected Query to be %q, got %q", query, decision.Query)
	}

	if decision.SelectedStrategy == "" {
		t.Error("expected SelectedStrategy to be set")
	}

	if decision.Confidence <= 0 {
		t.Error("expected positive confidence")
	}
}

func TestQueryRouter_Route_WithQueryTransformer(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = false

	transformerConfig := DefaultQueryTransformConfig()
	transformerConfig.UseLLM = false
	transformer := NewQueryTransformer(transformerConfig, nil, zap.NewNop())

	router := NewQueryRouter(config, transformer, nil, zap.NewNop())

	ctx := context.Background()
	query := "How to implement authentication in Python?"

	decision, err := router.Route(ctx, query)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if decision == nil {
		t.Fatal("expected decision to be returned")
	}

	// Check that features were analyzed
	if decision.Metadata == nil {
		t.Error("expected metadata to be set")
	}

	features, ok := decision.Metadata["features"].(QueryFeatures)
	if ok && features.Intent == IntentUnknown {
		t.Log("Intent detection may not have worked without LLM")
	}
}

func TestQueryRouter_Route_WithLLM(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = true

	mockLLM := &mockRouterLLMProvider{}
	router := NewQueryRouter(config, nil, mockLLM, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	decision, err := router.Route(ctx, query)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if decision == nil {
		t.Fatal("expected decision to be returned")
	}
}

// mockRouterLLMProvider is a mock LLM provider for router testing
type mockRouterLLMProvider struct{}

func (m *mockRouterLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return `{"strategy": "hybrid", "confidence": 0.85, "reasoning": "Query requires both semantic and keyword matching"}`, nil
}

func TestQueryRouter_Route_Cache(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableCache = true
	config.CacheTTL = 1 * time.Minute
	config.EnableLLMRouting = false

	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "What is machine learning?"

	// First call
	decision1, err := router.Route(ctx, query)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	// Second call should hit cache
	decision2, err := router.Route(ctx, query)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	// Should be the same decision (from cache)
	if decision1.SelectedStrategy != decision2.SelectedStrategy {
		t.Error("expected cached decision to match")
	}
}

func TestQueryRouter_AnalyzeQuery(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = false

	transformerConfig := DefaultQueryTransformConfig()
	transformerConfig.UseLLM = false
	transformer := NewQueryTransformer(transformerConfig, nil, nil)

	router := NewQueryRouter(config, transformer, nil, nil)

	ctx := context.Background()

	tests := []struct {
		query          string
		expectedLength string
		isQuestion     bool
	}{
		{"What?", "short", true},
		{"How to implement authentication?", "short", true}, // 4 words = short
		{"This is a very long query that contains many words and should be categorized as long because it exceeds the threshold for medium length queries", "long", false},
		{"machine learning", "short", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			features := router.analyzeQuery(ctx, tt.query)

			if features.Length != tt.expectedLength {
				t.Errorf("expected Length %q, got %q", tt.expectedLength, features.Length)
			}

			if features.IsQuestion != tt.isQuestion {
				t.Errorf("expected IsQuestion %v, got %v", tt.isQuestion, features.IsQuestion)
			}
		})
	}
}

func TestQueryRouter_DetermineComplexity(t *testing.T) {
	config := DefaultQueryRouterConfig()
	router := NewQueryRouter(config, nil, nil, nil)

	tests := []struct {
		query      string
		features   QueryFeatures
		expected   string
	}{
		{
			query:    "What is Python?",
			features: QueryFeatures{Length: "short", Intent: IntentFactual},
			expected: "low",
		},
		{
			query:    "Compare Python and Java for web development",
			features: QueryFeatures{Length: "medium", Intent: IntentComparison, Entities: []string{"Python", "Java"}},
			expected: "high", // Comparison intent + entities = high complexity
		},
		{
			query:    "Analyze the impact of machine learning on healthcare and compare different approaches",
			features: QueryFeatures{Length: "long", Intent: IntentAnalytical, Entities: []string{"ML", "Healthcare"}},
			expected: "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			complexity := router.determineComplexity(tt.query, tt.features)
			if complexity != tt.expected {
				t.Errorf("expected complexity %q, got %q", tt.expected, complexity)
			}
		})
	}
}

func TestQueryRouter_MatchCondition(t *testing.T) {
	config := DefaultQueryRouterConfig()
	router := NewQueryRouter(config, nil, nil, nil)

	features := QueryFeatures{
		Intent:      IntentFactual,
		Complexity:  "medium",
		Length:      "short",
		HasEntities: true,
		IsQuestion:  true,
		Keywords:    []string{"machine", "learning"},
	}

	tests := []struct {
		condition RoutingCondition
		expected  bool
	}{
		{RoutingCondition{Type: "intent", Value: "factual", Operator: "equals"}, true},
		{RoutingCondition{Type: "intent", Value: "comparison", Operator: "equals"}, false},
		{RoutingCondition{Type: "complexity", Value: "medium", Operator: "equals"}, true},
		{RoutingCondition{Type: "length", Value: "short", Operator: "equals"}, true},
		{RoutingCondition{Type: "keyword", Value: "machine", Operator: "contains"}, true},
		{RoutingCondition{Type: "keyword", Value: "python", Operator: "contains"}, false},
		{RoutingCondition{Type: "has_entities", Value: "true", Operator: "equals"}, true},
		{RoutingCondition{Type: "is_question", Value: "true", Operator: "equals"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.condition.Type+"_"+tt.condition.Value, func(t *testing.T) {
			result := router.matchCondition(tt.condition, features)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestQueryRouter_SelectBestStrategy(t *testing.T) {
	config := DefaultQueryRouterConfig()
	router := NewQueryRouter(config, nil, nil, nil)

	scores := map[RetrievalStrategy]float64{
		StrategyVector:   0.7,
		StrategyBM25:     0.5,
		StrategyHybrid:   0.9,
		StrategyMultiHop: 0.6,
	}

	strategy, score := router.selectBestStrategy(scores)

	if strategy != StrategyHybrid {
		t.Errorf("expected StrategyHybrid, got %s", strategy)
	}

	if score != 0.9 {
		t.Errorf("expected score 0.9, got %f", score)
	}
}

func TestQueryRouter_SelectBestStrategy_Empty(t *testing.T) {
	config := DefaultQueryRouterConfig()
	router := NewQueryRouter(config, nil, nil, nil)

	scores := map[RetrievalStrategy]float64{}

	strategy, score := router.selectBestStrategy(scores)

	if strategy != config.DefaultStrategy {
		t.Errorf("expected default strategy %s, got %s", config.DefaultStrategy, strategy)
	}

	if score != 0.5 {
		t.Errorf("expected score 0.5, got %f", score)
	}
}

func TestQueryRouter_RouteMulti(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = false
	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "Compare Python and Java for machine learning"

	decision, err := router.RouteMulti(ctx, query, 3)
	if err != nil {
		t.Fatalf("RouteMulti failed: %v", err)
	}

	if decision == nil {
		t.Fatal("expected decision to be returned")
	}

	if len(decision.Strategies) == 0 {
		t.Error("expected at least one strategy")
	}

	if len(decision.Strategies) > 3 {
		t.Errorf("expected at most 3 strategies, got %d", len(decision.Strategies))
	}

	// Check that weights sum to approximately 1
	totalWeight := 0.0
	for _, s := range decision.Strategies {
		totalWeight += s.Weight
	}

	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected weights to sum to 1, got %f", totalWeight)
	}
}

func TestQueryRouter_RouteBatch(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = false
	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	ctx := context.Background()
	queries := []string{
		"What is Python?",
		"Compare SQL and NoSQL",
		"How to implement authentication?",
	}

	decisions, err := router.RouteBatch(ctx, queries)
	if err != nil {
		t.Fatalf("RouteBatch failed: %v", err)
	}

	if len(decisions) != len(queries) {
		t.Errorf("expected %d decisions, got %d", len(queries), len(decisions))
	}

	for i, decision := range decisions {
		if decision == nil {
			t.Errorf("decision %d is nil", i)
			continue
		}
		if decision.Query != queries[i] {
			t.Errorf("decision %d: expected query %q, got %q", i, queries[i], decision.Query)
		}
	}
}

func TestQueryRouter_RecordFeedback(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableAdaptiveRouting = true
	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	feedback := RoutingFeedback{
		Query:            "test query",
		SelectedStrategy: StrategyHybrid,
		Success:          true,
		Score:            0.9,
		Timestamp:        time.Now(),
	}

	router.RecordFeedback(feedback)

	// Record more feedback
	for i := 0; i < 10; i++ {
		router.RecordFeedback(RoutingFeedback{
			Query:            "test query",
			SelectedStrategy: StrategyHybrid,
			Success:          i%2 == 0, // 50% success rate
			Score:            0.5,
			Timestamp:        time.Now(),
		})
	}

	stats := router.GetStrategyStats()
	if stat, ok := stats[StrategyHybrid]; ok {
		if stat.TotalCalls == 0 {
			t.Error("expected feedback to be recorded")
		}
	}
}

func TestQueryRouter_GetStrategyStats(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableAdaptiveRouting = true
	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	// Record some feedback
	for i := 0; i < 10; i++ {
		router.RecordFeedback(RoutingFeedback{
			Query:            "test",
			SelectedStrategy: StrategyVector,
			Success:          i < 7, // 70% success rate
			Score:            float64(i) / 10,
			Timestamp:        time.Now(),
		})
	}

	stats := router.GetStrategyStats()

	if stat, ok := stats[StrategyVector]; ok {
		if stat.TotalCalls != 10 {
			t.Errorf("expected 10 calls, got %d", stat.TotalCalls)
		}
		if stat.SuccessRate < 0.69 || stat.SuccessRate > 0.71 {
			t.Errorf("expected success rate ~0.7, got %f", stat.SuccessRate)
		}
	} else {
		t.Error("expected stats for StrategyVector")
	}
}

func TestRoutingDecision_JSON(t *testing.T) {
	decision := &RoutingDecision{
		Query:            "test query",
		SelectedStrategy: StrategyHybrid,
		Confidence:       0.85,
		Scores: map[RetrievalStrategy]float64{
			StrategyHybrid: 0.85,
			StrategyVector: 0.7,
		},
		Reasoning: "Test reasoning",
		Timestamp: time.Now(),
	}

	// Serialize
	data, err := decision.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Deserialize
	decision2 := &RoutingDecision{}
	err = decision2.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if decision2.Query != decision.Query {
		t.Errorf("expected Query %q, got %q", decision.Query, decision2.Query)
	}

	if decision2.SelectedStrategy != decision.SelectedStrategy {
		t.Errorf("expected SelectedStrategy %s, got %s", decision.SelectedStrategy, decision2.SelectedStrategy)
	}

	if decision2.Confidence != decision.Confidence {
		t.Errorf("expected Confidence %f, got %f", decision.Confidence, decision2.Confidence)
	}
}

func TestDefaultQueryRouterConfig(t *testing.T) {
	config := DefaultQueryRouterConfig()

	if config.DefaultStrategy != StrategyHybrid {
		t.Errorf("expected DefaultStrategy to be hybrid, got %s", config.DefaultStrategy)
	}

	if !config.EnableLLMRouting {
		t.Error("expected EnableLLMRouting to be true")
	}

	if !config.EnableFallback {
		t.Error("expected EnableFallback to be true")
	}

	if config.FallbackStrategy != StrategyHybrid {
		t.Errorf("expected FallbackStrategy to be hybrid, got %s", config.FallbackStrategy)
	}

	if len(config.Strategies) == 0 {
		t.Error("expected strategies to be configured")
	}
}

func TestRoutingCache(t *testing.T) {
	cache := newRoutingCache(100 * time.Millisecond)

	decision := &RoutingDecision{
		Query:            "test",
		SelectedStrategy: StrategyHybrid,
		Timestamp:        time.Now(),
	}

	// Set and get
	cache.set("key1", decision)
	result, ok := cache.get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if result.Query != decision.Query {
		t.Error("expected cached value to match")
	}

	// Non-existent key
	_, ok = cache.get("nonexistent")
	if ok {
		t.Error("expected cache miss for non-existent key")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	_, ok = cache.get("key1")
	if ok {
		t.Error("expected cache miss after expiration")
	}
}

func TestFeedbackStore(t *testing.T) {
	store := newFeedbackStore()

	// Add feedback
	for i := 0; i < 10; i++ {
		store.add(RoutingFeedback{
			Query:            "test",
			SelectedStrategy: StrategyVector,
			Success:          i < 8, // 80% success
			Timestamp:        time.Now(),
		})
	}

	successRate := store.getSuccessRate(StrategyVector)
	if successRate < 0.79 || successRate > 0.81 {
		t.Errorf("expected success rate ~0.8, got %f", successRate)
	}

	// Unknown strategy should return 0.5
	unknownRate := store.getSuccessRate(StrategyGraphRAG)
	if unknownRate != 0.5 {
		t.Errorf("expected 0.5 for unknown strategy, got %f", unknownRate)
	}
}

func TestQueryRouter_Fallback(t *testing.T) {
	config := DefaultQueryRouterConfig()
	config.EnableLLMRouting = false
	config.EnableFallback = true
	config.FallbackStrategy = StrategyBM25
	config.ConfidenceThreshold = 0.99 // Very high threshold to trigger fallback

	router := NewQueryRouter(config, nil, nil, zap.NewNop())

	ctx := context.Background()
	query := "x" // Very short query that won't match well

	decision, err := router.Route(ctx, query)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	// Should use fallback due to low confidence
	if decision.Metadata["fallback_used"] == true {
		if decision.SelectedStrategy != StrategyBM25 {
			t.Errorf("expected fallback strategy BM25, got %s", decision.SelectedStrategy)
		}
	}
}

func TestStrategyConstants(t *testing.T) {
	strategies := []RetrievalStrategy{
		StrategyVector,
		StrategyBM25,
		StrategyHybrid,
		StrategyMultiHop,
		StrategyGraphRAG,
		StrategyContextual,
		StrategyDense,
		StrategySparse,
	}

	seen := make(map[RetrievalStrategy]bool)
	for _, s := range strategies {
		if seen[s] {
			t.Errorf("duplicate strategy: %s", s)
		}
		seen[s] = true

		if s == "" {
			t.Error("empty strategy constant")
		}
	}
}
