package types

import (
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

// ModelOptions contains provider request parameters that shape model behavior.
type ModelOptions struct {
	Provider            string            `json:"provider,omitempty"`
	Model               string            `json:"model"`
	RoutePolicy         string            `json:"route_policy,omitempty"`
	MaxTokens           int               `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	Temperature         float32           `json:"temperature,omitempty"`
	TopP                float32           `json:"top_p,omitempty"`
	Stop                []string          `json:"stop,omitempty"`
	ResponseFormat      *ResponseFormat   `json:"response_format,omitempty"`
	ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
	ReasoningSummary    string            `json:"reasoning_summary,omitempty"`
	ReasoningDisplay    string            `json:"reasoning_display,omitempty"`
	InferenceSpeed      string            `json:"inference_speed,omitempty"`
	WebSearchOptions    *WebSearchOptions `json:"web_search_options,omitempty"`
}

// AgentControlOptions contains runtime loop, validation, and context controls.
type AgentControlOptions struct {
	SystemPrompt       string                `json:"system_prompt,omitempty"`
	Timeout            time.Duration         `json:"timeout,omitempty"`
	MaxReActIterations int                   `json:"max_react_iterations,omitempty"`
	MaxLoopIterations  int                   `json:"max_loop_iterations,omitempty"`
	MaxConcurrency     int                   `json:"max_concurrency,omitempty"`
	DisablePlanner     bool                  `json:"disable_planner,omitempty"`
	Context            *ContextConfig        `json:"context,omitempty"`
	Reflection         *ReflectionConfig     `json:"reflection,omitempty"`
	Guardrails         *GuardrailsConfig     `json:"guardrails,omitempty"`
	Memory             *MemoryConfig         `json:"memory,omitempty"`
	ToolSelection      *ToolSelectionConfig  `json:"tool_selection,omitempty"`
	PromptEnhancer     *PromptEnhancerConfig `json:"prompt_enhancer,omitempty"`
}

// ToolProtocolOptions contains tool exposure and invocation controls.
type ToolProtocolOptions struct {
	AllowedTools      []string     `json:"allowed_tools,omitempty"`
	ToolWhitelist     []string     `json:"tool_whitelist,omitempty"`
	DisableTools      bool         `json:"disable_tools,omitempty"`
	Handoffs          []string     `json:"handoffs,omitempty"`
	ToolModel         string       `json:"tool_model,omitempty"`
	ToolChoice        *ToolChoice  `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool        `json:"parallel_tool_calls,omitempty"`
	ToolCallMode      ToolCallMode `json:"tool_call_mode,omitempty"`
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
			SystemPrompt:       c.Runtime.SystemPrompt,
			MaxReActIterations: c.Runtime.MaxReActIterations,
			MaxLoopIterations:  c.Runtime.MaxLoopIterations,
			Context:            cloneContextConfig(c.Context),
			Reflection:         cloneReflectionConfig(c.Features.Reflection),
			Guardrails:         cloneGuardrailsConfig(c.Features.Guardrails),
			Memory:             cloneMemoryConfig(c.Features.Memory),
			ToolSelection:      cloneToolSelectionConfig(c.Features.ToolSelection),
			PromptEnhancer:     clonePromptEnhancerConfig(c.Features.PromptEnhancer),
		},
		Tools: ToolProtocolOptions{
			AllowedTools: cloneExecutionStrings(c.Runtime.Tools),
			Handoffs:     cloneExecutionStrings(c.Runtime.Handoffs),
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
		Provider:            o.Provider,
		Model:               o.Model,
		RoutePolicy:         o.RoutePolicy,
		MaxTokens:           o.MaxTokens,
		MaxCompletionTokens: cloneExecutionIntPtr(o.MaxCompletionTokens),
		Temperature:         o.Temperature,
		TopP:                o.TopP,
		Stop:                cloneExecutionStrings(o.Stop),
		ResponseFormat:      cloneResponseFormat(o.ResponseFormat),
		ReasoningEffort:     o.ReasoningEffort,
		ReasoningSummary:    o.ReasoningSummary,
		ReasoningDisplay:    o.ReasoningDisplay,
		InferenceSpeed:      o.InferenceSpeed,
		WebSearchOptions:    cloneWebSearchOptions(o.WebSearchOptions),
	}
}

func (o AgentControlOptions) clone() AgentControlOptions {
	return AgentControlOptions{
		SystemPrompt:       o.SystemPrompt,
		Timeout:            o.Timeout,
		MaxReActIterations: o.MaxReActIterations,
		MaxLoopIterations:  o.MaxLoopIterations,
		MaxConcurrency:     o.MaxConcurrency,
		DisablePlanner:     o.DisablePlanner,
		Context:            cloneContextConfig(o.Context),
		Reflection:         cloneReflectionConfig(o.Reflection),
		Guardrails:         cloneGuardrailsConfig(o.Guardrails),
		Memory:             cloneMemoryConfig(o.Memory),
		ToolSelection:      cloneToolSelectionConfig(o.ToolSelection),
		PromptEnhancer:     clonePromptEnhancerConfig(o.PromptEnhancer),
	}
}

