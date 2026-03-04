package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	claudeprov "github.com/BaSui01/agentflow/llm/providers/anthropic"
	geminiprov "github.com/BaSui01/agentflow/llm/providers/gemini"
	openaiprov "github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func runOpenAIResponsesWebSearchRegression(ctx context.Context, logger *zap.Logger) error {
	logger.Info("test H: openai responses web_search regression start")

	var completionBody map[string]any
	var streamBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"bad body"}}`))
			return
		}

		if isStream, _ := body["stream"].(bool); isStream {
			streamBody = body
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("event: response.created\n"))
			_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_stream","model":"gpt-5.2-codex"}}` + "\n\n"))
			_, _ = w.Write([]byte("event: response.output_text.delta\n"))
			_, _ = w.Write([]byte(`data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}

		completionBody = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_complete",
			"model":  "gpt-5.2-codex",
			"status": "completed",
			"output": []map[string]any{
				{
					"type": "message",
					"role": "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": "ok"},
					},
				},
			},
		})
	}))
	defer server.Close()

	p := openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Timeout: 15 * time.Second,
		},
		UseResponsesAPI: true,
	}, logger)

	_, err := p.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-5.2-codex",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "check web search mapping"},
		},
		ToolChoice: "auto",
		Tools: []types.ToolSchema{
			{
				Name:        "web_search",
				Description: "native web search",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
		WebSearchOptions: &llm.WebSearchOptions{
			SearchContextSize: "high",
			AllowedDomains:    []string{"openai.com", "example.com"},
			UserLocation: &llm.WebSearchLocation{
				Country: "US",
				City:    "San Francisco",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("responses completion failed: %w", err)
	}

	ch, err := p.Stream(ctx, &llm.ChatRequest{
		Model: "gpt-5.2-codex",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "stream check web search mapping"},
		},
		ToolChoice: "auto",
		Tools: []types.ToolSchema{
			{
				Name:       "web_search_preview",
				Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
		WebSearchOptions: &llm.WebSearchOptions{
			SearchContextSize: "low",
			AllowedDomains:    []string{"news.example.com"},
			UserLocation: &llm.WebSearchLocation{
				Country: "CN",
				City:    "Shanghai",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("responses stream failed: %w", err)
	}
	for range ch {
	}

	completionTool, err := extractResponsesWebSearchTool(completionBody)
	if err != nil {
		return fmt.Errorf("completion web_search tool mapping check failed: %w", err)
	}
	if got := asString(completionTool["search_context_size"]); got != "high" {
		return fmt.Errorf("completion search_context_size mismatch: got=%q want=high", got)
	}
	completionLoc, ok := completionTool["user_location"].(map[string]any)
	if !ok {
		return fmt.Errorf("completion user_location missing or invalid")
	}
	if got := asString(completionLoc["type"]); got != "approximate" {
		return fmt.Errorf("completion user_location.type mismatch: got=%q want=approximate", got)
	}
	if got := asString(completionLoc["country"]); got != "US" {
		return fmt.Errorf("completion user_location.country mismatch: got=%q want=US", got)
	}
	completionFilters, ok := completionTool["filters"].(map[string]any)
	if !ok {
		return fmt.Errorf("completion filters missing or invalid")
	}
	if got := asStringSlice(completionFilters["allowed_domains"]); !equalStringSlices(got, []string{"openai.com", "example.com"}) {
		return fmt.Errorf("completion allowed_domains mismatch: got=%v want=%v", got, []string{"openai.com", "example.com"})
	}

	streamTool, err := extractResponsesWebSearchTool(streamBody)
	if err != nil {
		return fmt.Errorf("stream web_search tool mapping check failed: %w", err)
	}
	if got := asString(streamTool["search_context_size"]); got != "low" {
		return fmt.Errorf("stream search_context_size mismatch: got=%q want=low", got)
	}
	streamLoc, ok := streamTool["user_location"].(map[string]any)
	if !ok {
		return fmt.Errorf("stream user_location missing or invalid")
	}
	if got := asString(streamLoc["type"]); got != "approximate" {
		return fmt.Errorf("stream user_location.type mismatch: got=%q want=approximate", got)
	}
	if got := asString(streamLoc["country"]); got != "CN" {
		return fmt.Errorf("stream user_location.country mismatch: got=%q want=CN", got)
	}
	streamFilters, ok := streamTool["filters"].(map[string]any)
	if !ok {
		return fmt.Errorf("stream filters missing or invalid")
	}
	if got := asStringSlice(streamFilters["allowed_domains"]); !equalStringSlices(got, []string{"news.example.com"}) {
		return fmt.Errorf("stream allowed_domains mismatch: got=%v want=%v", got, []string{"news.example.com"})
	}

	logger.Info("test H: openai responses web_search regression done",
		zap.String("completion_tool_type", asString(completionTool["type"])),
		zap.String("completion_search_context_size", asString(completionTool["search_context_size"])),
		zap.Strings("completion_allowed_domains", asStringSlice(completionFilters["allowed_domains"])),
		zap.String("stream_tool_type", asString(streamTool["type"])),
		zap.String("stream_search_context_size", asString(streamTool["search_context_size"])),
		zap.Strings("stream_allowed_domains", asStringSlice(streamFilters["allowed_domains"])),
	)
	return nil
}

func runOpenAIEndpointModeRoutingRegression(ctx context.Context, logger *zap.Logger) error {
	logger.Info("test L: openai endpoint_mode routing regression start")

	calledPaths := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPaths = append(calledPaths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "resp_endpoint_mode",
				"model":  "gpt-5.2-codex",
				"status": "completed",
				"output": []map[string]any{
					{
						"type": "message",
						"role": "assistant",
						"content": []map[string]any{
							{"type": "output_text", "text": "ok"},
						},
					},
				},
			})
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "chatcmpl_endpoint_mode",
				"model": "gpt-5.2-codex",
				"choices": []map[string]any{
					{
						"index":         0,
						"finish_reason": "stop",
						"message": map[string]any{
							"role":    "assistant",
							"content": "ok",
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unexpected endpoint"}}`))
		}
	}))
	defer server.Close()

	p := openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Timeout: 15 * time.Second,
		},
		UseResponsesAPI: false,
	}, logger)

	baseReq := &llm.ChatRequest{
		Model: "gpt-5.2-codex",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "route by endpoint mode"},
		},
	}

	reqResponses := *baseReq
	reqResponses.Metadata = map[string]string{"endpoint_mode": "responses"}
	if _, err := p.Completion(ctx, &reqResponses); err != nil {
		return fmt.Errorf("responses endpoint_mode request failed: %w", err)
	}

	reqChat := *baseReq
	reqChat.Metadata = map[string]string{"endpoint_mode": "chat_completions"}
	if _, err := p.Completion(ctx, &reqChat); err != nil {
		return fmt.Errorf("chat_completions endpoint_mode request failed: %w", err)
	}

	reqAuto := *baseReq
	reqAuto.Metadata = map[string]string{"endpoint_mode": "auto"}
	if _, err := p.Completion(ctx, &reqAuto); err != nil {
		return fmt.Errorf("auto endpoint_mode request failed: %w", err)
	}

	expected := []string{"/v1/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(calledPaths) != len(expected) {
		return fmt.Errorf("unexpected endpoint call count: got=%d want=%d", len(calledPaths), len(expected))
	}
	for i := range expected {
		if calledPaths[i] != expected[i] {
			return fmt.Errorf("endpoint call[%d] mismatch: got=%s want=%s", i, calledPaths[i], expected[i])
		}
	}

	logger.Info("test L: openai endpoint_mode routing regression done",
		zap.Strings("called_paths", calledPaths),
	)
	return nil
}

