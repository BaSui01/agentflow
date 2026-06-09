package types

//go:generate python ../scripts/generate_execution_options_clone.py

import (
	"reflect"
	"strings"
	"time"
)

// ToolChoiceMode describes the runtime intent for selecting tools.
type ToolChoiceMode string

const (
	ToolChoiceModeAuto     ToolChoiceMode = "auto"
	ToolChoiceModeNone     ToolChoiceMode = "none"
	ToolChoiceModeRequired ToolChoiceMode = "required"
	ToolChoiceModeSpecific ToolChoiceMode = "specific"
	ToolChoiceModeAllowed  ToolChoiceMode = "allowed"
)

// ToolChoice captures tool selection intent in a provider-agnostic form.
type ToolChoice struct {
	Mode                             ToolChoiceMode `json:"mode"`
	ToolName                         string         `json:"tool_name,omitempty"`
	AllowedTools                     []string       `json:"allowed_tools,omitempty"`
	DisableParallelToolUse           *bool          `json:"disable_parallel_tool_use,omitempty"`
	IncludeServerSideToolInvocations *bool          `json:"include_server_side_tool_invocations,omitempty"`
}

// SafetySetting captures provider-neutral safety filter settings.
type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// OutputSpeechOptions captures provider-neutral speech output controls.
type OutputSpeechOptions struct {
	VoiceName    string `json:"voice_name,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// OutputImageOptions captures provider-neutral generated-image output controls.
type OutputImageOptions struct {
	AspectRatio        string `json:"aspect_ratio,omitempty"`
	ImageSize          string `json:"image_size,omitempty"`
	PersonGeneration   string `json:"person_generation,omitempty"`
	OutputMIMEType     string `json:"output_mime_type,omitempty"`
	CompressionQuality *int32 `json:"compression_quality,omitempty"`
}

// MemoryExternalContextPolicy controls memory behavior when external context
// sources like MCP or web search are involved in the turn.
type MemoryExternalContextPolicy struct {
	DisableAllOnExternalContext    bool `json:"disable_all_on_external_context,omitempty"`
	DisableRecallOnExternalContext bool `json:"disable_recall_on_external_context,omitempty"`
	DisableWriteOnExternalContext  bool `json:"disable_write_on_external_context,omitempty"`
}

// SubagentExecutionPolicy captures provider-neutral limits for handoffs and
// multi-agent fan-out.
type SubagentExecutionPolicy struct {
	AllowHandoffs  *bool `json:"allow_handoffs,omitempty"`
	MaxDepth       int   `json:"max_depth,omitempty"`
	MaxParallelism int   `json:"max_parallelism,omitempty"`
}

// ModelOptions contains provider request parameters that shape model behavior.
type ModelOptions struct {
	Provider             string               `json:"provider,omitempty"`
	Model                string               `json:"model"`
	RoutePolicy          string               `json:"route_policy,omitempty"`
	MaxTokens            int                  `json:"max_tokens,omitempty"`
	MaxCompletionTokens  *int                 `json:"max_completion_tokens,omitempty"`
	Temperature          float32              `json:"temperature,omitempty"`
	TopP                 float32              `json:"top_p,omitempty"`
	Stop                 []string             `json:"stop,omitempty"`
	FrequencyPenalty     *float32             `json:"frequency_penalty,omitempty"`
	PresencePenalty      *float32             `json:"presence_penalty,omitempty"`
	RepetitionPenalty    *float32             `json:"repetition_penalty,omitempty"`
	N                    *int                 `json:"n,omitempty"`
	LogProbs             *bool                `json:"logprobs,omitempty"`
	TopLogProbs          *int                 `json:"top_logprobs,omitempty"`
	User                 string               `json:"user,omitempty"`
	ResponseFormat       *ResponseFormat      `json:"response_format,omitempty"`
	StreamOptions        *StreamOptions       `json:"stream_options,omitempty"`
	ServiceTier          *string              `json:"service_tier,omitempty"`
	ReasoningEffort      string               `json:"reasoning_effort,omitempty"`
	ReasoningSummary     string               `json:"reasoning_summary,omitempty"`
	ReasoningDisplay     string               `json:"reasoning_display,omitempty"`
	ReasoningMode        string               `json:"reasoning_mode,omitempty"`
	ThinkingType         string               `json:"thinking_type,omitempty"`
	ThinkingLevel        string               `json:"thinking_level,omitempty"`
	ThinkingBudget       *int32               `json:"thinking_budget,omitempty"`
	IncludeThoughts      *bool                `json:"include_thoughts,omitempty"`
	MediaResolution      string               `json:"media_resolution,omitempty"`
	SafetySettings       []SafetySetting      `json:"safety_settings,omitempty"`
	OutputSpeech         *OutputSpeechOptions `json:"output_speech,omitempty"`
	OutputImage          *OutputImageOptions  `json:"output_image,omitempty"`
	InferenceSpeed       string               `json:"inference_speed,omitempty"`
	Store                *bool                `json:"store,omitempty"`
	Modalities           []string             `json:"modalities,omitempty"`
	PromptCacheKey       string               `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string               `json:"prompt_cache_retention,omitempty"`
	CacheControl         *CacheControl        `json:"cache_control,omitempty"`
	CachedContent        string               `json:"cached_content,omitempty"`
	Include              []string             `json:"include,omitempty"`
	Truncation           string               `json:"truncation,omitempty"`
	PreviousResponseID   string               `json:"previous_response_id,omitempty"`
	ConversationID       string               `json:"conversation_id,omitempty"`
	ThoughtSignatures    []string             `json:"thought_signatures,omitempty"`
	Verbosity            string               `json:"verbosity,omitempty"`
	Phase                string               `json:"phase,omitempty"`
	WebSearchOptions     *WebSearchOptions    `json:"web_search_options,omitempty"`
}

