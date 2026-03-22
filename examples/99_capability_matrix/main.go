package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	claude "github.com/BaSui01/agentflow/llm/providers/anthropic"
	"github.com/BaSui01/agentflow/llm/providers/gemini"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultOpenAIModel    = "gpt-4o-mini"
	defaultAnthropicModel = "claude-3-5-sonnet-20241022"
	defaultGeminiModel    = "gemini-2.5-flash"
	defaultTimeout        = 75 * time.Second
	suiteTimeout          = 6 * time.Minute
)

type providerScenario struct {
	Name         string
	Family       string
	EndpointMode string
	Provider     llm.Provider
	Model        string
}

type matrixResult struct {
	Scenario string
	Check    string
	Status   string
	Elapsed  time.Duration
	Detail   string
}

var results []matrixResult

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	scenarios := buildScenarios(logger)
	if len(scenarios) == 0 {
		printMissingEnvHint()
		return
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║   AgentFlow Provider Capability Matrix                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	for _, scenario := range scenarios {
		runScenario(context.Background(), scenario)
	}

	printSummary()
}

func buildScenarios(logger *zap.Logger) []providerScenario {
	var scenarios []providerScenario

	if apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); apiKey != "" {
		baseURL := envOrDefault("OPENAI_BASE_URL", "https://api.openai.com")
		model := envOrDefault("OPENAI_MODEL", defaultOpenAIModel)

		chatCfg := providers.OpenAIConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Model:   model,
				Timeout: defaultTimeout,
			},
			UseResponsesAPI: false,
		}
		scenarios = append(scenarios, providerScenario{
			Name:         "OpenAI Chat Completions",
			Family:       "openai",
			EndpointMode: "chat_completions",
			Provider:     openai.NewOpenAIProvider(chatCfg, logger),
			Model:        model,
		})

		responsesCfg := chatCfg
		responsesCfg.UseResponsesAPI = true
		scenarios = append(scenarios, providerScenario{
			Name:         "OpenAI Responses",
			Family:       "openai",
			EndpointMode: "responses",
			Provider:     openai.NewOpenAIProvider(responsesCfg, logger),
			Model:        model,
		})
	}

	if apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); apiKey != "" {
		cfg := providers.ClaudeConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  apiKey,
				BaseURL: envOrDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
				Model:   envOrDefault("ANTHROPIC_MODEL", defaultAnthropicModel),
				Timeout: defaultTimeout,
			},
			AnthropicVersion: strings.TrimSpace(os.Getenv("ANTHROPIC_VERSION")),
		}
		scenarios = append(scenarios, providerScenario{
			Name:         "Anthropic Messages",
			Family:       "anthropic",
			EndpointMode: "messages",
			Provider:     claude.NewClaudeProvider(cfg, logger),
			Model:        cfg.Model,
		})
	}

	if apiKey := firstNonEmptyEnv("GEMINI_API_KEY", "GOOGLE_API_KEY"); apiKey != "" {
		model := envOrDefault("GEMINI_MODEL", defaultGeminiModel)

		googleAICfg := providers.GeminiConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  apiKey,
				BaseURL: strings.TrimSpace(os.Getenv("GEMINI_BASE_URL")),
				Model:   model,
				Timeout: defaultTimeout,
			},
			AuthType: envOrDefault("GEMINI_AUTH_TYPE", "api_key"),
		}
		scenarios = append(scenarios, providerScenario{
			Name:         "Gemini Google AI",
			Family:       "gemini",
			EndpointMode: "google_ai",
			Provider:     gemini.NewGeminiProvider(googleAICfg, logger),
			Model:        model,
		})

		if projectID := firstNonEmptyEnv("GEMINI_PROJECT_ID", "VERTEX_PROJECT_ID"); projectID != "" {
			vertexCfg := providers.GeminiConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  apiKey,
					BaseURL: strings.TrimSpace(os.Getenv("GEMINI_VERTEX_BASE_URL")),
					Model:   model,
					Timeout: defaultTimeout,
				},
				ProjectID: projectID,
				Region:    envOrDefault("GEMINI_REGION", "us-central1"),
				AuthType:  envOrDefault("GEMINI_VERTEX_AUTH_TYPE", envOrDefault("GEMINI_AUTH_TYPE", "api_key")),
			}
			scenarios = append(scenarios, providerScenario{
				Name:         "Gemini Vertex AI",
				Family:       "gemini",
				EndpointMode: "vertex_ai",
				Provider:     gemini.NewGeminiProvider(vertexCfg, logger),
				Model:        model,
			})
		}
	}

	return scenarios
}

