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
	return ExecutionOptions{
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