// AgentControlOptions contains runtime loop, validation, and context controls.
type AgentControlOptions struct {
	SystemPrompt          string                       `json:"system_prompt,omitempty"`
	Timeout               time.Duration                `json:"timeout,omitempty"`
	MaxReActIterations    int                          `json:"max_react_iterations,omitempty"`
	MaxLoopIterations     int                          `json:"max_loop_iterations,omitempty"`
	MaxConcurrency        int                          `json:"max_concurrency,omitempty"`
	ApprovalPolicy        string                       `json:"approval_policy,omitempty"`
	SandboxMode           string                       `json:"sandbox_mode,omitempty"`
	DisablePlanner        bool                         `json:"disable_planner,omitempty"`
	Context               *ContextConfig               `json:"context,omitempty"`
	Reflection            *ReflectionConfig            `json:"reflection,omitempty"`
	Guardrails            *GuardrailsConfig            `json:"guardrails,omitempty"`
	Memory                *MemoryConfig                `json:"memory,omitempty"`
	MemoryExternalContext *MemoryExternalContextPolicy `json:"memory_external_context,omitempty"`
	ToolSelection         *ToolSelectionConfig         `json:"tool_selection,omitempty"`
	PromptEnhancer        *PromptEnhancerConfig        `json:"prompt_enhancer,omitempty"`
}