func runScenario(parent context.Context, scenario providerScenario) {
	fmt.Printf("\n=== %s ===\n", scenario.Name)
	printEndpoints(scenario.Provider.Endpoints())

	ctx, cancel := context.WithTimeout(parent, suiteTimeout)
	defer cancel()

	testProviderMetadata(scenario)
	testNativeFunctionCallingSupport(scenario)
	testHealthCheck(ctx, scenario)
	testModelsEndpoint(ctx, scenario)
	testCompletionEndpoint(ctx, scenario)
	testStreamEndpoint(ctx, scenario)
	testMultiTurn(ctx, scenario)
	testStopSequence(ctx, scenario)
	testStructuredOutput(ctx, scenario)
	testToolCalling(ctx, scenario)
	testResponsesStatefulConversation(ctx, scenario)
}

func testProviderMetadata(scenario providerScenario) {
	start := time.Now()
	endpoints := scenario.Provider.Endpoints()
	switch {
	case strings.TrimSpace(scenario.Provider.Name()) == "":
		record(scenario.Name, "Provider Metadata", "FAIL", time.Since(start), "provider name is empty")
	case strings.TrimSpace(endpoints.BaseURL) == "":
		record(scenario.Name, "Provider Metadata", "FAIL", time.Since(start), "base_url is empty")
	case strings.TrimSpace(endpoints.Completion) == "":
		record(scenario.Name, "Provider Metadata", "FAIL", time.Since(start), "completion endpoint is empty")
	case strings.TrimSpace(endpoints.Models) == "":
		record(scenario.Name, "Provider Metadata", "FAIL", time.Since(start), "models endpoint is empty")
	default:
		record(scenario.Name, "Provider Metadata", "PASS", time.Since(start), fmt.Sprintf("name=%s mode=%s", scenario.Provider.Name(), scenario.EndpointMode))
	}
}

func testNativeFunctionCallingSupport(scenario providerScenario) {
	start := time.Now()
	if scenario.Provider.SupportsNativeFunctionCalling() {
		record(scenario.Name, "Native Function Call", "PASS", time.Since(start), "provider reports native function calling support")
		return
	}
	record(scenario.Name, "Native Function Call", "FAIL", time.Since(start), "provider reports native function calling unsupported")
}

func testHealthCheck(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	start := time.Now()
	status, err := scenario.Provider.HealthCheck(ctx)
	if err != nil {
		record(scenario.Name, "Health Check", "FAIL", time.Since(start), err.Error())
		return
	}
	if status == nil {
		record(scenario.Name, "Health Check", "FAIL", time.Since(start), "health status is nil")
		return
	}
	if !status.Healthy {
		record(scenario.Name, "Health Check", "WARN", time.Since(start), fmt.Sprintf("healthy=false latency=%v msg=%s", status.Latency.Round(time.Millisecond), status.Message))
		return
	}
	record(scenario.Name, "Health Check", "PASS", time.Since(start), fmt.Sprintf("healthy=true latency=%v", status.Latency.Round(time.Millisecond)))
}

func testModelsEndpoint(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	start := time.Now()
	models, err := scenario.Provider.ListModels(ctx)
	if err != nil {
		record(scenario.Name, "Models Endpoint", "FAIL", time.Since(start), fmt.Sprintf("%s -> %v", scenario.Provider.Endpoints().Models, err))
		return
	}

	if len(models) == 0 {
		record(scenario.Name, "Models Endpoint", "WARN", time.Since(start), fmt.Sprintf("%s -> 0 models", scenario.Provider.Endpoints().Models))
		return
	}

	record(scenario.Name, "Models Endpoint", "PASS", time.Since(start), fmt.Sprintf("%s -> %d models, sample=%s", scenario.Provider.Endpoints().Models, len(models), models[0].ID))
}