func (o ToolProtocolOptions) clone() ToolProtocolOptions {
	return ToolProtocolOptions{
		AllowedTools:      cloneExecutionStrings(o.AllowedTools),
		ToolWhitelist:     cloneExecutionStrings(o.ToolWhitelist),
		DisableTools:      o.DisableTools,
		Handoffs:          cloneExecutionStrings(o.Handoffs),
		ToolModel:         o.ToolModel,
		ToolChoice:        cloneToolChoice(o.ToolChoice),
		ParallelToolCalls: cloneExecutionBoolPtr(o.ParallelToolCalls),
		ToolCallMode:      o.ToolCallMode,
	}
}

func (c AgentConfig) hasFormalMainFace() bool {
	return strings.TrimSpace(c.Model.Model) != "" ||
		strings.TrimSpace(c.Model.Provider) != "" ||
		strings.TrimSpace(c.Control.SystemPrompt) != "" ||
		c.Control.Timeout != 0 ||
		c.Control.MaxReActIterations != 0 ||
		c.Control.MaxLoopIterations != 0 ||
		c.Control.MaxConcurrency != 0 ||
		c.Control.DisablePlanner ||
		c.Control.Context != nil ||
		c.Control.Reflection != nil ||
		c.Control.Guardrails != nil ||
		c.Control.Memory != nil ||
		c.Control.ToolSelection != nil ||
		c.Control.PromptEnhancer != nil ||
		len(c.Tools.AllowedTools) > 0 ||
		len(c.Tools.ToolWhitelist) > 0 ||
		c.Tools.DisableTools ||
		len(c.Tools.Handoffs) > 0 ||
		strings.TrimSpace(c.Tools.ToolModel) != "" ||
		c.Tools.ToolChoice != nil ||
		c.Tools.ParallelToolCalls != nil ||
		c.Tools.ToolCallMode != ""
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
		out.MaxCompletionTokens = cloneExecutionIntPtr(override.MaxCompletionTokens)
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
	if override.ResponseFormat != nil {
		out.ResponseFormat = cloneResponseFormat(override.ResponseFormat)
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
	if strings.TrimSpace(override.InferenceSpeed) != "" {
		out.InferenceSpeed = strings.TrimSpace(override.InferenceSpeed)
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
	if strings.TrimSpace(override.ToolModel) != "" {
		out.ToolModel = strings.TrimSpace(override.ToolModel)
	}
	if override.ToolChoice != nil {
		out.ToolChoice = cloneToolChoice(override.ToolChoice)
	}
	if override.ParallelToolCalls != nil {
		out.ParallelToolCalls = cloneExecutionBoolPtr(override.ParallelToolCalls)
	}
	if override.ToolCallMode != "" {
		out.ToolCallMode = override.ToolCallMode
	}
	return out
}

func cloneExecutionStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneExecutionMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneExecutionIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneExecutionBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneToolChoice(choice *ToolChoice) *ToolChoice {
	if choice == nil {
		return nil
	}
	cloned := *choice
	cloned.AllowedTools = cloneExecutionStrings(choice.AllowedTools)
	cloned.DisableParallelToolUse = cloneExecutionBoolPtr(choice.DisableParallelToolUse)
	cloned.IncludeServerSideToolInvocations = cloneExecutionBoolPtr(choice.IncludeServerSideToolInvocations)
	return &cloned
}

func cloneResponseFormat(value *ResponseFormat) *ResponseFormat {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.JSONSchema != nil {
		schema := *value.JSONSchema
		if len(value.JSONSchema.Schema) > 0 {
			schema.Schema = cloneJSONSchemaMap(value.JSONSchema.Schema)
		}
		if value.JSONSchema.Strict != nil {
			strict := *value.JSONSchema.Strict
			schema.Strict = &strict
		}
		cloned.JSONSchema = &schema
	}
	return &cloned
}

func cloneJSONSchemaMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneWebSearchOptions(value *WebSearchOptions) *WebSearchOptions {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.AllowedDomains = cloneExecutionStrings(value.AllowedDomains)
	cloned.BlockedDomains = cloneExecutionStrings(value.BlockedDomains)
	if value.UserLocation != nil {
		location := *value.UserLocation
		cloned.UserLocation = &location
	}
	return &cloned
}

func cloneContextConfig(value *ContextConfig) *ContextConfig {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneReflectionConfig(value *ReflectionConfig) *ReflectionConfig {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneGuardrailsConfig(value *GuardrailsConfig) *GuardrailsConfig {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.BlockedKeywords = cloneExecutionStrings(value.BlockedKeywords)
	return &cloned
}

func cloneMemoryConfig(value *MemoryConfig) *MemoryConfig {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
