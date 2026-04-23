package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- resolveAPIKey ---

func TestGeminiProvider_ResolveAPIKey_CredentialOverride(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "default-key"},
	}, zap.NewNop())

	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "override-key"})
	key := p.resolveAPIKey(ctx)
	assert.Equal(t, "override-key", key)
}

func TestGeminiProvider_ResolveAPIKey_MultiKey(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey: "fallback",
			APIKeys: []providers.APIKeyEntry{
				{Key: "key-a"},
				{Key: "key-b"},
			},
		},
	}, zap.NewNop())

	key1 := p.resolveAPIKey(context.Background())
	key2 := p.resolveAPIKey(context.Background())
	// Should round-robin between key-a and key-b
	assert.Contains(t, []string{"key-a", "key-b"}, key1)
	assert.Contains(t, []string{"key-a", "key-b"}, key2)
}

// --- convertToGeminiContents ---

func TestConvertToGeminiContents_ToolCalls(t *testing.T) {
	msgs := []types.Message{
		{Role: llm.RoleUser, Content: "What's the weather?"},
		{Role: llm.RoleAssistant, Content: "Let me check.", ToolCalls: []types.ToolCall{
			{ID: "tc1", Name: "get_weather", Arguments: json.RawMessage(`{"city":"NYC"}`)},
		}},
		{Role: llm.RoleTool, ToolCallID: "tc1", Name: "get_weather", Content: `{"temp":72}`},
	}

	sys, contents := convertToGeminiContents(msgs)
	assert.Nil(t, sys)
	require.Len(t, contents, 3)

	// Assistant message should have function call part
	assert.Equal(t, "model", contents[1].Role)
	hasFunctionCall := false
	for _, p := range contents[1].Parts {
		if p.FunctionCall != nil {
			hasFunctionCall = true
			assert.Equal(t, "get_weather", p.FunctionCall.Name)
		}
	}
	assert.True(t, hasFunctionCall)

	// Tool message should have function response part
	hasFunctionResponse := false
	for _, p := range contents[2].Parts {
		if p.FunctionResponse != nil {
			hasFunctionResponse = true
			assert.Equal(t, "get_weather", p.FunctionResponse.Name)
		}
	}
	assert.True(t, hasFunctionResponse)
}

func TestConvertToGeminiContents_ToolResponseNonJSON(t *testing.T) {
	msgs := []types.Message{
		{Role: llm.RoleTool, ToolCallID: "tc1", Name: "search", Content: "plain text result"},
	}

	_, contents := convertToGeminiContents(msgs)
	require.Len(t, contents, 1)
	hasFunctionResponse := false
	for _, p := range contents[0].Parts {
		if p.FunctionResponse != nil {
			hasFunctionResponse = true
			assert.Equal(t, "plain text result", p.FunctionResponse.Response["result"])
		}
	}
	assert.True(t, hasFunctionResponse)
}

// --- convertToGeminiTools ---