func testCompletionEndpoint(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model: scenario.Model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你是一个简洁的技术助手。"},
			{Role: llm.RoleUser, Content: "用一句中文解释什么是 AgentFlow。"},
		},
		MaxTokens:   128,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Completion Endpoint", "FAIL", time.Since(start), fmt.Sprintf("%s -> %v", scenario.Provider.Endpoints().Completion, err))
		return
	}

	content := responseText(resp)
	if content == "" {
		record(scenario.Name, "Completion Endpoint", "WARN", time.Since(start), "response received but content is empty")
		return
	}

	record(scenario.Name, "Completion Endpoint", "PASS", time.Since(start), fmt.Sprintf("%s -> %s", scenario.Provider.Endpoints().Completion, trunc(content, 90)))
}

func testStreamEndpoint(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	start := time.Now()
	ch, err := scenario.Provider.Stream(ctx, &llm.ChatRequest{
		Model: scenario.Model,
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "用一句中文说明流式响应的作用。"},
		},
		MaxTokens:   128,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Stream Endpoint", "FAIL", time.Since(start), fmt.Sprintf("%s -> %v", streamEndpointLabel(scenario.Provider.Endpoints()), err))
		return
	}

	var builder strings.Builder
	chunkCount := 0
	for chunk := range ch {
		if chunk.Err != nil {
			record(scenario.Name, "Stream Endpoint", "FAIL", time.Since(start), fmt.Sprintf("%s -> %v", streamEndpointLabel(scenario.Provider.Endpoints()), chunk.Err))
			return
		}

		text := messageText(chunk.Delta)
		if text == "" {
			continue
		}

		builder.WriteString(text)
		chunkCount++
	}

	content := strings.TrimSpace(builder.String())
	if content == "" {
		record(scenario.Name, "Stream Endpoint", "WARN", time.Since(start), fmt.Sprintf("%s -> stream closed without text chunks", streamEndpointLabel(scenario.Provider.Endpoints())))
		return
	}

	record(scenario.Name, "Stream Endpoint", "PASS", time.Since(start), fmt.Sprintf("%s -> %d chunks, %s", streamEndpointLabel(scenario.Provider.Endpoints()), chunkCount, trunc(content, 90)))
}

func testMultiTurn(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	start := time.Now()
	messages := []types.Message{
		{Role: llm.RoleSystem, Content: "你是一个记忆准确的助手，只回答必要内容。"},
		{Role: llm.RoleUser, Content: "我叫小明，今年25岁。记住这些信息，只回复“记住了”。"},
	}

	resp1, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model:       scenario.Model,
		Messages:    messages,
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Multi Turn", "FAIL", time.Since(start), fmt.Sprintf("round1 failed: %v", err))
		return
	}

	messages = append(messages,
		types.Message{Role: llm.RoleAssistant, Content: responseText(resp1)},
		types.Message{Role: llm.RoleUser, Content: "我叫什么？今年几岁？只直接回答。"},
	)

	resp2, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model:       scenario.Model,
		Messages:    messages,
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Multi Turn", "FAIL", time.Since(start), fmt.Sprintf("round2 failed: %v", err))
		return
	}

	answer := responseText(resp2)
	hasName := strings.Contains(answer, "小明")
	hasAge := strings.Contains(answer, "25")

	switch {
	case hasName && hasAge:
		record(scenario.Name, "Multi Turn", "PASS", time.Since(start), trunc(answer, 90))
	case hasName || hasAge:
		record(scenario.Name, "Multi Turn", "WARN", time.Since(start), "partial memory: "+trunc(answer, 90))
	default:
		record(scenario.Name, "Multi Turn", "FAIL", time.Since(start), trunc(answer, 90))
	}
}

