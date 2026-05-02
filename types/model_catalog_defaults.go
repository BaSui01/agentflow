package types

// DefaultModelCatalogVerifiedAt is the snapshot verification date for the
// built-in model catalog. It is intentionally date-scoped because upstream
// model availability and limits change frequently.
const DefaultModelCatalogVerifiedAt = "2026-05-02"

const (
	modelCatalogSourceOpenAIResponses  = "https://platform.openai.com/docs/api-reference/responses/create"
	modelCatalogSourceOpenAIModels     = "https://platform.openai.com/docs/models"
	modelCatalogSourceAnthropicModels  = "https://docs.claude.com/en/docs/about-claude/models/overview"
	modelCatalogSourceAnthropicAPI     = "https://docs.claude.com/en/api/messages"
	modelCatalogSourceGeminiModels     = "https://ai.google.dev/gemini-api/docs/models"
	modelCatalogSourceGeminiThinking   = "https://ai.google.dev/gemini-api/docs/thinking"
	modelCatalogSourceDeepSeekChat     = "https://api-docs.deepseek.com/api/create-chat-completion"
	modelCatalogSourceDeepSeekReasoner = "https://api-docs.deepseek.com/guides/reasoning_model"
	modelCatalogSourceQwenModels       = "https://www.alibabacloud.com/help/en/model-studio/user-guide/model/"
	modelCatalogSourceQwenAPI          = "https://www.alibabacloud.com/help/en/model-studio/use-qwen-by-calling-api"
	modelCatalogSourceXAIModels        = "https://docs.x.ai/docs/models/"
	modelCatalogSourceXAIReasoning     = "https://docs.x.ai/developers/model-capabilities/text/reasoning"
	modelCatalogSourceMistralModels    = "https://docs.mistral.ai/models/overview/"
	modelCatalogSourceMistralAPI       = "https://docs.mistral.ai/api/#tag/chat"
	modelCatalogSourceMoonshotAPI      = "https://platform.moonshot.cn/docs/api-reference"
)

// DefaultModelCatalog returns the built-in model descriptor snapshot used by
// routing, validation, and documentation examples when no external catalog is
// injected. Callers receive a fresh immutable lookup wrapper on each call.
func DefaultModelCatalog() *ModelCatalog {
	return NewModelCatalog(DefaultModelDescriptors())
}

// DefaultModelDescriptors returns a cloned date-scoped snapshot of mainstream
// chat / agent models. This is catalog data only; request parameters still flow
// through ModelOptions and ChatRequest.
func DefaultModelDescriptors() []ModelDescriptor {
	return cloneModelDescriptors(defaultModelDescriptors)
}

var defaultTextInputOutput = []string{"text"}

var defaultAgentCapabilities = []ModelCapability{
	ModelCapabilityTextInput,
	ModelCapabilityTextOutput,
	ModelCapabilityStreaming,
	ModelCapabilityToolCalling,
}

var defaultStructuredAgentCapabilities = []ModelCapability{
	ModelCapabilityTextInput,
	ModelCapabilityTextOutput,
	ModelCapabilityStreaming,
	ModelCapabilityToolCalling,
	ModelCapabilityStructuredOutput,
}

var defaultReasoningAgentCapabilities = []ModelCapability{
	ModelCapabilityTextInput,
	ModelCapabilityTextOutput,
	ModelCapabilityStreaming,
	ModelCapabilityToolCalling,
	ModelCapabilityStructuredOutput,
	ModelCapabilityReasoning,
}

var defaultThinkingAgentCapabilities = []ModelCapability{
	ModelCapabilityTextInput,
	ModelCapabilityTextOutput,
	ModelCapabilityStreaming,
	ModelCapabilityToolCalling,
	ModelCapabilityStructuredOutput,
	ModelCapabilityThinking,
}

