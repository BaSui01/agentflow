package agent

import (
	"strconv"
	"strings"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// ChatRequestAdapter translates provider-agnostic execution options into the
// LLM contract DTO consumed by gateways/providers.
type ChatRequestAdapter interface {
	Build(options types.ExecutionOptions, messages []types.Message) (*types.ChatRequest, error)
}

// DefaultChatRequestAdapter is the runtime's canonical chat request adapter.
type DefaultChatRequestAdapter struct{}

func NewDefaultChatRequestAdapter() ChatRequestAdapter {
	return DefaultChatRequestAdapter{}
}

func (DefaultChatRequestAdapter) Build(options types.ExecutionOptions, messages []types.Message) (*types.ChatRequest, error) {
	if len(messages) == 0 {
		return nil, NewError(types.ErrInputValidation, "messages cannot be nil or empty")
	}

	req := &types.ChatRequest{
		Model:               options.Model.Model,
		Messages:            append([]types.Message(nil), messages...),
		MaxTokens:           options.Model.MaxTokens,
		Temperature:         options.Model.Temperature,
		TopP:                options.Model.TopP,
		Stop:                append([]string(nil), options.Model.Stop...),
		ResponseFormat:      cloneResponseFormatForAdapter(options.Model.ResponseFormat),
		Timeout:             options.Control.Timeout,
		Metadata:            cloneAdapterMetadata(options.Metadata),
		Tags:                append([]string(nil), options.Tags...),
		MaxCompletionTokens: cloneAdapterIntPtr(options.Model.MaxCompletionTokens),
		ReasoningEffort:     options.Model.ReasoningEffort,
		ReasoningSummary:    options.Model.ReasoningSummary,
		ReasoningDisplay:    options.Model.ReasoningDisplay,
		InferenceSpeed:      options.Model.InferenceSpeed,
		WebSearchOptions:    cloneWebSearchOptionsForAdapter(options.Model.WebSearchOptions),
	}

	if strings.TrimSpace(options.Model.Provider) != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, 2)
		}
		req.Metadata[llmcore.MetadataKeyChatProvider] = strings.TrimSpace(options.Model.Provider)
	}
	if strings.TrimSpace(options.Model.RoutePolicy) != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, 2)
		}
		req.Metadata["route_policy"] = strings.TrimSpace(options.Model.RoutePolicy)
	}
	if options.Control.MaxLoopIterations > 0 {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, 1)
		}
		req.Metadata["max_loop_iterations"] = strconv.Itoa(options.Control.MaxLoopIterations)
	}
	if options.Tools.ToolChoice != nil {
		req.ToolChoice = toolChoiceToRequestValue(options.Tools.ToolChoice)
	}
	if options.Tools.ParallelToolCalls != nil {
		req.ParallelToolCalls = cloneAdapterBoolPtr(options.Tools.ParallelToolCalls)
	}
	if options.Tools.ToolCallMode != "" {
		req.ToolCallMode = options.Tools.ToolCallMode
	}

	return req, nil
}

func toolChoiceToRequestValue(choice *types.ToolChoice) any {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case types.ToolChoiceModeAuto:
		return "auto"
	case types.ToolChoiceModeNone:
		return "none"
	case types.ToolChoiceModeRequired:
		return "required"
	case types.ToolChoiceModeSpecific:
		return strings.TrimSpace(choice.ToolName)
	case types.ToolChoiceModeAllowed:
		payload := map[string]any{
			"type": "allowed",
		}
		if len(choice.AllowedTools) > 0 {
			payload["allowed_function_names"] = append([]string(nil), choice.AllowedTools...)
		}
		if choice.DisableParallelToolUse != nil {
			payload["disable_parallel_tool_use"] = *choice.DisableParallelToolUse
		}
		if choice.IncludeServerSideToolInvocations != nil {
			payload["include_server_side_tool_invocations"] = *choice.IncludeServerSideToolInvocations
		}
		return payload
	default:
		return nil
	}
}

func cloneAdapterMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneAdapterIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneAdapterBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneResponseFormatForAdapter(value *types.ResponseFormat) *types.ResponseFormat {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.JSONSchema != nil {
		schema := *value.JSONSchema
		if len(value.JSONSchema.Schema) > 0 {
			schema.Schema = make(map[string]any, len(value.JSONSchema.Schema))
			for key, item := range value.JSONSchema.Schema {
				schema.Schema[key] = item
			}
		}
		if value.JSONSchema.Strict != nil {
			strict := *value.JSONSchema.Strict
			schema.Strict = &strict
		}
		cloned.JSONSchema = &schema
	}
	return &cloned
}

func cloneWebSearchOptionsForAdapter(value *types.WebSearchOptions) *types.WebSearchOptions {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.AllowedDomains = append([]string(nil), value.AllowedDomains...)
	cloned.BlockedDomains = append([]string(nil), value.BlockedDomains...)
	if value.UserLocation != nil {
		location := *value.UserLocation
		cloned.UserLocation = &location
	}
	return &cloned
}