func TestConvertToGeminiTools_WithValidTools(t *testing.T) {
	tools := []types.ToolSchema{
		{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
	}

	result := convertToGeminiTools(tools, nil)
	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	assert.Equal(t, "get_weather", result[0].FunctionDeclarations[0].Name)
}

func TestConvertToGeminiTools_Empty(t *testing.T) {
	result := convertToGeminiTools(nil, nil)
	assert.Nil(t, result)
}

func TestConvertToGeminiTools_InvalidJSON(t *testing.T) {
	tools := []types.ToolSchema{
		{Name: "bad", Parameters: json.RawMessage(`invalid`)},
	}
	result := convertToGeminiTools(tools, nil)
	assert.Nil(t, result) // all declarations fail to parse -> nil
}

// --- normalizeFinishReason ---

func TestNormalizeFinishReason(t *testing.T) {
	tests := []struct{ in, out string }{
		{"STOP", "stop"},
		{"MAX_TOKENS", "length"},
		{"SAFETY", "content_filter"},
		{"RECITATION", "content_filter"},
		{"BLOCKLIST", "content_filter"},
		{"LANGUAGE", "content_filter"},
		{"", ""},
		{"OTHER", "other"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.out, normalizeFinishReason(tt.in), "input: %s", tt.in)
	}
}

// --- convertToolChoice ---

func TestConvertToolChoice(t *testing.T) {
	assert.Nil(t, convertToolChoice(nil, nil))

	tc := convertToolChoice(&types.ToolChoice{Mode: types.ToolChoiceModeAuto}, nil)
	require.NotNil(t, tc)
	assert.Equal(t, "AUTO", tc.FunctionCallingConfig.Mode)

	tc = convertToolChoice(&types.ToolChoice{Mode: types.ToolChoiceModeRequired}, nil)
	require.NotNil(t, tc)
	assert.Equal(t, "ANY", tc.FunctionCallingConfig.Mode)

	tc = convertToolChoice(&types.ToolChoice{Mode: types.ToolChoiceModeNone}, nil)
	require.NotNil(t, tc)
	assert.Equal(t, "NONE", tc.FunctionCallingConfig.Mode)

	tc = convertToolChoice(&types.ToolChoice{
		Mode:     types.ToolChoiceModeSpecific,
		ToolName: "get_weather",
	}, nil)
	require.NotNil(t, tc)
	assert.Equal(t, "ANY", tc.FunctionCallingConfig.Mode)
	assert.Equal(t, []string{"get_weather"}, tc.FunctionCallingConfig.AllowedFunctionNames)

	includeServerSide := true
	tc = convertToolChoice(&types.ToolChoice{
		Mode:                             types.ToolChoiceModeAllowed,
		AllowedTools:                     []string{"weather_lookup"},
		IncludeServerSideToolInvocations: &includeServerSide,
	}, nil)
	require.NotNil(t, tc)
	require.NotNil(t, tc.FunctionCallingConfig)
	assert.Equal(t, "ANY", tc.FunctionCallingConfig.Mode)
	assert.Equal(t, []string{"weather_lookup"}, tc.FunctionCallingConfig.AllowedFunctionNames)
	require.NotNil(t, tc.IncludeServerSideToolInvocations)
	assert.True(t, *tc.IncludeServerSideToolInvocations)

	assert.Nil(t, convertToolChoice(&types.ToolChoice{}, nil))
}

// --- checkPromptFeedback ---

func TestCheckPromptFeedback_NoBlock(t *testing.T) {
	resp := geminiResponse{Candidates: []geminiCandidate{{Content: geminiContent{}}}}
	assert.NoError(t, checkPromptFeedback(resp, "gemini"))
}

func TestCheckPromptFeedback_Blocked(t *testing.T) {
	resp := geminiResponse{
		PromptFeedback: &geminiPromptFeedback{BlockReason: "SAFETY"},
	}
	err := checkPromptFeedback(resp, "gemini")
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrContentFiltered, llmErr.Code)
	assert.Contains(t, llmErr.Message, "SAFETY")
}

// --- SupportsStructuredOutput ---

func TestGeminiProvider_SupportsStructuredOutput(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{}, zap.NewNop())
	assert.True(t, p.SupportsStructuredOutput())
}

// --- buildGenerationConfig ---

func TestBuildGenerationConfig_Empty(t *testing.T) {
	req := &llm.ChatRequest{}
	assert.Nil(t, buildGenerationConfig(req))
}

func TestBuildGenerationConfig_WithThinking(t *testing.T) {
	req := &llm.ChatRequest{ReasoningMode: "high"}
	cfg := buildGenerationConfig(req)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.ThinkingConfig)
	assert.Equal(t, "high", cfg.ThinkingConfig.ThinkingLevel)
	assert.True(t, cfg.ThinkingConfig.IncludeThoughts)
}

func TestBuildGenAIGenerationConfig_Gemini25UsesThinkingBudget(t *testing.T) {
	req := &llm.ChatRequest{Model: "gemini-2.5-flash", ReasoningMode: "high"}
	cfg := buildGenAIGenerationConfig(req, nil)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.ThinkingConfig)
	require.NotNil(t, cfg.ThinkingConfig.ThinkingBudget)
	assert.Equal(t, int32(-1), *cfg.ThinkingConfig.ThinkingBudget)
	assert.True(t, cfg.ThinkingConfig.IncludeThoughts)
	assert.Equal(t, "", string(cfg.ThinkingConfig.ThinkingLevel))
}

