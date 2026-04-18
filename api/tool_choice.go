package api

import (
	"strings"

	"github.com/BaSui01/agentflow/types"
)

// ParseToolChoice normalizes inbound API tool_choice payloads into the formal
// runtime ToolChoice surface consumed by llm/types.
func ParseToolChoice(raw any) *types.ToolChoice {
	switch v := raw.(type) {
	case nil:
		return nil
	case *types.ToolChoice:
		if v == nil {
			return nil
		}
		cloned := *v
		cloned.AllowedTools = append([]string(nil), v.AllowedTools...)
		cloned.DisableParallelToolUse = cloneToolChoiceBool(v.DisableParallelToolUse)
		cloned.IncludeServerSideToolInvocations = cloneToolChoiceBool(v.IncludeServerSideToolInvocations)
		return &cloned
	case types.ToolChoice:
		return ParseToolChoice(&v)
	case string:
		return types.ParseToolChoiceString(v)
	case map[string]any:
		return parseToolChoiceMap(v)
	default:
		return nil
	}
}

func parseToolChoiceMap(raw map[string]any) *types.ToolChoice {
	if raw == nil {
		return nil
	}

	mode := strings.ToLower(strings.TrimSpace(readToolChoiceString(raw, "mode", "Mode")))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(readToolChoiceString(raw, "type")))
	}

	toolName := strings.TrimSpace(readToolChoiceString(raw, "tool_name", "toolName", "name"))
	if toolName == "" {
		if fn, ok := raw["function"].(map[string]any); ok {
			toolName = strings.TrimSpace(readToolChoiceString(fn, "name"))
		}
	}

	allowed := normalizeToolChoiceStrings(
		raw["allowed_tools"],
		raw["allowedTools"],
		raw["allowed_function_names"],
		raw["allowedFunctionNames"],
	)
	disableParallel := readToolChoiceBool(raw, "disable_parallel_tool_use", "disableParallelToolUse")
	includeServerSide := readToolChoiceBool(raw, "include_server_side_tool_invocations", "includeServerSideToolInvocations")

	switch mode {
	case "", "function", "tool":
		if toolName == "" {
			if len(allowed) > 0 {
				return &types.ToolChoice{
					Mode:                             types.ToolChoiceModeAllowed,
					AllowedTools:                     allowed,
					DisableParallelToolUse:           disableParallel,
					IncludeServerSideToolInvocations: includeServerSide,
				}
			}
			return nil
		}
		return &types.ToolChoice{
			Mode:                             types.ToolChoiceModeSpecific,
			ToolName:                         toolName,
			DisableParallelToolUse:           disableParallel,
			IncludeServerSideToolInvocations: includeServerSide,
		}
	case "auto":
		return &types.ToolChoice{
			Mode:                             types.ToolChoiceModeAuto,
			DisableParallelToolUse:           disableParallel,
			IncludeServerSideToolInvocations: includeServerSide,
		}
	case "none":
		return &types.ToolChoice{
			Mode:                             types.ToolChoiceModeNone,
			DisableParallelToolUse:           disableParallel,
			IncludeServerSideToolInvocations: includeServerSide,
		}
	case "required", "any", "validated":
		choice := &types.ToolChoice{
			Mode:                             types.ToolChoiceModeRequired,
			DisableParallelToolUse:           disableParallel,
			IncludeServerSideToolInvocations: includeServerSide,
		}
		if len(allowed) > 0 {
			choice.Mode = types.ToolChoiceModeAllowed
			choice.AllowedTools = allowed
		}
		return choice
	case "allowed":
		return &types.ToolChoice{
			Mode:                             types.ToolChoiceModeAllowed,
			AllowedTools:                     allowed,
			DisableParallelToolUse:           disableParallel,
			IncludeServerSideToolInvocations: includeServerSide,
		}
	default:
		if toolName == "" {
			return nil
		}
		return &types.ToolChoice{
			Mode:                             types.ToolChoiceModeSpecific,
			ToolName:                         toolName,
			DisableParallelToolUse:           disableParallel,
			IncludeServerSideToolInvocations: includeServerSide,
		}
	}
}

func readToolChoiceString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func readToolChoiceBool(raw map[string]any, keys ...string) *bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		flag, ok := value.(bool)
		if ok {
			return &flag
		}
	}
	return nil
}

func normalizeToolChoiceStrings(values ...any) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, raw := range values {
		switch v := raw.(type) {
		case []string:
			for _, item := range v {
				appendToolChoiceString(&out, seen, item)
			}
		case []any:
			for _, item := range v {
				text, ok := item.(string)
				if ok {
					appendToolChoiceString(&out, seen, text)
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendToolChoiceString(out *[]string, seen map[string]struct{}, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	if _, ok := seen[trimmed]; ok {
		return
	}
	seen[trimmed] = struct{}{}
	*out = append(*out, trimmed)
}

func cloneToolChoiceBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}