func runGeminiModelAwareRegression(ctx context.Context, logger *zap.Logger) error {
	logger.Info("test I: gemini model-aware adapter regression start")

	var imagenBody map[string]any
	var geminiImageBody map[string]any
	var veoBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1beta/models/imagen-4.0-generate-001:predict"):
			if err := json.NewDecoder(r.Body).Decode(&imagenBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"predictions": []map[string]any{
					{"bytesBase64Encoded": "ZmFrZS1pbWFnZQ=="},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/v1beta/models/gemini-3-pro-image-preview:generateContent"):
			if err := json.NewDecoder(r.Body).Decode(&geminiImageBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{
									"inlineData": map[string]any{
										"mimeType": "image/png",
										"data":     "ZmFrZS1nZW1pbmktbmF0aXZlLWltYWdl",
									},
								},
							},
						},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/v1beta/models/veo-3.1-generate-preview:predictLongRunning"):
			if err := json.NewDecoder(r.Body).Decode(&veoBody); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/veo-regression-1",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unexpected endpoint"}}`))
		}
	}))
	defer server.Close()

	p := geminiprov.NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Timeout: 15 * time.Second,
		},
	}, logger)

	imagenResp, err := p.GenerateImage(ctx, &llm.ImageGenerationRequest{
		Model:          "imagen-4.0-generate-001",
		Prompt:         "a cat in watercolor",
		NegativePrompt: "blurry",
		N:              2,
		Size:           "1024x1024",
	})
	if err != nil {
		return fmt.Errorf("imagen routing call failed: %w", err)
	}
	if len(imagenResp.Data) == 0 || imagenResp.Data[0].B64JSON == "" {
		return fmt.Errorf("imagen response parse failed: %+v", imagenResp)
	}

	geminiImgResp, err := p.GenerateImage(ctx, &llm.ImageGenerationRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "a mountain at sunrise",
		N:      1,
		Size:   "1536x1024",
	})
	if err != nil {
		return fmt.Errorf("gemini-image routing call failed: %w", err)
	}
	if len(geminiImgResp.Data) == 0 || geminiImgResp.Data[0].B64JSON == "" {
		return fmt.Errorf("gemini-image response parse failed: %+v", geminiImgResp)
	}

	veoResp, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{
		Model:      "veo-3.1-generate-preview",
		Prompt:     "a short cinematic drone shot over mountains",
		Duration:   5,
		FPS:        24,
		Resolution: "1920x1080",
		Style:      "cinematic",
	})
	if err != nil {
		return fmt.Errorf("veo routing call failed: %w", err)
	}
	if veoResp.ID == "" {
		return fmt.Errorf("veo response parse failed: empty id")
	}

	if _, ok := imagenBody["instances"].([]any); !ok {
		return fmt.Errorf("imagen request should contain instances")
	}
	imagenParams, ok := imagenBody["parameters"].(map[string]any)
	if !ok {
		return fmt.Errorf("imagen request should contain parameters")
	}
	if got := asString(imagenParams["aspectRatio"]); got != "1:1" {
		return fmt.Errorf("imagen aspectRatio mismatch: got=%q want=1:1", got)
	}

	if _, ok := geminiImageBody["contents"].([]any); !ok {
		return fmt.Errorf("gemini-image request should contain contents")
	}
	if _, hasInstances := geminiImageBody["instances"]; hasInstances {
		return fmt.Errorf("gemini-image request should not contain instances")
	}
	genCfg, ok := geminiImageBody["generationConfig"].(map[string]any)
	if !ok {
		return fmt.Errorf("gemini-image request should contain generationConfig")
	}
	if got := asString(genCfg["aspectRatio"]); got != "3:2" {
		return fmt.Errorf("gemini-image aspectRatio mismatch: got=%q want=3:2", got)
	}

	if _, ok := veoBody["instances"].([]any); !ok {
		return fmt.Errorf("veo request should contain instances")
	}
	veoParams, ok := veoBody["parameters"].(map[string]any)
	if !ok {
		return fmt.Errorf("veo request should contain parameters")
	}
	if got := asString(veoParams["aspectRatio"]); got != "16:9" {
		return fmt.Errorf("veo aspectRatio mismatch: got=%q want=16:9", got)
	}

	logger.Info("test I: gemini model-aware adapter regression done",
		zap.String("imagen_aspect_ratio", asString(imagenParams["aspectRatio"])),
		zap.String("gemini_image_aspect_ratio", asString(genCfg["aspectRatio"])),
		zap.String("veo_operation_id", veoResp.ID),
		zap.String("veo_aspect_ratio", asString(veoParams["aspectRatio"])),
	)
	return nil
}

func runAnthropicCompatEndpointRegression(ctx context.Context, logger *zap.Logger) error {
	logger.Info("test M: anthropic compatibility endpoint regression start")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"models/glm-5","owned_by":"openai-compatible"}]}`))
		case "/v1/messages":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "msg_compat_livecheck",
				"type":       "message",
				"role":       "assistant",
				"model":      "glm-5",
				"content":    "OK",
				"stopReason": "end_turn",
				"usage": map[string]any{
					"prompt_tokens":     12,
					"completion_tokens": 5,
					"total_tokens":      17,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unexpected endpoint"}}`))
		}
	}))
	defer server.Close()

	p := claudeprov.NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "glm-5",
			Timeout: 15 * time.Second,
		},
	}, logger)

	models, err := p.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("anthropic compat list models failed: %w", err)
	}
	if !hasModel(models, "glm-5") {
		return fmt.Errorf("anthropic compat model glm-5 not found in models envelope")
	}

	resp, err := p.Completion(ctx, &llm.ChatRequest{
		Model: "glm-5",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "Reply with exactly: OK"},
		},
	})
	if err != nil {
		return fmt.Errorf("anthropic compat completion failed: %w", err)
	}
	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return fmt.Errorf("anthropic compat no choice: %w", err)
	}
	if strings.TrimSpace(choice.Message.Content) != "OK" {
		return fmt.Errorf("anthropic compat content mismatch: got=%q want=OK", strings.TrimSpace(choice.Message.Content))
	}
	if resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 17 {
		return fmt.Errorf("anthropic compat usage mismatch: got=%+v", resp.Usage)
	}

	logger.Info("test M: anthropic compatibility endpoint regression done",
		zap.String("model", resp.Model),
		zap.Int("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int("completion_tokens", resp.Usage.CompletionTokens),
	)
	return nil
}

