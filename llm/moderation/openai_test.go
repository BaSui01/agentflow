package moderation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultOpenAIConfig(t *testing.T) {
	cfg := DefaultOpenAIConfig()
	assert.Equal(t, "https://api.openai.com/v1", cfg.BaseURL)
	assert.Equal(t, "omni-moderation-latest", cfg.Model)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestNewOpenAIProvider(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{APIKey: "test-key"})
	require.NotNil(t, p)
	assert.Equal(t, "https://api.openai.com/v1", p.cfg.BaseURL)
	assert.Equal(t, "omni-moderation-latest", p.cfg.Model)
}

func TestNewOpenAIProvider_CustomConfig(t *testing.T) {
	cfg := OpenAIConfig{
		APIKey:  "key",
		BaseURL: "https://custom.api.com",
		Model:   "custom-model",
		Timeout: 10 * time.Second,
	}
	p := NewOpenAIProvider(cfg)
	assert.Equal(t, "https://custom.api.com", p.cfg.BaseURL)
	assert.Equal(t, "custom-model", p.cfg.Model)
}

func TestOpenAIProvider_Name(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	assert.Equal(t, "openai-moderation", p.Name())
}

func TestOpenAIProvider_Moderate_TextOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/moderations", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req openAIModerationRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "omni-moderation-latest", req.Model)

		resp := openAIModerationResponse{
			ID:    "modr-123",
			Model: "omni-moderation-latest",
			Results: []struct {
				Flagged        bool               `json:"flagged"`
				Categories     map[string]bool    `json:"categories"`
				CategoryScores map[string]float64 `json:"category_scores"`
			}{
				{
					Flagged:        true,
					Categories:     map[string]bool{"hate": true, "violence": false},
					CategoryScores: map[string]float64{"hate": 0.95, "violence": 0.01},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Moderate(context.Background(), &ModerationRequest{
		Input: []string{"some hateful text"},
	})
	require.NoError(t, err)
	assert.Equal(t, "openai-moderation", result.Provider)
	assert.Equal(t, "omni-moderation-latest", result.Model)
	require.Len(t, result.Results, 1)
	assert.True(t, result.Results[0].Flagged)
	assert.True(t, result.Results[0].Categories.Hate)
	assert.False(t, result.Results[0].Categories.Violence)
	assert.InDelta(t, 0.95, result.Results[0].Scores.Hate, 0.001)
}

func TestOpenAIProvider_Moderate_WithImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIModerationRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Input should be multimodal (array of items)
		items, ok := req.Input.([]any)
		require.True(t, ok)
		assert.GreaterOrEqual(t, len(items), 2) // at least 1 text + 1 image

		resp := openAIModerationResponse{
			ID:    "modr-456",
			Model: "omni-moderation-latest",
			Results: []struct {
				Flagged        bool               `json:"flagged"`
				Categories     map[string]bool    `json:"categories"`
				CategoryScores map[string]float64 `json:"category_scores"`
			}{
				{Flagged: false, Categories: map[string]bool{}, CategoryScores: map[string]float64{}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider(OpenAIConfig{APIKey: "test-key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Moderate(context.Background(), &ModerationRequest{
		Input:  []string{"check this"},
		Images: []string{"base64imagedata"},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.False(t, result.Results[0].Flagged)
}

func TestOpenAIProvider_Moderate_CustomModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIModerationRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "custom-model", req.Model)

		resp := openAIModerationResponse{Model: "custom-model"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Moderate(context.Background(), &ModerationRequest{
		Input: []string{"text"},
		Model: "custom-model",
	})
	require.NoError(t, err)
	assert.Equal(t, "custom-model", result.Model)
}

func TestOpenAIProvider_Moderate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Moderate(context.Background(), &ModerationRequest{Input: []string{"text"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=429")
}

func TestOpenAIProvider_Moderate_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	p := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Moderate(ctx, &ModerationRequest{Input: []string{"text"}})
	assert.Error(t, err)
}

func TestMapCategories(t *testing.T) {
	cats := map[string]bool{
		"hate":             true,
		"hate/threatening": true,
		"harassment":       false,
		"self-harm":        true,
		"self-harm/intent": false,
		"sexual":           false,
		"sexual/minors":    false,
		"violence":         true,
		"violence/graphic": false,
		"illicit":          true,
		"illicit/violent":  false,
	}

	result := mapCategories(cats)
	assert.True(t, result.Hate)
	assert.True(t, result.HateThreatening)
	assert.False(t, result.Harassment)
	assert.True(t, result.SelfHarm)
	assert.False(t, result.SelfHarmIntent)
	assert.False(t, result.Sexual)
	assert.False(t, result.SexualMinors)
	assert.True(t, result.Violence)
	assert.False(t, result.ViolenceGraphic)
	assert.True(t, result.Illicit)
	assert.False(t, result.IllicitViolent)
}

func TestMapScores(t *testing.T) {
	scores := map[string]float64{
		"hate":             0.95,
		"hate/threatening": 0.1,
		"harassment":       0.2,
		"self-harm":        0.3,
		"self-harm/intent": 0.4,
		"sexual":           0.5,
		"sexual/minors":    0.6,
		"violence":         0.7,
		"violence/graphic": 0.8,
		"illicit":          0.85,
		"illicit/violent":  0.9,
	}

	result := mapScores(scores)
	assert.InDelta(t, 0.95, result.Hate, 0.001)
	assert.InDelta(t, 0.1, result.HateThreatening, 0.001)
	assert.InDelta(t, 0.2, result.Harassment, 0.001)
	assert.InDelta(t, 0.3, result.SelfHarm, 0.001)
	assert.InDelta(t, 0.4, result.SelfHarmIntent, 0.001)
	assert.InDelta(t, 0.5, result.Sexual, 0.001)
	assert.InDelta(t, 0.6, result.SexualMinors, 0.001)
	assert.InDelta(t, 0.7, result.Violence, 0.001)
	assert.InDelta(t, 0.8, result.ViolenceGraphic, 0.001)
	assert.InDelta(t, 0.85, result.Illicit, 0.001)
	assert.InDelta(t, 0.9, result.IllicitViolent, 0.001)
}

func TestOpenAIProvider_ImplementsModerationProvider(t *testing.T) {
	var _ ModerationProvider = (*OpenAIProvider)(nil)
}
