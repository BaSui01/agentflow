package vendor

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

type ChatCapabilityMatrix struct {
	NativeSDK         bool
	NativeToolCalling bool
	StructuredOutput  bool
	Streaming         bool
}

type compatProviderProfile struct {
	Code           string
	DefaultBaseURL string
	FallbackModel  string
	EndpointPath   string
	AuthHeaderName string
	SupportsTools  func(ChatProviderConfig) *bool
	RequestHook    func(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest)
	BuildHeaders   func(ChatProviderConfig) func(req *http.Request, apiKey string)
	ResolveName    func(ChatProviderConfig) string
	ResolveBaseURL func(ChatProviderConfig) string
	Capabilities   ChatCapabilityMatrix
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
		Code:           "qwen",
		DefaultBaseURL: "https://dashscope.aliyuncs.com",
		FallbackModel:  "qwen3-235b-a22b",
		EndpointPath:   "/compatible-mode/v1/chat/completions",
		RequestHook:    qwenRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"glm": {
		Code:           "glm",
		DefaultBaseURL: "https://open.bigmodel.cn",
		FallbackModel:  "glm-4-plus",
		EndpointPath:   "/api/paas/v4/chat/completions",
		RequestHook:    glmRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"grok": {
		Code:           "grok",
		DefaultBaseURL: "https://api.x.ai",
		FallbackModel:  "grok-3",
		RequestHook:    grokRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"kimi": {
		Code:           "kimi",
		DefaultBaseURL: "https://api.moonshot.cn",
		FallbackModel:  "moonshot-v1-32k",
		RequestHook:    kimiRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"mistral": {
		Code:           "mistral",
		DefaultBaseURL: "https://api.mistral.ai",
		FallbackModel:  "mistral-large-latest",
		RequestHook:    mistralRequestHook,
		Capabilities:   compatCapabilities(true),
	},
	"minimax": {
		Code:           "minimax",
		DefaultBaseURL: "https://api.minimax.io",
		FallbackModel:  "MiniMax-Text-01",
		SupportsTools:  minimaxSupportsTools,
		Capabilities:   compatCapabilities(true),
	},
	"hunyuan": {
		Code:           "hunyuan",
		DefaultBaseURL: "https://api.hunyuan.cloud.tencent.com",
		FallbackModel:  "hunyuan-turbos-latest",
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
		ProviderName:   providerName,
		APIKey:         cfg.APIKey,
		APIKeys:        cfg.APIKeys,
		BaseURL:        baseURL,
		DefaultModel:   cfg.Model,
		FallbackModel:  profile.FallbackModel,
		Timeout:        cfg.Timeout,
		EndpointPath:   profile.EndpointPath,
		AuthHeaderName: profile.AuthHeaderName,
		RequestHook:    profile.RequestHook,
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

func deepseekRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "deepseek-reasoner"
		}
		body.Temperature = 0
		body.TopP = 0
	}
}

func qwenRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "qwen3-max"
		}
	}
}

func glmRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "glm-z1-flash"
		}
	}
}

func grokRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "grok-3-mini"
		}
	}
}

func kimiRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "k1"
		}
	}
}

func mistralRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "magistral-medium-latest"
		}
	}
}

func doubaoRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode != "" {
		switch req.ReasoningMode {
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
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
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