// ToolProtocolOptions contains tool exposure and invocation controls.
type ToolProtocolOptions struct {
	AllowedTools      []string                 `json:"allowed_tools,omitempty"`
	ToolWhitelist     []string                 `json:"tool_whitelist,omitempty"`
	DisableTools      bool                     `json:"disable_tools,omitempty"`
	Handoffs          []string                 `json:"handoffs,omitempty"`
	Subagents         *SubagentExecutionPolicy `json:"subagents,omitempty"`
	ToolModel         string                   `json:"tool_model,omitempty"`
	ToolChoice        *ToolChoice              `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool                    `json:"parallel_tool_calls,omitempty"`
	ToolCallMode      ToolCallMode             `json:"tool_call_mode,omitempty"`
}

func (o ToolProtocolOptions) SubagentsMaxDepth() int {
	if o.Subagents == nil {
		return 0
	}
	return o.Subagents.MaxDepth
}

func (o ToolProtocolOptions) SubagentsMaxParallelism() int {
	if o.Subagents == nil {
		return 0
	}
	return o.Subagents.MaxParallelism
}

// RunOverrides is the staged override surface for a single execution.
type RunOverrides struct {
	Model    *ModelOptions        `json:"model,omitempty"`
	Control  *AgentControlOptions `json:"control,omitempty"`
	Tools    *ToolProtocolOptions `json:"tools,omitempty"`
	Metadata map[string]string    `json:"metadata,omitempty"`
	Tags     []string             `json:"tags,omitempty"`
}

// ExecutionOptions is the runtime view consumed by the execution chain.
type ExecutionOptions struct {
	Core     CoreConfig          `json:"core"`
	Model    ModelOptions        `json:"model"`
	Control  AgentControlOptions `json:"control"`
	Tools    ToolProtocolOptions `json:"tools"`
	Metadata map[string]string   `json:"metadata,omitempty"`
	Tags     []string            `json:"tags,omitempty"`
}

// Clone returns a deep copy of the execution options.
func (o ExecutionOptions) Clone() ExecutionOptions {
	return ExecutionOptions{
		Core:     o.Core,
		Model:    o.Model.clone(),
		Control:  o.Control.clone(),
		Tools:    o.Tools.clone(),
		Metadata: cloneExecutionMetadata(o.Metadata),
		Tags:     cloneExecutionStrings(o.Tags),
	}
}

// ExecutionOptions returns the runtime execution view derived from AgentConfig.
func (c AgentConfig) ExecutionOptions() ExecutionOptions {
	options := ExecutionOptions{
		Core: c.Core,
		Model: ModelOptions{
			Provider:    c.LLM.Provider,
			Model:       c.LLM.Model,
			MaxTokens:   c.LLM.MaxTokens,
			Temperature: c.LLM.Temperature,
			TopP:        c.LLM.TopP,
			Stop:        cloneExecutionStrings(c.LLM.Stop),
		},
		Control: AgentControlOptions{
			SystemPrompt:          c.Runtime.SystemPrompt,
			MaxReActIterations:    c.Runtime.MaxReActIterations,
			MaxLoopIterations:     c.Runtime.MaxLoopIterations,
			ApprovalPolicy:        strings.TrimSpace(c.Runtime.ApprovalPolicy),
			SandboxMode:           strings.TrimSpace(c.Runtime.SandboxMode),
			Context:               cloneContextConfig(c.Context),
			Reflection:            cloneReflectionConfig(c.Features.Reflection),
			Guardrails:            cloneGuardrailsConfig(c.Features.Guardrails),
			Memory:                cloneMemoryConfig(c.Features.Memory),
			MemoryExternalContext: memoryConfigToExternalContextPolicy(c.Features.Memory),
			ToolSelection:         cloneToolSelectionConfig(c.Features.ToolSelection),
			PromptEnhancer:        clonePromptEnhancerConfig(c.Features.PromptEnhancer),
		},
		Tools: ToolProtocolOptions{
			AllowedTools: cloneExecutionStrings(c.Runtime.Tools),
			Handoffs:     cloneExecutionStrings(c.Runtime.Handoffs),
			Subagents:    runtimeConfigToSubagentPolicy(c.Runtime),
			ToolModel:    c.Runtime.ToolModel,
		},
		Metadata: cloneExecutionMetadata(c.Metadata),
	}
	if c.hasFormalMainFace() {
		options.Model = mergeModelOptions(options.Model, c.Model)
		options.Control = mergeAgentControlOptions(options.Control, c.Control)
		options.Tools = mergeToolProtocolOptions(options.Tools, c.Tools)
	}
	return options
}

// ParseToolChoiceString converts the existing string-based runtime setting into
// a provider-agnostic tool choice structure.
func ParseToolChoiceString(value string) *ToolChoice {
	switch trimmed := strings.TrimSpace(value); trimmed {
	case "":
		return nil
	case "auto":
		return &ToolChoice{Mode: ToolChoiceModeAuto}
	case "none":
		return &ToolChoice{Mode: ToolChoiceModeNone}
	case "required", "any":
		return &ToolChoice{Mode: ToolChoiceModeRequired}
	default:
		return &ToolChoice{Mode: ToolChoiceModeSpecific, ToolName: trimmed}
	}
}

func (o ModelOptions) clone() ModelOptions {
	return ModelOptions{
		Provider:             o.Provider,
		Model:                o.Model,
		RoutePolicy:          o.RoutePolicy,
		MaxTokens:            o.MaxTokens,
		MaxCompletionTokens:  cloneExecutionScalarPtr(o.MaxCompletionTokens),
		Temperature:          o.Temperature,
		TopP:                 o.TopP,
		Stop:                 cloneExecutionStrings(o.Stop),
		FrequencyPenalty:     cloneExecutionScalarPtr(o.FrequencyPenalty),
		PresencePenalty:      cloneExecutionScalarPtr(o.PresencePenalty),
		RepetitionPenalty:    cloneExecutionScalarPtr(o.RepetitionPenalty),
		N:                    cloneExecutionScalarPtr(o.N),
		LogProbs:             cloneExecutionScalarPtr(o.LogProbs),
		TopLogProbs:          cloneExecutionScalarPtr(o.TopLogProbs),
		User:                 o.User,
		ResponseFormat:       cloneResponseFormat(o.ResponseFormat),
		StreamOptions:        cloneStreamOptions(o.StreamOptions),
		ServiceTier:          cloneExecutionScalarPtr(o.ServiceTier),
		ReasoningEffort:      o.ReasoningEffort,
		ReasoningSummary:     o.ReasoningSummary,
		ReasoningDisplay:     o.ReasoningDisplay,
		ReasoningMode:        o.ReasoningMode,
		ThinkingType:         o.ThinkingType,
		ThinkingLevel:        o.ThinkingLevel,
		ThinkingBudget:       cloneExecutionScalarPtr(o.ThinkingBudget),
		IncludeThoughts:      cloneExecutionScalarPtr(o.IncludeThoughts),
		MediaResolution:      o.MediaResolution,
		SafetySettings:       cloneSafetySettings(o.SafetySettings),
		OutputSpeech:         cloneOutputSpeechOptions(o.OutputSpeech),
		OutputImage:          cloneOutputImageOptions(o.OutputImage),
		InferenceSpeed:       o.InferenceSpeed,
		Store:                cloneExecutionScalarPtr(o.Store),
		Modalities:           cloneExecutionStrings(o.Modalities),
		PromptCacheKey:       o.PromptCacheKey,
		PromptCacheRetention: o.PromptCacheRetention,
		CacheControl:         cloneCacheControl(o.CacheControl),
		CachedContent:        o.CachedContent,
		Include:              cloneExecutionStrings(o.Include),
		Truncation:           o.Truncation,
		PreviousResponseID:   o.PreviousResponseID,
		ConversationID:       o.ConversationID,
		ThoughtSignatures:    cloneExecutionStrings(o.ThoughtSignatures),
		Verbosity:            o.Verbosity,
		Phase:                o.Phase,
		WebSearchOptions:     cloneWebSearchOptions(o.WebSearchOptions),
	}
}

func (o AgentControlOptions) clone() AgentControlOptions {
	return AgentControlOptions{
		SystemPrompt:          o.SystemPrompt,
		Timeout:               o.Timeout,
		MaxReActIterations:    o.MaxReActIterations,
		MaxLoopIterations:     o.MaxLoopIterations,
		MaxConcurrency:        o.MaxConcurrency,
		ApprovalPolicy:        o.ApprovalPolicy,
		SandboxMode:           o.SandboxMode,
		DisablePlanner:        o.DisablePlanner,
		Context:               cloneContextConfig(o.Context),
		Reflection:            cloneReflectionConfig(o.Reflection),
		Guardrails:            cloneGuardrailsConfig(o.Guardrails),
		Memory:                cloneMemoryConfig(o.Memory),
		MemoryExternalContext: cloneMemoryExternalContextPolicy(o.MemoryExternalContext),
		ToolSelection:         cloneToolSelectionConfig(o.ToolSelection),
		PromptEnhancer:        clonePromptEnhancerConfig(o.PromptEnhancer),
	}
}

func (o ToolProtocolOptions) clone() ToolProtocolOptions {
	return ToolProtocolOptions{
		AllowedTools:      cloneExecutionStrings(o.AllowedTools),
		ToolWhitelist:     cloneExecutionStrings(o.ToolWhitelist),
		DisableTools:      o.DisableTools,
		Handoffs:          cloneExecutionStrings(o.Handoffs),
		Subagents:         cloneSubagentExecutionPolicy(o.Subagents),
		ToolModel:         o.ToolModel,
		ToolChoice:        cloneToolChoice(o.ToolChoice),
		ParallelToolCalls: cloneExecutionScalarPtr(o.ParallelToolCalls),
		ToolCallMode:      o.ToolCallMode,
	}
}

func (c AgentConfig) hasFormalMainFace() bool {
	return formalSurfaceHasValues(c.Model) ||
		formalSurfaceHasValues(c.Control) ||
		formalSurfaceHasValues(c.Tools)
}

func formalSurfaceHasValues(surface any) bool {
	return formalValueHasValue(reflect.ValueOf(surface))
}

func formalValueHasValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Pointer:
		return !value.IsNil()
	case reflect.String:
		return strings.TrimSpace(value.String()) != ""
	case reflect.Slice, reflect.Map:
		return value.Len() > 0
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return value.Float() != 0
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if formalValueHasValue(value.Field(i)) {
				return true
			}
		}
		return false
	default:
		return !value.IsZero()
	}
}

func mergeModelOptions(base ModelOptions, override ModelOptions) ModelOptions {
	out := base.clone()
	if strings.TrimSpace(override.Provider) != "" {
		out.Provider = strings.TrimSpace(override.Provider)
	}
	if strings.TrimSpace(override.Model) != "" {
		out.Model = strings.TrimSpace(override.Model)
	}
	if strings.TrimSpace(override.RoutePolicy) != "" {
		out.RoutePolicy = strings.TrimSpace(override.RoutePolicy)
	}
	if override.MaxTokens > 0 {
		out.MaxTokens = override.MaxTokens
	}
	if override.MaxCompletionTokens != nil {
		out.MaxCompletionTokens = cloneExecutionScalarPtr(override.MaxCompletionTokens)
	}
	if override.Temperature != 0 {
		out.Temperature = override.Temperature
	}
	if override.TopP != 0 {
		out.TopP = override.TopP
	}
	if len(override.Stop) > 0 {
		out.Stop = cloneExecutionStrings(override.Stop)
	}
	if override.FrequencyPenalty != nil {
		out.FrequencyPenalty = cloneExecutionScalarPtr(override.FrequencyPenalty)
	}
	if override.PresencePenalty != nil {
		out.PresencePenalty = cloneExecutionScalarPtr(override.PresencePenalty)
	}
	if override.RepetitionPenalty != nil {
		out.RepetitionPenalty = cloneExecutionScalarPtr(override.RepetitionPenalty)
	}
	if override.N != nil {
		out.N = cloneExecutionScalarPtr(override.N)
	}
	if override.LogProbs != nil {
		out.LogProbs = cloneExecutionScalarPtr(override.LogProbs)
	}
	if override.TopLogProbs != nil {
		out.TopLogProbs = cloneExecutionScalarPtr(override.TopLogProbs)
	}
	if strings.TrimSpace(override.User) != "" {
		out.User = strings.TrimSpace(override.User)
	}
	if override.ResponseFormat != nil {
		out.ResponseFormat = cloneResponseFormat(override.ResponseFormat)
	}
	if override.StreamOptions != nil {
		out.StreamOptions = cloneStreamOptions(override.StreamOptions)
	}
	if override.ServiceTier != nil {
		out.ServiceTier = cloneExecutionScalarPtr(override.ServiceTier)
	}
	if strings.TrimSpace(override.ReasoningEffort) != "" {
		out.ReasoningEffort = strings.TrimSpace(override.ReasoningEffort)
	}
	if strings.TrimSpace(override.ReasoningSummary) != "" {
		out.ReasoningSummary = strings.TrimSpace(override.ReasoningSummary)
	}
	if strings.TrimSpace(override.ReasoningDisplay) != "" {
		out.ReasoningDisplay = strings.TrimSpace(override.ReasoningDisplay)
	}
	if strings.TrimSpace(override.ReasoningMode) != "" {
		out.ReasoningMode = strings.TrimSpace(override.ReasoningMode)
	}
	if strings.TrimSpace(override.ThinkingType) != "" {
		out.ThinkingType = strings.TrimSpace(override.ThinkingType)
	}
	if strings.TrimSpace(override.ThinkingLevel) != "" {
		out.ThinkingLevel = strings.TrimSpace(override.ThinkingLevel)
	}
	if override.ThinkingBudget != nil {
		out.ThinkingBudget = cloneExecutionScalarPtr(override.ThinkingBudget)
	}
	if override.IncludeThoughts != nil {
		out.IncludeThoughts = cloneExecutionScalarPtr(override.IncludeThoughts)
	}
	if strings.TrimSpace(override.MediaResolution) != "" {
		out.MediaResolution = strings.TrimSpace(override.MediaResolution)
	}
	if len(override.SafetySettings) > 0 {
		out.SafetySettings = cloneSafetySettings(override.SafetySettings)
	}
	if override.OutputSpeech != nil {
		out.OutputSpeech = cloneOutputSpeechOptions(override.OutputSpeech)
	}
	if override.OutputImage != nil {
		out.OutputImage = cloneOutputImageOptions(override.OutputImage)
	}
	if strings.TrimSpace(override.InferenceSpeed) != "" {
		out.InferenceSpeed = strings.TrimSpace(override.InferenceSpeed)
	}
	if override.Store != nil {
		out.Store = cloneExecutionScalarPtr(override.Store)
	}
	if len(override.Modalities) > 0 {
		out.Modalities = cloneExecutionStrings(override.Modalities)
	}
	if strings.TrimSpace(override.PromptCacheKey) != "" {
		out.PromptCacheKey = strings.TrimSpace(override.PromptCacheKey)
	}
	if strings.TrimSpace(override.PromptCacheRetention) != "" {
		out.PromptCacheRetention = strings.TrimSpace(override.PromptCacheRetention)
	}
	if override.CacheControl != nil {
		out.CacheControl = cloneCacheControl(override.CacheControl)
	}
	if strings.TrimSpace(override.CachedContent) != "" {
		out.CachedContent = strings.TrimSpace(override.CachedContent)
	}
	if len(override.Include) > 0 {
		out.Include = cloneExecutionStrings(override.Include)
	}
	if strings.TrimSpace(override.Truncation) != "" {
		out.Truncation = strings.TrimSpace(override.Truncation)
	}
	if strings.TrimSpace(override.PreviousResponseID) != "" {
		out.PreviousResponseID = strings.TrimSpace(override.PreviousResponseID)
	}
	if strings.TrimSpace(override.ConversationID) != "" {
		out.ConversationID = strings.TrimSpace(override.ConversationID)
	}
	if len(override.ThoughtSignatures) > 0 {
		out.ThoughtSignatures = cloneExecutionStrings(override.ThoughtSignatures)
	}
	if strings.TrimSpace(override.Verbosity) != "" {
		out.Verbosity = strings.TrimSpace(override.Verbosity)
	}
	if strings.TrimSpace(override.Phase) != "" {
		out.Phase = strings.TrimSpace(override.Phase)
	}
	if override.WebSearchOptions != nil {
		out.WebSearchOptions = cloneWebSearchOptions(override.WebSearchOptions)
	}
	return out
}

func mergeAgentControlOptions(base AgentControlOptions, override AgentControlOptions) AgentControlOptions {
	out := base.clone()
	if strings.TrimSpace(override.SystemPrompt) != "" {
		out.SystemPrompt = strings.TrimSpace(override.SystemPrompt)
	}
	if override.Timeout != 0 {
		out.Timeout = override.Timeout
	}
	if override.MaxReActIterations > 0 {
		out.MaxReActIterations = override.MaxReActIterations
	}
	if override.MaxLoopIterations > 0 {
		out.MaxLoopIterations = override.MaxLoopIterations
	}
	if override.MaxConcurrency > 0 {
		out.MaxConcurrency = override.MaxConcurrency
	}
	if strings.TrimSpace(override.ApprovalPolicy) != "" {
		out.ApprovalPolicy = strings.TrimSpace(override.ApprovalPolicy)
	}
	if strings.TrimSpace(override.SandboxMode) != "" {
		out.SandboxMode = strings.TrimSpace(override.SandboxMode)
	}
	if override.DisablePlanner {
		out.DisablePlanner = true
	}
	if override.Context != nil {
		out.Context = cloneContextConfig(override.Context)
	}
	if override.Reflection != nil {
		out.Reflection = cloneReflectionConfig(override.Reflection)
	}
	if override.Guardrails != nil {
		out.Guardrails = cloneGuardrailsConfig(override.Guardrails)
	}
	if override.Memory != nil {
		out.Memory = cloneMemoryConfig(override.Memory)
	}
	if override.MemoryExternalContext != nil {
		out.MemoryExternalContext = cloneMemoryExternalContextPolicy(override.MemoryExternalContext)
	}
	if override.ToolSelection != nil {
		out.ToolSelection = cloneToolSelectionConfig(override.ToolSelection)
	}
	if override.PromptEnhancer != nil {
		out.PromptEnhancer = clonePromptEnhancerConfig(override.PromptEnhancer)
	}
	return out
}

func mergeToolProtocolOptions(base ToolProtocolOptions, override ToolProtocolOptions) ToolProtocolOptions {
	out := base.clone()
	if len(override.AllowedTools) > 0 {
		out.AllowedTools = cloneExecutionStrings(override.AllowedTools)
	}
	if len(override.ToolWhitelist) > 0 {
		out.ToolWhitelist = cloneExecutionStrings(override.ToolWhitelist)
	}
	if override.DisableTools {
		out.DisableTools = true
		out.ToolWhitelist = nil
	}
	if len(override.Handoffs) > 0 {
		out.Handoffs = cloneExecutionStrings(override.Handoffs)
	}
	if override.Subagents != nil {
		out.Subagents = cloneSubagentExecutionPolicy(override.Subagents)
	}
	if strings.TrimSpace(override.ToolModel) != "" {
		out.ToolModel = strings.TrimSpace(override.ToolModel)
	}
	if override.ToolChoice != nil {
		out.ToolChoice = cloneToolChoice(override.ToolChoice)
	}
	if override.ParallelToolCalls != nil {
		out.ParallelToolCalls = cloneExecutionScalarPtr(override.ParallelToolCalls)
	}
	if override.ToolCallMode != "" {
		out.ToolCallMode = override.ToolCallMode
	}
	return out
}

func memoryConfigToExternalContextPolicy(value *MemoryConfig) *MemoryExternalContextPolicy {
	if value == nil {
		return nil
	}
	if !value.DisableOnExternalContext && !value.DisableRecallOnExternalContext && !value.DisableWriteOnExternalContext {
		return nil
	}
	return &MemoryExternalContextPolicy{
		DisableAllOnExternalContext:    value.DisableOnExternalContext,
		DisableRecallOnExternalContext: value.DisableRecallOnExternalContext,
		DisableWriteOnExternalContext:  value.DisableWriteOnExternalContext,
	}
}

func runtimeConfigToSubagentPolicy(value RuntimeConfig) *SubagentExecutionPolicy {
	if len(value.Handoffs) == 0 {
		return nil
	}
	allow := true
	return &SubagentExecutionPolicy{AllowHandoffs: &allow}
}

func cloneToolSelectionConfig(value *ToolSelectionConfig) *ToolSelectionConfig {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func clonePromptEnhancerConfig(value *PromptEnhancerConfig) *PromptEnhancerConfig {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSafetySettings(values []SafetySetting) []SafetySetting {
	if len(values) == 0 {
		return nil
	}
	return append([]SafetySetting(nil), values...)
}

func cloneOutputSpeechOptions(value *OutputSpeechOptions) *OutputSpeechOptions {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneOutputImageOptions(value *OutputImageOptions) *OutputImageOptions {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.CompressionQuality = cloneExecutionScalarPtr(value.CompressionQuality)
	return &cloned
}
