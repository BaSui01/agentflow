package core

import (
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
		RoutePolicy:         strings.TrimSpace(options.Model.RoutePolicy),
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
	if toolChoiceValue := toolChoiceToRequestValue(options.Tools.ToolChoice); toolChoiceValue != nil {
		typedChoice, ok := toolChoiceValue.(*types.ToolChoice)
		if !ok {
			return nil, NewError(types.ErrInputValidation, "tool choice adapter produced unsupported request payload")
		}
		req.ToolChoice = typedChoice
	}
	if options.Tools.ParallelToolCalls != nil {
		req.ParallelToolCalls = cloneAdapterBoolPtr(options.Tools.ParallelToolCalls)
	}
	if options.Tools.ToolCallMode != "" {
		req.ToolCallMode = options.Tools.ToolCallMode
	}

	return req, nil
}

func cloneAdapterToolChoice(choice *types.ToolChoice) *types.ToolChoice {
	if choice == nil {
		return nil
	}
	cloned := *choice
	cloned.ToolName = strings.TrimSpace(choice.ToolName)
	cloned.AllowedTools = append([]string(nil), choice.AllowedTools...)
	cloned.DisableParallelToolUse = cloneAdapterBoolPtr(choice.DisableParallelToolUse)
	cloned.IncludeServerSideToolInvocations = cloneAdapterBoolPtr(choice.IncludeServerSideToolInvocations)
	return &cloned
}

func toolChoiceToRequestValue(choice *types.ToolChoice) any {
	if choice == nil {
		return nil
	}
	return cloneAdapterToolChoice(choice)
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