func TestBuildGenAIGenerationConfig_Gemini25FlashDisableThinking(t *testing.T) {
	req := &llm.ChatRequest{Model: "gemini-2.5-flash", ReasoningMode: "disabled"}
	cfg := buildGenAIGenerationConfig(req, nil)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.ThinkingConfig)
	require.NotNil(t, cfg.ThinkingConfig.ThinkingBudget)
	assert.Equal(t, int32(0), *cfg.ThinkingConfig.ThinkingBudget)
	assert.False(t, cfg.ThinkingConfig.IncludeThoughts)
}

func TestBuildGenAIGenerationConfig_Gemini3UsesThinkingLevel(t *testing.T) {
	req := &llm.ChatRequest{Model: "gemini-3-pro-preview", ReasoningMode: "high"}
	cfg := buildGenAIGenerationConfig(req, nil)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.ThinkingConfig)
	assert.Equal(t, "HIGH", string(cfg.ThinkingConfig.ThinkingLevel))
	assert.Nil(t, cfg.ThinkingConfig.ThinkingBudget)
	assert.True(t, cfg.ThinkingConfig.IncludeThoughts)
}

func TestBuildGenerationConfig_WithResponseFormat(t *testing.T) {
	req := &llm.ChatRequest{
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatJSONSchema,
			JSONSchema: &llm.JSONSchemaParam{
				Name:   "test",
				Schema: map[string]any{"type": "object"},
			},
		},
	}
	cfg := buildGenerationConfig(req)
	require.NotNil(t, cfg)
	assert.Equal(t, "application/json", cfg.ResponseMimeType)
	assert.NotNil(t, cfg.ResponseSchema)
}

func TestGeminiProvider_Completion_PreservesCachedContentWithToolConfig(t *testing.T) {
	var reqBody geminiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}},
				FinishReason: "STOP",
			}},
		}))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "system"},
			{Role: llm.RoleUser, Content: "hi"},
		},
		Tools: []types.ToolSchema{{
			Name:        "weather_lookup",
			Description: "Lookup weather",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
		ToolChoice:    &types.ToolChoice{Mode: types.ToolChoiceModeRequired},
		CachedContent: "cachedContents/abc",
	})
	require.NoError(t, err)
	assert.Equal(t, "cachedContents/abc", reqBody.CachedContent)
	require.NotNil(t, reqBody.SystemInstruction)
	require.NotNil(t, reqBody.ToolConfig)
	require.NotEmpty(t, reqBody.Tools)
}

// --- Completion with thinking ---

func TestGeminiProvider_Completion_WithThinking(t *testing.T) {
	thoughtFlag := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role: "model",
					Parts: []geminiPart{
						{Text: "Let me think...", Thought: &thoughtFlag, ThoughtSignature: "c2lnX3N5bmM="},
						{Text: "The answer is 42."},
					},
				},
				FinishReason: "STOP",
			}},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
				TotalTokenCount:      18,
				ThoughtsTokenCount:   5,
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:      []types.Message{{Role: llm.RoleUser, Content: "What is 6*7?"}},
		ReasoningMode: "high",
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "The answer is 42.", resp.Choices[0].Message.Content)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Let me think...", *resp.Choices[0].Message.ReasoningContent)
	require.Len(t, resp.Choices[0].Message.ReasoningSummaries, 1)
	assert.Equal(t, "thought_summary", resp.Choices[0].Message.ReasoningSummaries[0].Kind)
	require.Len(t, resp.Choices[0].Message.OpaqueReasoning, 1)
	assert.Equal(t, "c2lnX3N5bmM=", resp.Choices[0].Message.OpaqueReasoning[0].State)
	require.NotNil(t, resp.Usage.CompletionTokensDetails)
	assert.Equal(t, 5, resp.Usage.CompletionTokensDetails.ReasoningTokens)
}

