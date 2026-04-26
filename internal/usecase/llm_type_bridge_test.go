package usecase

import (
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func TestLLMTypeBridge_WebSearchOptions(t *testing.T) {
	bridge := NewLLMTypeBridge(nil)

	t.Run("nil input returns nil", func(t *testing.T) {
		if got := bridge.ToLLMWebSearchOptions(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
		if got := bridge.FromLLMWebSearchOptions(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("converts usecase to llm WebSearchOptions", func(t *testing.T) {
		input := &WebSearchOptions{
			SearchContextSize: "high",
			UserLocation: &WebSearchLocation{
				Type:     "approximate",
				Country:  "US",
				Region:   "California",
				City:     "San Francisco",
				Timezone: "America/Los_Angeles",
			},
			AllowedDomains: []string{"example.com", "test.com"},
			BlockedDomains: []string{"spam.com"},
			MaxUses:        5,
		}

		got := bridge.ToLLMWebSearchOptions(input)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.SearchContextSize != "high" {
			t.Errorf("SearchContextSize: expected high, got %s", got.SearchContextSize)
		}
		if got.UserLocation == nil {
			t.Fatal("expected UserLocation")
		}
		if got.UserLocation.Country != "US" {
			t.Errorf("UserLocation.Country: expected US, got %s", got.UserLocation.Country)
		}
		if len(got.AllowedDomains) != 2 {
			t.Errorf("AllowedDomains: expected 2, got %d", len(got.AllowedDomains))
		}
		if got.MaxUses != 5 {
			t.Errorf("MaxUses: expected 5, got %d", got.MaxUses)
		}
	})

	t.Run("converts llm to usecase WebSearchOptions", func(t *testing.T) {
		input := &llmcore.WebSearchOptions{
			SearchContextSize: "medium",
			UserLocation: &llmcore.WebSearchLocation{
				Type:     "approximate",
				Country:  "CN",
				Region:   "Beijing",
				City:     "Beijing",
				Timezone: "Asia/Shanghai",
			},
			AllowedDomains: []string{"baidu.com"},
			MaxUses:        3,
		}

		got := bridge.FromLLMWebSearchOptions(input)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.SearchContextSize != "medium" {
			t.Errorf("SearchContextSize: expected medium, got %s", got.SearchContextSize)
		}
		if got.UserLocation == nil {
			t.Fatal("expected UserLocation")
		}
		if got.UserLocation.Country != "CN" {
			t.Errorf("UserLocation.Country: expected CN, got %s", got.UserLocation.Country)
		}
	})
}

func TestLLMTypeBridge_MergeWebSearchOptions(t *testing.T) {
	bridge := NewLLMTypeBridge(nil).(*DefaultLLMTypeBridge)

	t.Run("both nil returns nil", func(t *testing.T) {
		if got := bridge.MergeLLMWebSearchOptions(nil, nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("override takes precedence", func(t *testing.T) {
		base := &llmcore.WebSearchOptions{
			SearchContextSize: "low",
			MaxUses:           1,
		}
		override := &llmcore.WebSearchOptions{
			SearchContextSize: "high",
			MaxUses:           5,
		}

		got := bridge.MergeLLMWebSearchOptions(base, override)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.SearchContextSize != "high" {
			t.Errorf("SearchContextSize: expected high, got %s", got.SearchContextSize)
		}
		if got.MaxUses != 5 {
			t.Errorf("MaxUses: expected 5, got %d", got.MaxUses)
		}
	})

	t.Run("merge user location", func(t *testing.T) {
		base := &llmcore.WebSearchOptions{
			UserLocation: &llmcore.WebSearchLocation{
				Country: "US",
			},
		}
		override := &llmcore.WebSearchOptions{
			UserLocation: &llmcore.WebSearchLocation{
				Region: "California",
			},
		}

		got := bridge.MergeLLMWebSearchOptions(base, override)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.UserLocation == nil {
			t.Fatal("expected UserLocation")
		}
		if got.UserLocation.Country != "US" {
			t.Errorf("Country: expected US, got %s", got.UserLocation.Country)
		}
		if got.UserLocation.Region != "California" {
			t.Errorf("Region: expected California, got %s", got.UserLocation.Region)
		}
	})
}

func TestLLMTypeBridge_APIKey(t *testing.T) {
	bridge := NewLLMTypeBridge(nil)

	t.Run("converts CreateAPIKeyInput to LLMProviderAPIKey", func(t *testing.T) {
		enabled := true
		input := CreateAPIKeyInput{
			APIKey:       "sk-test-1234567890",
			BaseURL:      "https://api.example.com",
			Label:        "Test Key",
			Priority:     50,
			Weight:       80,
			Enabled:      &enabled,
			RateLimitRPM: 100,
			RateLimitRPD: 1000,
		}

		got := bridge.ToLLMProviderAPIKey(1, input)
		if got.ProviderID != 1 {
			t.Errorf("ProviderID: expected 1, got %d", got.ProviderID)
		}
		if got.APIKey != "sk-test-1234567890" {
			t.Errorf("APIKey: expected sk-test-1234567890, got %s", got.APIKey)
		}
		if got.Priority != 50 {
			t.Errorf("Priority: expected 50, got %d", got.Priority)
		}
		if !got.Enabled {
			t.Error("Enabled: expected true")
		}
	})

	t.Run("default priority and weight", func(t *testing.T) {
		input := CreateAPIKeyInput{
			APIKey: "sk-test",
		}

		got := bridge.ToLLMProviderAPIKey(1, input)
		if got.Priority != 100 {
			t.Errorf("Priority: expected default 100, got %d", got.Priority)
		}
		if got.Weight != 100 {
			t.Errorf("Weight: expected default 100, got %d", got.Weight)
		}
	})

	t.Run("converts LLMProviderAPIKey to APIKeyView", func(t *testing.T) {
		input := llmcore.LLMProviderAPIKey{
			ID:             1,
			ProviderID:     10,
			APIKey:         "sk-test-1234567890",
			BaseURL:        "https://api.example.com",
			Label:          "Test Key",
			Priority:       50,
			Weight:         80,
			Enabled:        true,
			TotalRequests:  1000,
			FailedRequests: 10,
			RateLimitRPM:   100,
			RateLimitRPD:   1000,
		}

		got := bridge.FromLLMProviderAPIKey(input)
		if got.ID != 1 {
			t.Errorf("ID: expected 1, got %d", got.ID)
		}
		if got.APIKeyMasked != "**************7890" {
			t.Errorf("APIKeyMasked: expected **************7890, got %s", got.APIKeyMasked)
		}
		if got.TotalRequests != 1000 {
			t.Errorf("TotalRequests: expected 1000, got %d", got.TotalRequests)
		}
	})

	t.Run("converts slice of APIKeys", func(t *testing.T) {
		input := []llmcore.LLMProviderAPIKey{
			{ID: 1, ProviderID: 10, APIKey: "sk-test-1"},
			{ID: 2, ProviderID: 10, APIKey: "sk-test-2"},
		}

		got := bridge.FromLLMProviderAPIKeys(input)
		if len(got) != 2 {
			t.Errorf("expected 2 items, got %d", len(got))
		}
	})

	t.Run("converts to APIKeyStatsView", func(t *testing.T) {
		now := time.Now()
		input := []llmcore.LLMProviderAPIKey{
			{
				ID:             1,
				Label:          "Test Key",
				BaseURL:        "https://api.example.com",
				Enabled:        true,
				TotalRequests:  100,
				FailedRequests: 10,
				CurrentRPM:     5,
				CurrentRPD:     50,
				LastUsedAt:     &now,
			},
		}

		got := bridge.FromLLMProviderAPIKeyStats(input)
		if len(got) != 1 {
			t.Fatalf("expected 1 item, got %d", len(got))
		}
		if got[0].SuccessRate != 0.9 {
			t.Errorf("SuccessRate: expected 0.9, got %f", got[0].SuccessRate)
		}
		if !got[0].IsHealthy {
			t.Error("IsHealthy: expected true")
		}
	})
}

func TestLLMTypeBridge_Providers(t *testing.T) {
	bridge := NewLLMTypeBridge(nil)

	t.Run("converts providers", func(t *testing.T) {
		input := []llmcore.LLMProvider{
			{
				ID:          1,
				Code:        "openai",
				Name:        "OpenAI",
				Description: "OpenAI API",
				Status:      llmcore.LLMProviderStatusActive,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		got := bridge.FromLLMProviders(input)
		if len(got) != 1 {
			t.Fatalf("expected 1 item, got %d", len(got))
		}
		if got[0].Code != "openai" {
			t.Errorf("Code: expected openai, got %s", got[0].Code)
		}
		if got[0].Status != "active" {
			t.Errorf("Status: expected active, got %s", got[0].Status)
		}
	})

	t.Run("nil slice returns nil", func(t *testing.T) {
		if got := bridge.FromLLMProviders(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestLLMTypeBridge_ChatRequest(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		bridge := NewLLMTypeBridge(nil)
		if got := bridge.ToLLMChatRequest(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("direct conversion fallback", func(t *testing.T) {
		bridge := NewLLMTypeBridge(nil)
		input := &ChatRequest{
			Model:       "gpt-4",
			Temperature: 0.7,
			MaxTokens:   1000,
			Messages: []Message{
				{Role: "user", Content: "Hello"},
			},
		}

		got := bridge.ToLLMChatRequest(input)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.Model != "gpt-4" {
			t.Errorf("Model: expected gpt-4, got %s", got.Model)
		}
		if got.Temperature != 0.7 {
			t.Errorf("Temperature: expected 0.7, got %f", got.Temperature)
		}
		if len(got.Messages) != 1 {
			t.Errorf("Messages: expected 1, got %d", len(got.Messages))
		}
	})
}

func TestLLMTypeBridge_ChatResponse(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		bridge := NewLLMTypeBridge(nil)
		if got := bridge.FromLLMChatResponse(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("direct conversion fallback", func(t *testing.T) {
		bridge := NewLLMTypeBridge(nil)
		input := &types.ChatResponse{
			ID:       "chat-123",
			Provider: "openai",
			Model:    "gpt-4",
			Choices: []types.ChatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: types.Message{
						Role:    "assistant",
						Content: "Hello!",
					},
				},
			},
			Usage: types.ChatUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		got := bridge.FromLLMChatResponse(input)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.ID != "chat-123" {
			t.Errorf("ID: expected chat-123, got %s", got.ID)
		}
		if got.Provider != "openai" {
			t.Errorf("Provider: expected openai, got %s", got.Provider)
		}
		if len(got.Choices) != 1 {
			t.Errorf("Choices: expected 1, got %d", len(got.Choices))
		}
		if got.Choices[0].Message.Content != "Hello!" {
			t.Errorf("Content: expected Hello!, got %s", got.Choices[0].Message.Content)
		}
	})
}

func TestNormalizeWebSearchDomains(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{"nil input", nil, nil},
		{"empty input", []string{}, nil},
		{"single domain", []string{"example.com"}, []string{"example.com"}},
		{"multiple domains", []string{"a.com", "b.com"}, []string{"a.com", "b.com"}},
		{"deduplication", []string{"a.com", "a.com", "b.com"}, []string{"a.com", "b.com"}},
		{"trim whitespace", []string{"  a.com  ", "b.com"}, []string{"a.com", "b.com"}},
		{"skip empty", []string{"", "a.com", "  "}, []string{"a.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWebSearchDomains(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("expected %v, got %v", tt.expected, got)
					return
				}
			}
		})
	}
}