func runGeminiCompatEndpointRegression(ctx context.Context, logger *zap.Logger) error {
	logger.Info("test N: gemini compatibility endpoint regression start")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1beta/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"glm-5","max_input_tokens":128000,"max_output_tokens":8192}]}`))
		case "/v1beta/models/glm-5:generateContent":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"response_id":"resp_gemini_compat",
				"model_version":"glm-5",
				"candidates":[
					{
						"index":0,
						"finish_reason":"STOP",
						"content":{
							"role":"model",
							"parts":[
								{"text":"OK"},
								{"function_call":{"name":"lookup","arguments":{"city":"Shanghai"}}}
							]
						}
					}
				],
				"usage_metadata":{
					"prompt_token_count":9,
					"candidates_token_count":4,
					"total_token_count":13,
					"thoughts_token_count":1
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unexpected endpoint"}}`))
		}
	}))
	defer server.Close()

	p := geminiprov.NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "glm-5",
			Timeout: 15 * time.Second,
		},
	}, logger)

	models, err := p.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("gemini compat list models failed: %w", err)
	}
	if !hasModel(models, "glm-5") {
		return fmt.Errorf("gemini compat model glm-5 not found in data envelope")
	}

	resp, err := p.Completion(ctx, &llm.ChatRequest{
		Model: "glm-5",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "Reply with exactly: OK"},
		},
	})
	if err != nil {
		return fmt.Errorf("gemini compat completion failed: %w", err)
	}
	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return fmt.Errorf("gemini compat no choice: %w", err)
	}
	if strings.TrimSpace(choice.Message.Content) != "OK" {
		return fmt.Errorf("gemini compat content mismatch: got=%q want=OK", strings.TrimSpace(choice.Message.Content))
	}
	if len(choice.Message.ToolCalls) != 1 {
		return fmt.Errorf("gemini compat expected 1 tool call, got %d", len(choice.Message.ToolCalls))
	}
	if choice.Message.ToolCalls[0].Name != "lookup" {
		return fmt.Errorf("gemini compat tool call name mismatch: got=%q", choice.Message.ToolCalls[0].Name)
	}
	if resp.Usage.PromptTokens != 9 || resp.Usage.CompletionTokens != 4 || resp.Usage.TotalTokens != 13 {
		return fmt.Errorf("gemini compat usage mismatch: got=%+v", resp.Usage)
	}

	logger.Info("test N: gemini compatibility endpoint regression done",
		zap.String("model", resp.Model),
		zap.Int("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int("completion_tokens", resp.Usage.CompletionTokens),
	)
	return nil
}

func extractResponsesWebSearchTool(body map[string]any) (map[string]any, error) {
	if body == nil {
		return nil, fmt.Errorf("empty request body")
	}
	toolsAny, ok := body["tools"]
	if !ok {
		return nil, fmt.Errorf("tools field missing")
	}
	tools, ok := toolsAny.([]any)
	if !ok {
		return nil, fmt.Errorf("tools field is not array")
	}
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if asString(tool["type"]) == "web_search" {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("web_search tool not found")
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
