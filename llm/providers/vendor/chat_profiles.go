package vendor

import (
	"fmt"
	"net/http"
	"strings"

	llm "github.com/BaSui01/agentflow/llm/core"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type ChatCapabilityMatrix struct {
	NativeSDK         bool
	NativeToolCalling bool
	StructuredOutput  bool
	Streaming         bool
}

type compatProviderProfile struct {
	Code            string
	DefaultBaseURL  string
	FallbackModel   string
	EndpointPath    string
	AuthHeaderName  string
	SupportsTools   func(ChatProviderConfig) *bool
	RequestHook     func(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest)
	ValidateRequest func(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) error
	BuildHeaders    func(ChatProviderConfig) func(req *http.Request, apiKey string)
	ResolveName     func(ChatProviderConfig) string
	ResolveBaseURL  func(ChatProviderConfig) string
	Capabilities    ChatCapabilityMatrix
}

var compatProviderProfiles = map[string]compatProviderProfile{
	"deepseek": {
		Code:           "deepseek",
		DefaultBaseURL: "https://api.deepseek.com",
		FallbackModel:  "deepseek-chat",
		EndpointPath:   "/chat/completions",
		RequestHook:    deepseekRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"qwen": {
		Code:            "qwen",
		DefaultBaseURL:  "https://dashscope.aliyuncs.com",
		FallbackModel:   "qwen3-max-2026-01-23",
		EndpointPath:    "/compatible-mode/v1/chat/completions",
		RequestHook:     qwenRequestHook,
		ValidateRequest: validateQwenRequest,
		Capabilities:    compatCapabilities(true),
	},
	"glm": {
		Code:           "glm",
		DefaultBaseURL: "https://open.bigmodel.cn",
		FallbackModel:  "glm-5.1",
		EndpointPath:   "/api/paas/v4/chat/completions",
		RequestHook:    glmRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"grok": {
		Code:            "grok",
		DefaultBaseURL:  "https://api.x.ai",
		FallbackModel:   "grok-4.20",
		RequestHook:     grokRequestHook,
		ValidateRequest: validateGrokRequest,
		Capabilities:    compatCapabilities(true),
	},
	"kimi": {
		Code:            "kimi",
		DefaultBaseURL:  "https://api.moonshot.cn",
		FallbackModel:   "kimi-k2.5",
		RequestHook:     kimiRequestHook,
		ValidateRequest: validateKimiRequest,
		Capabilities:    compatCapabilities(true),
	},
	"mistral": {
		Code:           "mistral",
		DefaultBaseURL: "https://api.mistral.ai",
		FallbackModel:  "mistral-medium-latest",
		RequestHook:    mistralRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"minimax": {
		Code:           "minimax",
		DefaultBaseURL: "https://api.minimax.io",
		FallbackModel:  "MiniMax-M2.7",
		SupportsTools:  minimaxSupportsTools,
		Capabilities:   compatCapabilities(true),
	},
	"hunyuan": {
		Code:           "hunyuan",
		DefaultBaseURL: "https://api.hunyuan.cloud.tencent.com",
		FallbackModel:  "hunyuan-t1-latest",
		RequestHook:    hunyuanRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"doubao": {
		Code:           "doubao",
		DefaultBaseURL: "https://ark.cn-beijing.volces.com",
		FallbackModel:  "Doubao-1.5-pro-32k",
		EndpointPath:   "/api/v3/chat/completions",
		RequestHook:    doubaoRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"llama": {
		Code:           "llama",
		FallbackModel:  "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		ResolveName:    resolveLlamaProviderName,
		ResolveBaseURL: resolveLlamaBaseURL,
		Capabilities:   compatCapabilities(true),
	},
}

func compatCapabilities(nativeTools bool) ChatCapabilityMatrix {
	return ChatCapabilityMatrix{
		NativeSDK:         false,
		NativeToolCalling: nativeTools,
		StructuredOutput:  true,
		Streaming:         true,
	}
}

func LookupChatCapabilityMatrix(providerCode string) (ChatCapabilityMatrix, bool) {
	code := strings.ToLower(strings.TrimSpace(providerCode))
	switch code {
	case "openai", "openai-responses", "openai-responses-api":
		return ChatCapabilityMatrix{NativeSDK: true, NativeToolCalling: true, StructuredOutput: true, Streaming: true}, true
	case "anthropic", "claude", "anthropic-sdk", "anthropic-sdk-go", "claude-sdk":
		return ChatCapabilityMatrix{NativeSDK: true, NativeToolCalling: true, StructuredOutput: true, Streaming: true}, true
	case "gemini", "google", "google-genai", "vertex-ai", "vertexai", "gemini-vertex":
		return ChatCapabilityMatrix{NativeSDK: true, NativeToolCalling: true, StructuredOutput: true, Streaming: true}, true
	default:
		profile, ok := compatProviderProfiles[code]
		if !ok {
			return ChatCapabilityMatrix{NativeSDK: false, NativeToolCalling: false, StructuredOutput: true, Streaming: true}, false
		}
		return profile.Capabilities, true
	}
}

func newCompatBuiltInChatProvider(providerCode string, cfg ChatProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	profile, ok := compatProviderProfiles[providerCode]
	if !ok {
		return nil, fmt.Errorf("compat provider profile %q not found", providerCode)
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if profile.ResolveBaseURL != nil {
		baseURL = strings.TrimSpace(profile.ResolveBaseURL(cfg))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(profile.DefaultBaseURL)
	}
	if baseURL == "" {
		return nil, fmt.Errorf("provider %q requires base_url", providerCode)
	}

	providerName := profile.Code
	if profile.ResolveName != nil {
		providerName = strings.TrimSpace(profile.ResolveName(cfg))
	}
	if providerName == "" {
		providerName = profile.Code
	}

	compatCfg := openaicompat.Config{
		ProviderName:    providerName,
		APIKey:          cfg.APIKey,
		APIKeys:         cfg.APIKeys,
		BaseURL:         baseURL,
		DefaultModel:    cfg.Model,
		FallbackModel:   profile.FallbackModel,
		Timeout:         cfg.Timeout,
		EndpointPath:    profile.EndpointPath,
		AuthHeaderName:  profile.AuthHeaderName,
		RequestHook:     profile.RequestHook,
		ValidateRequest: profile.ValidateRequest,
	}
	if profile.SupportsTools != nil {
		compatCfg.SupportsTools = profile.SupportsTools(cfg)
	}
	if profile.BuildHeaders != nil {
		compatCfg.BuildHeaders = profile.BuildHeaders(cfg)
	}
	return openaicompat.New(compatCfg, logger), nil
}

func resolveLlamaProviderName(cfg ChatProviderConfig) string {
	subProvider := strings.ToLower(strings.TrimSpace(extraString(cfg.Extra, "provider")))
	if subProvider == "" {
		subProvider = "together"
	}
	return fmt.Sprintf("llama-%s", subProvider)
}

func resolveLlamaBaseURL(cfg ChatProviderConfig) string {
	if trimmed := strings.TrimSpace(cfg.BaseURL); trimmed != "" {
		return trimmed
	}
	switch strings.ToLower(strings.TrimSpace(extraString(cfg.Extra, "provider"))) {
	case "replicate":
		return "https://api.replicate.com"
	case "openrouter":
		return "https://openrouter.ai/api"
	default:
		return "https://api.together.xyz"
	}
}

func minimaxSupportsTools(cfg ChatProviderConfig) *bool {
	model := strings.TrimSpace(cfg.Model)
	supportsTools := !strings.HasPrefix(model, "abab")
	return &supportsTools
}

// resolveCompatThinkingMode returns the effective thinking mode for compat providers.
// ThinkingType takes priority over legacy ReasoningMode.
func resolveCompatThinkingMode(req *llm.ChatRequest) string {
	if req == nil {
		return ""
	}
	if tt := strings.ToLower(strings.TrimSpace(req.ThinkingType)); tt != "" {
		return tt
	}
	return strings.ToLower(strings.TrimSpace(req.ReasoningMode))
}

func deepseekRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" {
		if req.Model == "" {
			body.Model = "deepseek-reasoner"
		}
		body.Temperature = 0
		body.TopP = 0
	}
}

func qwenRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" || mode == "enabled" {
		if req.Model == "" {
			body.Model = "qwen3-max-2026-01-23"
		}
		enableThinking := true
		incrementalOutput := true
		body.EnableThinking = &enableThinking
		body.IncrementalOutput = &incrementalOutput
	}
}

func glmRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" {
		if req.Model == "" {
			body.Model = "glm-z1-flash"
		}
	}
}

func grokRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" {
		if req.Model == "" {
			body.Model = "grok-4.20-reasoning"
		}
	}
}

func kimiRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" || mode == "enabled" {
		if req.Model == "" {
			body.Model = "kimi-k2.5"
		}
		body.Thinking = &providerbase.Thinking{Type: "enabled"}
	} else if mode == "disabled" {
		if req.Model == "" {
			body.Model = "kimi-k2.5"
		}
		body.Thinking = &providerbase.Thinking{Type: "disabled"}
	}
}

func mistralRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" {
		if req.Model == "" {
			body.Model = "magistral-medium-latest"
		}
	}
}

func doubaoRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode != "" {
		switch mode {
		case "thinking", "enabled":
			body.Thinking = &providerbase.Thinking{Type: "enabled"}
		case "disabled":
			body.Thinking = &providerbase.Thinking{Type: "disabled"}
		case "auto":
			body.Thinking = &providerbase.Thinking{Type: "auto"}
		}
	}
}

func hunyuanRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	mode := resolveCompatThinkingMode(req)
	if mode == "thinking" || mode == "extended" {
		if req.Model == "" {
			body.Model = "hunyuan-t1"
		}
		return
	}
	if len(body.Tools) > 0 && req.Model == "" &&
		body.Model != "hunyuan-functioncall" && body.Model != "hunyuan-t1" {
		body.Model = "hunyuan-functioncall"
	}
}