func testStructuredOutput(parent context.Context, scenario providerScenario) {
	if !supportsStructuredOutput(scenario.Provider) {
		record(scenario.Name, "Structured Output", "SKIP", 0, "provider does not expose native structured output support")
		return
	}

	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model: scenario.Model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你是一个结构化输出引擎。"},
			{Role: llm.RoleUser, Content: `输出一个对象，字段必须包含 language、creator、year。不要输出任何解释。`},
		},
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatJSONSchema,
			JSONSchema: &llm.JSONSchemaParam{
				Name:        "capability_matrix_object",
				Description: "provider structured output smoke test",
				Strict:      boolPtr(true),
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"language": map[string]any{"type": "string"},
						"creator":  map[string]any{"type": "string"},
						"year":     map[string]any{"type": "integer"},
					},
					"required":             []string{"language", "creator", "year"},
					"additionalProperties": false,
				},
			},
		},
		MaxTokens:   128,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Structured Output", "FAIL", time.Since(start), err.Error())
		return
	}

	content := extractJSON(responseText(resp))
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		record(scenario.Name, "Structured Output", "FAIL", time.Since(start), fmt.Sprintf("invalid json: %v | %s", err, trunc(content, 90)))
		return
	}

	if payload["language"] == nil || payload["creator"] == nil || payload["year"] == nil {
		record(scenario.Name, "Structured Output", "WARN", time.Since(start), trunc(content, 90))
		return
	}

	record(scenario.Name, "Structured Output", "PASS", time.Since(start), trunc(content, 90))
}

func testToolCalling(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model: scenario.Model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你必须先调用 get_weather 工具，禁止直接回答。"},
			{Role: llm.RoleUser, Content: "请查询上海天气。"},
		},
		Tools: []types.ToolSchema{
			{
				Name:        "get_weather",
				Description: "获取指定城市的天气信息",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string","description":"城市名称"}},"required":["city"]}`),
			},
		},
		ToolChoice:  "required",
		MaxTokens:   256,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Tool Calling", "FAIL", time.Since(start), err.Error())
		return
	}

	toolCalls := responseToolCalls(resp)
	if len(toolCalls) == 0 {
		record(scenario.Name, "Tool Calling", "WARN", time.Since(start), "no native tool call returned")
		return
	}

	record(scenario.Name, "Tool Calling", "PASS", time.Since(start), fmt.Sprintf("tool=%s args=%s", toolCalls[0].Name, trunc(string(toolCalls[0].Arguments), 90)))
}

func testStopSequence(parent context.Context, scenario providerScenario) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model: scenario.Model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "严格按要求输出，不要解释。"},
			{Role: llm.RoleUser, Content: "原样输出：alpha STOP beta"},
		},
		Stop:        []string{"STOP"},
		MaxTokens:   32,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Stop Sequence", "FAIL", time.Since(start), err.Error())
		return
	}

	content := responseText(resp)
	hasAlpha := strings.Contains(strings.ToLower(content), "alpha")
	hasBeta := strings.Contains(strings.ToLower(content), "beta")
	hasStop := strings.Contains(content, "STOP")

	switch {
	case hasAlpha && !hasBeta:
		record(scenario.Name, "Stop Sequence", "PASS", time.Since(start), trunc(content, 90))
	case hasStop:
		record(scenario.Name, "Stop Sequence", "WARN", time.Since(start), "stop token leaked: "+trunc(content, 90))
	default:
		record(scenario.Name, "Stop Sequence", "WARN", time.Since(start), trunc(content, 90))
	}
}

func testResponsesStatefulConversation(parent context.Context, scenario providerScenario) {
	if scenario.Family != "openai" || scenario.EndpointMode != "responses" {
		record(scenario.Name, "Stateful Response", "SKIP", 0, "only applicable to OpenAI Responses API")
		return
	}

	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()

	start := time.Now()
	firstResp, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model: scenario.Model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你是一个短记忆助手。"},
			{Role: llm.RoleUser, Content: "记住代号“蓝狐”，只回复“记住了”。"},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Stateful Response", "FAIL", time.Since(start), "round1: "+err.Error())
		return
	}
	if firstResp == nil || strings.TrimSpace(firstResp.ID) == "" {
		record(scenario.Name, "Stateful Response", "FAIL", time.Since(start), "round1 returned empty response id")
		return
	}

	secondResp, err := scenario.Provider.Completion(ctx, &llm.ChatRequest{
		Model:              scenario.Model,
		PreviousResponseID: firstResp.ID,
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "刚才记住的代号是什么？只回答代号。"},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		record(scenario.Name, "Stateful Response", "FAIL", time.Since(start), "round2: "+err.Error())
		return
	}

	answer := responseText(secondResp)
	if strings.Contains(answer, "蓝狐") {
		record(scenario.Name, "Stateful Response", "PASS", time.Since(start), fmt.Sprintf("previous_response_id=%s -> %s", firstResp.ID, trunc(answer, 90)))
		return
	}
	record(scenario.Name, "Stateful Response", "WARN", time.Since(start), fmt.Sprintf("response_id=%s but memory not observed: %s", firstResp.ID, trunc(answer, 90)))
}

