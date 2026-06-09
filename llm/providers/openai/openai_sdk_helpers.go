package openai

import (
	"strings"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/types"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// resolvedOpenAIToolChoice holds the sub-components of a resolved OpenAI tool choice.
// Each wrapper function (for Responses API vs CountTokens API) maps these into its SDK-specific union type.
type resolvedOpenAIToolChoice struct {
	customTool   *responses.ToolChoiceCustomParam
	functionTool *responses.ToolChoiceFunctionParam
	allowedTools *responses.ToolChoiceAllowedParam
	mode         param.Opt[responses.ToolChoiceOptions]
}

// isEmpty returns true when no sub-component has been populated (i.e., the resolution did not
// produce a valid result and the caller should fall back to decodeSDKParam).
func (r resolvedOpenAIToolChoice) isEmpty() bool {
	return r.customTool == nil && r.functionTool == nil && r.allowedTools == nil && !r.mode.Valid()
}

// resolveOpenAIToolChoice normalizes the given tool_choice and resolves it into the shared
// sub-components that are identical between the Responses API and InputTokenCount API.
// Both buildSDKResponseToolChoice and buildSDKInputTokenToolChoice delegate to this function.
func resolveOpenAIToolChoice(choice any, tools []any) resolvedOpenAIToolChoice {
	normalized := providerbase.NormalizeToolChoice(choice)
	switch normalized.Mode {
	case "tool":
		name := strings.TrimSpace(normalized.SpecificName)
		if name == "" {
			return resolvedOpenAIToolChoice{}
		}
		if toolType := findResponseToolTypeByName(tools, name); toolType == types.ToolTypeCustom {
			return resolvedOpenAIToolChoice{
				customTool: &responses.ToolChoiceCustomParam{Name: name},
			}
		}
		return resolvedOpenAIToolChoice{
			functionTool: &responses.ToolChoiceFunctionParam{Name: name},
		}
	case "any", "validated":
		allowedTools := buildAllowedToolsChoice(tools)
		if len(allowedTools) == 0 {
			return resolvedOpenAIToolChoice{}
		}
		return resolvedOpenAIToolChoice{
			allowedTools: &responses.ToolChoiceAllowedParam{
				Mode:  responses.ToolChoiceAllowedModeRequired,
				Tools: allowedTools,
			},
		}
	case "auto":
		return resolvedOpenAIToolChoice{
			mode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	case "none":
		return resolvedOpenAIToolChoice{
			mode: param.NewOpt(responses.ToolChoiceOptionsNone),
		}
	default:
		return resolvedOpenAIToolChoice{}
	}
}

// buildResponseToolChoiceUnion wraps resolved components into ResponseNewParamsToolChoiceUnion.
func buildResponseToolChoiceUnion(r resolvedOpenAIToolChoice) responses.ResponseNewParamsToolChoiceUnion {
	if r.customTool != nil {
		return responses.ResponseNewParamsToolChoiceUnion{OfCustomTool: r.customTool}
	}
	if r.functionTool != nil {
		return responses.ResponseNewParamsToolChoiceUnion{OfFunctionTool: r.functionTool}
	}
	if r.allowedTools != nil {
		return responses.ResponseNewParamsToolChoiceUnion{OfAllowedTools: r.allowedTools}
	}
	if r.mode.Valid() {
		return responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: r.mode}
	}
	return responses.ResponseNewParamsToolChoiceUnion{}
}

// buildInputTokenToolChoiceUnion wraps resolved components into InputTokenCountParamsToolChoiceUnion.
func buildInputTokenToolChoiceUnion(r resolvedOpenAIToolChoice) responses.InputTokenCountParamsToolChoiceUnion {
	if r.customTool != nil {
		return responses.InputTokenCountParamsToolChoiceUnion{OfCustomTool: r.customTool}
	}
	if r.functionTool != nil {
		return responses.InputTokenCountParamsToolChoiceUnion{OfFunctionTool: r.functionTool}
	}
	if r.allowedTools != nil {
		return responses.InputTokenCountParamsToolChoiceUnion{OfAllowedTools: r.allowedTools}
	}
	if r.mode.Valid() {
		return responses.InputTokenCountParamsToolChoiceUnion{OfToolChoiceMode: r.mode}
	}
	return responses.InputTokenCountParamsToolChoiceUnion{}
}