// --- Completion with promptFeedback block ---

func TestGeminiProvider_Completion_PromptBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			PromptFeedback: &geminiPromptFeedback{BlockReason: "SAFETY"},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "bad content"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrContentFiltered, llmErr.Code)
}

// --- Stream with SSE and thinking ---

func TestGeminiProvider_Stream_WithThinking(t *testing.T) {
	thoughtFlag := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role: "model",
					Parts: []geminiPart{
						{Text: "thinking...", Thought: &thoughtFlag, ThoughtSignature: "c2lnX3N0cmVhbQ=="},
						{Text: "result"},
					},
				},
				FinishReason: "STOP",
			}},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		require.Nil(t, c.Err)
		chunks = append(chunks, c)
	}
	require.NotEmpty(t, chunks)
	assert.Equal(t, "result", chunks[0].Delta.Content)
	require.NotNil(t, chunks[0].Delta.ReasoningContent)
	assert.Equal(t, "thinking...", *chunks[0].Delta.ReasoningContent)
	require.Len(t, chunks[0].Delta.ReasoningSummaries, 1)
	assert.Equal(t, "thinking...", chunks[0].Delta.ReasoningSummaries[0].Text)
	require.Len(t, chunks[0].Delta.OpaqueReasoning, 1)
	assert.Equal(t, "c2lnX3N0cmVhbQ==", chunks[0].Delta.OpaqueReasoning[0].State)
	assert.Equal(t, "stop", chunks[0].FinishReason)
}

func TestConvertToGeminiContents_PreservesThoughtSignature(t *testing.T) {
	reasoning := "thinking..."
	system, contents := convertToGeminiContents([]types.Message{
		{
			Role:             llm.RoleAssistant,
			Content:          "result",
			ReasoningContent: &reasoning,
			OpaqueReasoning: []types.OpaqueReasoning{
				{Provider: "gemini", Kind: "thought_signature", State: "sig_req", PartIndex: 0},
			},
		},
	})

	assert.Nil(t, system)
	require.Len(t, contents, 1)
	require.GreaterOrEqual(t, len(contents[0].Parts), 2)
	require.NotNil(t, contents[0].Parts[0].Thought)
	assert.True(t, *contents[0].Parts[0].Thought)
	assert.Equal(t, "sig_req", contents[0].Parts[0].ThoughtSignature)
	assert.Equal(t, "result", contents[0].Parts[1].Text)
}

// --- convertSafetySettings ---

func TestConvertSafetySettings(t *testing.T) {
	assert.Nil(t, convertSafetySettings(nil))

	settings := []providers.GeminiSafetySetting{
		{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
	}
	result := convertSafetySettings(settings)
	require.Len(t, result, 1)
	assert.Equal(t, "HARM_CATEGORY_HARASSMENT", result[0].Category)
	assert.Equal(t, "BLOCK_NONE", result[0].Threshold)
}

// --- Stream ---

func TestGeminiProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		w.Header().Set("Content-Type", "text/event-stream")
		// Return SSE format
		chunk := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Hello"}},
				}},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	for range ch {
	}
	// At least we got a channel and it closed without panic
	assert.NotNil(t, ch)
}

func TestGeminiProvider_Stream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	var gotErr *types.Error
	for chunk := range ch {
		gotErr = chunk.Err
	}
	require.NotNil(t, gotErr)
}

func TestGeminiProvider_Stream_PromptBlocked(t *testing.T) {
	// 验证流式模式下 promptFeedback 安全阻断被正确传递（而非静默丢失）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Gemini 安全阻断: candidates 为空，只有 promptFeedback
		resp := geminiResponse{
			PromptFeedback: &geminiPromptFeedback{
				BlockReason:  "SAFETY",
				BlockMessage: "content blocked due to safety",
			},
		}
		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "something unsafe"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	// 必须收到一个包含错误的 chunk
	require.Len(t, chunks, 1, "should receive exactly one error chunk")
	require.NotNil(t, chunks[0].Err, "chunk must contain an error")
	assert.Contains(t, chunks[0].Err.Message, "SAFETY")
	assert.Contains(t, chunks[0].Err.Message, "safety filter")
	assert.Equal(t, llm.ErrContentFiltered, chunks[0].Err.Code)
}

