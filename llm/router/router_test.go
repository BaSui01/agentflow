package router

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm/config"

	"go.uber.org/zap"
)

func TestWeightedRouter_Select(t *testing.T) {
	logger := zap.NewNop()
	router := NewWeightedRouter(logger, []config.PrefixRule{})

	// 加载测试配置
	cfg := &config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				Code:    "openai",
				Enabled: true,
				Models: []config.ModelConfig{
					{ID: "m1", Name: "gpt-4o", PriceInput: 0.005, PriceOutput: 0.015, Tags: []string{"fast"}, Enabled: true},
					{ID: "m2", Name: "gpt-3.5", PriceInput: 0.0005, PriceOutput: 0.0015, Tags: []string{"cheap"}, Enabled: true},
				},
			},
		},
	}
	router.LoadCandidates(cfg)

	// 设置健康状态
	router.UpdateHealth("m1", &ModelHealth{ModelID: "m1", IsHealthy: true, SuccessRate: 0.99, AvgLatencyMs: 200})
	router.UpdateHealth("m2", &ModelHealth{ModelID: "m2", IsHealthy: true, SuccessRate: 0.95, AvgLatencyMs: 300})

	tests := []struct {
		name    string
		req     *RouteRequest
		wantErr bool
	}{
		{
			name:    "basic select",
			req:     &RouteRequest{TaskType: "chat"},
			wantErr: false,
		},
		{
			name:    "select with tags",
			req:     &RouteRequest{TaskType: "chat", Tags: []string{"cheap"}},
			wantErr: false,
		},
		{
			name:    "select with cost limit",
			req:     &RouteRequest{TaskType: "chat", MaxCost: 0.01},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.Select(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Select() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("Select() returned nil result")
			}
		})
	}
}

func TestWeightedRouter_FilterByHealth(t *testing.T) {
	logger := zap.NewNop()
	router := NewWeightedRouter(logger, []config.PrefixRule{})

	cfg := &config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"test": {
				Code:    "test",
				Enabled: true,
				Models: []config.ModelConfig{
					{ID: "healthy", Name: "healthy-model", Enabled: true},
					{ID: "unhealthy", Name: "unhealthy-model", Enabled: true},
				},
			},
		},
	}
	router.LoadCandidates(cfg)

	router.UpdateHealth("healthy", &ModelHealth{ModelID: "healthy", IsHealthy: true})
	router.UpdateHealth("unhealthy", &ModelHealth{ModelID: "unhealthy", IsHealthy: false})

	result, err := router.Select(context.Background(), &RouteRequest{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if result.ModelID != "healthy" {
		t.Errorf("Expected healthy model, got %s", result.ModelID)
	}
}

func TestWeightedRouter_FilterBySLA(t *testing.T) {
	logger := zap.NewNop()
	router := NewWeightedRouter(logger, []config.PrefixRule{})

	cfg := &config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"test": {
				Code:    "test",
				Enabled: true,
				Models: []config.ModelConfig{
					{ID: "fast", Name: "fast-model", Enabled: true},
					{ID: "slow", Name: "slow-model", Enabled: true},
				},
			},
		},
		RoutingWeights: map[string][]config.RoutingWeight{
			"default": {
				{ModelID: "fast", Weight: 100, MaxLatencyMs: 500, Enabled: true},
				{ModelID: "slow", Weight: 100, MaxLatencyMs: 1000, Enabled: true},
			},
		},
	}
	router.LoadCandidates(cfg)

	router.UpdateHealth("fast", &ModelHealth{ModelID: "fast", IsHealthy: true, AvgLatencyMs: 200})
	router.UpdateHealth("slow", &ModelHealth{ModelID: "slow", IsHealthy: true, AvgLatencyMs: 800})

	// 请求要求 300ms 以内，只有 fast 符合
	result, err := router.Select(context.Background(), &RouteRequest{MaxLatencyMs: 300})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if result.ModelID != "fast" {
		t.Errorf("Expected fast model, got %s", result.ModelID)
	}
}