func printEndpoints(endpoints llm.ProviderEndpoints) {
	fmt.Printf("  base_url:   %s\n", endpoints.BaseURL)
	fmt.Printf("  models:     %s\n", endpoints.Models)
	fmt.Printf("  completion: %s\n", endpoints.Completion)
	if strings.TrimSpace(endpoints.Stream) != "" {
		fmt.Printf("  stream:     %s\n", endpoints.Stream)
	}
}

func record(scenario, check, status string, elapsed time.Duration, detail string) {
	results = append(results, matrixResult{
		Scenario: scenario,
		Check:    check,
		Status:   status,
		Elapsed:  elapsed,
		Detail:   detail,
	})

	icon := map[string]string{
		"PASS": "PASS",
		"FAIL": "FAIL",
		"WARN": "WARN",
		"SKIP": "SKIP",
	}[status]
	fmt.Printf("  [%s] %-18s %8v  %s\n", icon, check, elapsed.Round(time.Millisecond), detail)
}

func printSummary() {
	fmt.Println("\n=== Summary ===")

	passCount := 0
	failCount := 0
	warnCount := 0
	skipCount := 0
	for _, result := range results {
		switch result.Status {
		case "PASS":
			passCount++
		case "FAIL":
			failCount++
		case "WARN":
			warnCount++
		case "SKIP":
			skipCount++
		}
	}

	fmt.Printf("Total=%d PASS=%d FAIL=%d WARN=%d SKIP=%d\n", len(results), passCount, failCount, warnCount, skipCount)
	for _, result := range results {
		fmt.Printf("  %-24s %-18s %-4s %8v  %s\n",
			result.Scenario,
			result.Check,
			result.Status,
			result.Elapsed.Round(time.Millisecond),
			result.Detail,
		)
	}
}

func printMissingEnvHint() {
	fmt.Println("No provider credentials found. Set at least one of the following:")
	fmt.Println("  OPENAI_API_KEY")
	fmt.Println("  ANTHROPIC_API_KEY")
	fmt.Println("  GEMINI_API_KEY or GOOGLE_API_KEY")
	fmt.Println()
	fmt.Println("Optional overrides:")
	fmt.Println("  OPENAI_BASE_URL, OPENAI_MODEL")
	fmt.Println("  ANTHROPIC_BASE_URL, ANTHROPIC_MODEL, ANTHROPIC_VERSION")
	fmt.Println("  GEMINI_BASE_URL, GEMINI_MODEL, GEMINI_AUTH_TYPE")
	fmt.Println("  GEMINI_PROJECT_ID, GEMINI_REGION, GEMINI_VERTEX_BASE_URL, GEMINI_VERTEX_AUTH_TYPE")
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func responseText(resp *llm.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return messageText(resp.Choices[0].Message)
}

func responseToolCalls(resp *llm.ChatResponse) []types.ToolCall {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	return resp.Choices[0].Message.ToolCalls
}

func messageText(msg types.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	if msg.ReasoningContent != nil {
		return strings.TrimSpace(*msg.ReasoningContent)
	}
	return ""
}

func extractJSON(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	for i, ch := range content {
		if ch == '{' || ch == '[' {
			return content[i:]
		}
	}
	return content
}

func trunc(value string, max int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

func streamEndpointLabel(endpoints llm.ProviderEndpoints) string {
	if strings.TrimSpace(endpoints.Stream) != "" {
		return endpoints.Stream
	}
	return endpoints.Completion
}

func supportsStructuredOutput(provider llm.Provider) bool {
	type structuredOutputProvider interface {
		SupportsStructuredOutput() bool
	}

	p, ok := provider.(structuredOutputProvider)
	return ok && p.SupportsStructuredOutput()
}

func boolPtr(v bool) *bool {
	return &v
}