func TestGeminiProvider_Stream_PromptBlocked_NoBlockMessage(t *testing.T) {
	// 验证只有 blockReason 没有 blockMessage 时也能正确处理
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		resp := geminiResponse{
			PromptFeedback: &geminiPromptFeedback{
				BlockReason: "OTHER",
			},
		}
		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "blocked content"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Err)
	assert.Contains(t, chunks[0].Err.Message, "OTHER")
}

// =============================================================================
// Google Search Grounding Tests
// =============================================================================

func TestConvertToGeminiTools_WithGoogleSearch(t *testing.T) {
	// 1. wsOpts 触发注入
	tools := []types.ToolSchema{
		{Name: "calc", Description: "Calc", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	wsOpts := &llm.WebSearchOptions{SearchContextSize: "medium"}

	result := convertToGeminiTools(tools, wsOpts)
	require.Len(t, result, 2) // FunctionDeclarations + GoogleSearch

	// 第一个条目：普通函数工具
	assert.NotNil(t, result[0].FunctionDeclarations)
	assert.Nil(t, result[0].GoogleSearch)

	// 第二个条目：google_search（独立条目）
	assert.Nil(t, result[1].FunctionDeclarations)
	assert.NotNil(t, result[1].GoogleSearch)
}

func TestConvertToGeminiTools_WithGoogleSearch_FromToolName(t *testing.T) {
	// 工具列表中有 web_search 占位工具时，替换为 google_search
	tools := []types.ToolSchema{
		{Name: "web_search", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "calc", Description: "Calc", Parameters: json.RawMessage(`{"type":"object"}`)},
	}

	result := convertToGeminiTools(tools, nil)
	require.Len(t, result, 2)
	// 验证 google_search 存在
	hasGoogleSearch := false
	for _, t := range result {
		if t.GoogleSearch != nil {
			hasGoogleSearch = true
		}
	}
	assert.True(t, hasGoogleSearch)
}

func TestConvertToGeminiTools_OnlyGoogleSearch(t *testing.T) {
	// 只有 google_search，没有函数工具
	result := convertToGeminiTools(nil, &llm.WebSearchOptions{})
	require.Len(t, result, 1)
	assert.NotNil(t, result[0].GoogleSearch)
	assert.Nil(t, result[0].FunctionDeclarations)
}

func TestExtractGroundingAnnotations_WithSupports(t *testing.T) {
	gm := &geminiGroundingMetadata{
		GroundingChunks: []geminiGroundingChunk{
			{Web: &geminiGroundingChunkWeb{URI: "https://example.com/a", Title: "Article A"}},
			{Web: &geminiGroundingChunkWeb{URI: "https://example.com/b", Title: "Article B"}},
		},
		GroundingSupports: []geminiGroundingSupport{
			{
				Segment:               &geminiGroundingSegment{StartIndex: 0, EndIndex: 50, Text: "Some text"},
				GroundingChunkIndices: []int{0},
			},
			{
				Segment:               &geminiGroundingSegment{StartIndex: 51, EndIndex: 100},
				GroundingChunkIndices: []int{1},
			},
		},
	}

	annotations := extractGroundingAnnotations(gm)
	require.Len(t, annotations, 2)

	assert.Equal(t, "url_citation", annotations[0].Type)
	assert.Equal(t, "https://example.com/a", annotations[0].URL)
	assert.Equal(t, "Article A", annotations[0].Title)
	assert.Equal(t, 0, annotations[0].StartIndex)
	assert.Equal(t, 50, annotations[0].EndIndex)

	assert.Equal(t, "url_citation", annotations[1].Type)
	assert.Equal(t, "https://example.com/b", annotations[1].URL)
	assert.Equal(t, 51, annotations[1].StartIndex)
	assert.Equal(t, 100, annotations[1].EndIndex)
}

func TestExtractGroundingAnnotations_WithoutSupports(t *testing.T) {
	gm := &geminiGroundingMetadata{
		GroundingChunks: []geminiGroundingChunk{
			{Web: &geminiGroundingChunkWeb{URI: "https://example.com/c", Title: "Article C"}},
		},
	}

	annotations := extractGroundingAnnotations(gm)
	require.Len(t, annotations, 1)
	assert.Equal(t, "url_citation", annotations[0].Type)
	assert.Equal(t, "https://example.com/c", annotations[0].URL)
	assert.Equal(t, 0, annotations[0].StartIndex) // 无位置信息
}

func TestExtractGroundingAnnotations_Nil(t *testing.T) {
	assert.Nil(t, extractGroundingAnnotations(nil))
	assert.Nil(t, extractGroundingAnnotations(&geminiGroundingMetadata{}))
}

func TestGeminiProvider_Completion_WithGrounding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求中包含 google_search 工具
		var reqBody map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		hasGoogleSearch := false
		if tools, ok := reqBody["tools"].([]any); ok {
			for _, toolAny := range tools {
				tool, ok := toolAny.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := tool["googleSearch"]; ok {
					hasGoogleSearch = true
					break
				}
				if _, ok := tool["google_search"]; ok {
					hasGoogleSearch = true
					break
				}
			}
		}
		if !hasGoogleSearch {
			if tools, ok := reqBody["tools"].([]any); ok {
				for _, toolAny := range tools {
					tool, ok := toolAny.(map[string]any)
					if !ok {
						continue
					}
					if _, ok := tool["googleSearchRetrieval"]; ok {
						hasGoogleSearch = true
						break
					}
				}
			}
		}
		if hasGoogleSearch {
			hasGoogleSearch = true
		}
		assert.True(t, hasGoogleSearch, "request should contain google_search tool")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "The answer based on search."}},
				},
				FinishReason: "STOP",
				GroundingMetadata: &geminiGroundingMetadata{
					WebSearchQueries: []string{"test query"},
					GroundingChunks: []geminiGroundingChunk{
						{Web: &geminiGroundingChunkWeb{URI: "https://example.com", Title: "Example"}},
					},
					GroundingSupports: []geminiGroundingSupport{
						{
							Segment:               &geminiGroundingSegment{StartIndex: 0, EndIndex: 27},
							GroundingChunkIndices: []int{0},
						},
					},
				},
			}},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
				TotalTokenCount:      18,
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:         []types.Message{{Role: llm.RoleUser, Content: "Search for something"}},
		WebSearchOptions: &llm.WebSearchOptions{},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "The answer based on search.", resp.Choices[0].Message.Content)

	// 验证 grounding annotations
	require.Len(t, resp.Choices[0].Message.Annotations, 1)
	ann := resp.Choices[0].Message.Annotations[0]
	assert.Equal(t, "url_citation", ann.Type)
	assert.Equal(t, "https://example.com", ann.URL)
	assert.Equal(t, "Example", ann.Title)
	assert.Equal(t, 0, ann.StartIndex)
	assert.Equal(t, 27, ann.EndIndex)
}

func TestGeminiProvider_Stream_WithGrounding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Grounded result."}},
				},
				FinishReason: "STOP",
				GroundingMetadata: &geminiGroundingMetadata{
					GroundingChunks: []geminiGroundingChunk{
						{Web: &geminiGroundingChunkWeb{URI: "https://news.example.com", Title: "News"}},
					},
				},
			}},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages:         []types.Message{{Role: llm.RoleUser, Content: "News?"}},
		WebSearchOptions: &llm.WebSearchOptions{},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		require.Nil(t, c.Err)
		chunks = append(chunks, c)
	}
	require.NotEmpty(t, chunks)

	// 验证流式 grounding annotations
	assert.Equal(t, "Grounded result.", chunks[0].Delta.Content)
	require.Len(t, chunks[0].Delta.Annotations, 1)
	assert.Equal(t, "url_citation", chunks[0].Delta.Annotations[0].Type)
	assert.Equal(t, "https://news.example.com", chunks[0].Delta.Annotations[0].URL)
	assert.Equal(t, "News", chunks[0].Delta.Annotations[0].Title)
}