func extraString(extra map[string]any, key string) string {
	if len(extra) == 0 {
		return ""
	}
	value, _ := extra[key].(string)
	return value
}

func validateQwenRequest(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) error {
	if req == nil {
		return nil
	}
	if !isReasoningModeEnabled(req) {
		return nil
	}
	if req.ResponseFormat == nil {
		return nil
	}
	switch req.ResponseFormat.Type {
	case llm.ResponseFormatJSONObject, llm.ResponseFormatJSONSchema:
		return invalidCompatRequest("qwen", "Qwen thinking mode does not support structured JSON response_format; disable thinking or remove response_format")
	default:
		return nil
	}
}

func validateGrokRequest(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) error {
	if req == nil {
		return nil
	}
	if !isGrokReasoningRequest(req, body) {
		return nil
	}
	if len(req.Stop) > 0 {
		return invalidCompatRequest("grok", "xAI Grok reasoning models do not support stop")
	}
	if req.FrequencyPenalty != nil {
		return invalidCompatRequest("grok", "xAI Grok reasoning models do not support frequency_penalty")
	}
	if req.PresencePenalty != nil {
		return invalidCompatRequest("grok", "xAI Grok reasoning models do not support presence_penalty")
	}
	if strings.TrimSpace(req.ReasoningEffort) != "" {
		return invalidCompatRequest("grok", "xAI Grok reasoning models do not support reasoning_effort")
	}
	return nil
}

func validateKimiRequest(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) error {
	if req == nil {
		return nil
	}
	if !isKimiThinkingRequest(req, body) {
		return nil
	}
	if req.ToolChoice != nil {
		mode := providerbase.NormalizeToolChoice(req.ToolChoice).Mode
		if mode != "" && mode != "auto" {
			return invalidCompatRequest("kimi", "Kimi thinking mode only supports tool_choice auto")
		}
	}
	if req.Temperature != 0 {
		return invalidCompatRequest("kimi", "Kimi thinking mode does not support custom temperature")
	}
	if req.TopP != 0 {
		return invalidCompatRequest("kimi", "Kimi thinking mode does not support custom top_p")
	}
	if req.N != nil {
		return invalidCompatRequest("kimi", "Kimi thinking mode does not support custom n")
	}
	if req.FrequencyPenalty != nil {
		return invalidCompatRequest("kimi", "Kimi thinking mode does not support frequency_penalty")
	}
	if req.PresencePenalty != nil {
		return invalidCompatRequest("kimi", "Kimi thinking mode does not support presence_penalty")
	}
	if req.RepetitionPenalty != nil {
		return invalidCompatRequest("kimi", "Kimi thinking mode does not support repetition_penalty")
	}
	return nil
}

func invalidCompatRequest(provider, message string) error {
	return &types.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
	}
}

func isReasoningModeEnabled(req *llm.ChatRequest) bool {
	if req == nil {
		return false
	}
	// ThinkingType takes priority over legacy ReasoningMode in compat layer.
	mode := strings.ToLower(strings.TrimSpace(req.ThinkingType))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(req.ReasoningMode))
	}
	switch mode {
	case "thinking", "extended", "enabled", "adaptive":
		return true
	default:
		return false
	}
}

func isGrokReasoningRequest(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) bool {
	if isReasoningModeEnabled(req) {
		return true
	}
	model := ""
	if req != nil {
		model = strings.ToLower(strings.TrimSpace(req.Model))
	}
	if model == "" && body != nil {
		model = strings.ToLower(strings.TrimSpace(body.Model))
	}
	return strings.Contains(model, "reasoning")
}

func isKimiThinkingRequest(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) bool {
	if isReasoningModeEnabled(req) {
		return true
	}
	model := ""
	if req != nil {
		model = strings.ToLower(strings.TrimSpace(req.Model))
	}
	if model == "" && body != nil {
		model = strings.ToLower(strings.TrimSpace(body.Model))
	}
	return strings.Contains(model, "thinking") || model == "k1"
}