var defaultModelDescriptors = []ModelDescriptor{
	{
		Provider:            "openai",
		ID:                  "gpt-5.4",
		Aliases:             []string{"gpt-5", "gpt-5.4-latest"},
		Family:              "gpt-5",
		DisplayName:         "OpenAI GPT-5.4",
		Stage:               ModelStageStable,
		Default:             true,
		ContextWindowTokens: 400000,
		MaxOutputTokens:     128000,
		InputModalities:     defaultTextInputOutput,
		OutputModalities:    defaultTextInputOutput,
		Capabilities:        append(defaultReasoningAgentCapabilities, ModelCapabilityPromptCaching, ModelCapabilityWebSearch),
		EndpointFamilies:    []ModelEndpointFamily{ModelEndpointOpenAIResponses, ModelEndpointOpenAIChat},
		VerifiedAt:          DefaultModelCatalogVerifiedAt,
		SourceURLs:          []string{modelCatalogSourceOpenAIResponses, modelCatalogSourceOpenAIModels},
		Metadata: map[string]string{
			"length_field":       "max_output_tokens",
			"reasoning_field":    "reasoning.effort",
			"verbosity_field":    "text.verbosity",
			"conversation_field": "conversation",
		},
	},
	{
		Provider:            "anthropic",
		ID:                  "claude-opus-4-7",
		Aliases:             []string{"claude-opus-latest"},
		Family:              "claude-4",
		DisplayName:         "Anthropic Claude Opus 4.7",
		Stage:               ModelStageStable,
		Default:             true,
		ContextWindowTokens: 200000,
		MaxOutputTokens:     32000,
		InputModalities:     defaultTextInputOutput,
		OutputModalities:    defaultTextInputOutput,
		Capabilities:        append(defaultThinkingAgentCapabilities, ModelCapabilityPromptCaching),
		EndpointFamilies:    []ModelEndpointFamily{ModelEndpointAnthropicMessage},
		VerifiedAt:          DefaultModelCatalogVerifiedAt,
		SourceURLs:          []string{modelCatalogSourceAnthropicModels, modelCatalogSourceAnthropicAPI},
		Metadata: map[string]string{
			"length_field":     "max_tokens",
			"thinking_field":   "thinking",
			"tool_choice":      "native",
			"provider_alias":   "claude",
			"cache_control":    "ephemeral",
			"round_trip_state": "thinking_blocks",
		},
	},
	{
		Provider:            "gemini",
		ID:                  "gemini-2.5-pro",
		Aliases:             []string{"gemini-pro"},
		Family:              "gemini-2.5",
		DisplayName:         "Google Gemini 2.5 Pro",
		Stage:               ModelStageStable,
		Default:             true,
		ContextWindowTokens: 1000000,
		MaxOutputTokens:     65536,
		InputModalities:     []string{"text", "image", "audio", "video"},
		OutputModalities:    defaultTextInputOutput,
		Capabilities:        append(defaultThinkingAgentCapabilities, ModelCapabilityMultimodalOutput),
		EndpointFamilies:    []ModelEndpointFamily{ModelEndpointGeminiGenerate, ModelEndpointVertexGenerate},
		VerifiedAt:          DefaultModelCatalogVerifiedAt,
		SourceURLs:          []string{modelCatalogSourceGeminiModels, modelCatalogSourceGeminiThinking},
		Metadata: map[string]string{
			"length_field":         "generationConfig.maxOutputTokens",
			"thinking_budget":      "thinkingConfig.thinkingBudget",
			"thinking_level":       "thinkingConfig.thinkingLevel",
			"structured_output":    "responseMimeType/responseSchema",
			"cached_content_field": "cachedContent",
		},
	},
	{
		Provider:         "deepseek",
		ID:               "deepseek-reasoner",
		Aliases:          []string{"deepseek-thinking"},
		Family:           "deepseek-reasoner",
		DisplayName:      "DeepSeek Reasoner",
		Stage:            ModelStageStable,
		Default:          true,
		InputModalities:  defaultTextInputOutput,
		OutputModalities: defaultTextInputOutput,
		Capabilities:     defaultReasoningAgentCapabilities,
		EndpointFamilies: []ModelEndpointFamily{ModelEndpointOpenAIChat},
		VerifiedAt:       DefaultModelCatalogVerifiedAt,
		SourceURLs:       []string{modelCatalogSourceDeepSeekChat, modelCatalogSourceDeepSeekReasoner},
		Metadata: map[string]string{
			"compat_provider": "openaicompat",
			"length_field":    "max_tokens",
			"reasoning_alias": "deepseek-reasoner",
		},
	},
	{
		Provider:         "qwen",
		ID:               "qwen3-max-2026-01-23",
		Aliases:          []string{"qwen3-max"},
		Family:           "qwen3",
		DisplayName:      "通义千问 Qwen3-Max",
		Stage:            ModelStageStable,
		Default:          true,
		InputModalities:  defaultTextInputOutput,
		OutputModalities: defaultTextInputOutput,
		Capabilities:     defaultThinkingAgentCapabilities,
		EndpointFamilies: []ModelEndpointFamily{ModelEndpointOpenAIChat},
		VerifiedAt:       DefaultModelCatalogVerifiedAt,
		SourceURLs:       []string{modelCatalogSourceQwenModels, modelCatalogSourceQwenAPI},
		Metadata: map[string]string{
			"compat_provider":       "openaicompat",
			"thinking_toggle_field": "enable_thinking",
			"thinking_budget_field": "thinking_budget",
			"incremental_output":    "true",
		},
	},
	{
		Provider:         "grok",
		ID:               "grok-4.20",
		Family:           "grok-4",
		DisplayName:      "xAI Grok 4.20",
		Stage:            ModelStageStable,
		Default:          true,
		InputModalities:  defaultTextInputOutput,
		OutputModalities: defaultTextInputOutput,
		Capabilities:     defaultReasoningAgentCapabilities,
		EndpointFamilies: []ModelEndpointFamily{ModelEndpointOpenAIChat},
		VerifiedAt:       DefaultModelCatalogVerifiedAt,
		SourceURLs:       []string{modelCatalogSourceXAIModels, modelCatalogSourceXAIReasoning},
		Metadata: map[string]string{
			"compat_provider": "openaicompat",
			"length_field":    "max_tokens",
			"reasoning_note":  "reasoning_effort semantics differ for multi-agent models",
		},
	},
	{
		Provider:         "mistral",
		ID:               "mistral-medium-latest",
		Family:           "mistral-medium",
		DisplayName:      "Mistral Medium",
		Stage:            ModelStageStable,
		Default:          true,
		InputModalities:  defaultTextInputOutput,
		OutputModalities: defaultTextInputOutput,
		Capabilities:     defaultStructuredAgentCapabilities,
		EndpointFamilies: []ModelEndpointFamily{ModelEndpointOpenAIChat},
		VerifiedAt:       DefaultModelCatalogVerifiedAt,
		SourceURLs:       []string{modelCatalogSourceMistralModels, modelCatalogSourceMistralAPI},
		Metadata: map[string]string{
			"compat_provider":  "openaicompat",
			"reasoning_family": "magistral-medium-latest",
		},
	},
	{
		Provider:         "kimi",
		ID:               "kimi-k2.5",
		Family:           "kimi-k2",
		DisplayName:      "Kimi K2.5",
		Stage:            ModelStageStable,
		Default:          true,
		InputModalities:  defaultTextInputOutput,
		OutputModalities: defaultTextInputOutput,
		Capabilities:     defaultThinkingAgentCapabilities,
		EndpointFamilies: []ModelEndpointFamily{ModelEndpointOpenAIChat},
		VerifiedAt:       DefaultModelCatalogVerifiedAt,
		SourceURLs:       []string{modelCatalogSourceMoonshotAPI},
		Metadata: map[string]string{
			"compat_provider": "openaicompat",
			"thinking_field":  "thinking",
			"tool_choice":     "auto_only_when_thinking",
		},
	},
}
