package providerbase

import (
	"encoding/json"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

type NormalizedToolChoice struct {
	Mode                     string
	SpecificName             string
	AllowedFunctionNames     []string
	DisableParallelToolUse   *bool
	IncludeServerSideToolUse *bool
}

func NormalizeToolType(toolType string) string {
	switch strings.ToLower(strings.TrimSpace(toolType)) {
	case "", types.ToolTypeFunction:
		return types.ToolTypeFunction
	case types.ToolTypeCustom:
		return types.ToolTypeCustom
	default:
		return types.ToolTypeFunction
	}
}

func IsSearchToolPlaceholder(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "web_search", "web_search_preview", "google_search":
		return true
	default:
		return false
	}
}

func NormalizeToolChoice(choice any) NormalizedToolChoice {
	var normalized NormalizedToolChoice
	switch v := choice.(type) {
	case *types.ToolChoice:
		if v == nil {
			return normalized
		}
		switch v.Mode {
		case types.ToolChoiceModeAuto:
			normalized.Mode = "auto"
		case types.ToolChoiceModeRequired:
			normalized.Mode = "any"
		case types.ToolChoiceModeNone:
			normalized.Mode = "none"
		case types.ToolChoiceModeSpecific:
			normalized.Mode = "tool"
			normalized.SpecificName = strings.TrimSpace(v.ToolName)
		case types.ToolChoiceModeAllowed:
			normalized.Mode = "any"
			normalized.AllowedFunctionNames = NormalizeUniqueStrings(v.AllowedTools)
		}
		normalized.DisableParallelToolUse = v.DisableParallelToolUse
		normalized.IncludeServerSideToolUse = v.IncludeServerSideToolInvocations
	case types.ToolChoice:
		return NormalizeToolChoice(&v)
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "auto":
			normalized.Mode = "auto"
		case "required", "any":
			normalized.Mode = "any"
		case "none":
			normalized.Mode = "none"
		case "validated":
			normalized.Mode = "validated"
		case "":
		default:
			normalized.Mode = "tool"
			normalized.SpecificName = strings.TrimSpace(v)
		}
	case map[string]any:
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
				normalized.Mode = "tool"
				normalized.SpecificName = strings.TrimSpace(name)
				break
			}
		}
		if t, ok := v["type"].(string); ok && strings.TrimSpace(t) != "" {
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "function":
				normalized.Mode = "tool"
				if fn, ok := v["function"].(map[string]any); ok {
					if name, ok := fn["name"].(string); ok {
						normalized.SpecificName = strings.TrimSpace(name)
					}
				}
			case "auto", "any", "none", "validated":
				normalized.Mode = strings.ToLower(strings.TrimSpace(t))
			case "tool":
				normalized.Mode = "tool"
				if name, ok := v["name"].(string); ok {
					normalized.SpecificName = strings.TrimSpace(name)
				}
			}
		}
		if normalized.Mode == "" {
			mode, _ := v["mode"].(string)
			if mode == "" {
				mode, _ = v["Mode"].(string)
			}
			normalized.Mode = strings.ToLower(strings.TrimSpace(mode))
		}
		if normalized.Mode == "tool" && normalized.SpecificName == "" {
			if name, ok := v["name"].(string); ok {
				normalized.SpecificName = strings.TrimSpace(name)
			}
		}
		normalized.AllowedFunctionNames = NormalizeStringSliceAny(v["allowed_function_names"])
		if len(normalized.AllowedFunctionNames) == 0 {
			normalized.AllowedFunctionNames = NormalizeStringSliceAny(v["allowedFunctionNames"])
		}
		if disable, ok := v["disable_parallel_tool_use"].(bool); ok {
			normalized.DisableParallelToolUse = &disable
		}
		if include, ok := v["include_server_side_tool_invocations"].(bool); ok {
			normalized.IncludeServerSideToolUse = &include
		} else if include, ok := v["includeServerSideToolInvocations"].(bool); ok {
			normalized.IncludeServerSideToolUse = &include
		}
	}
	return normalized
}

func NormalizeStringSliceAny(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return NormalizeUniqueStrings(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				continue
			}
			out = append(out, text)
		}
		return NormalizeUniqueStrings(out)
	default:
		return nil
	}
}

func NormalizeUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ToolParametersSchemaMap(parameters json.RawMessage) map[string]any {
	if len(parameters) == 0 {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal(parameters, &params); err != nil {
		return nil
	}
	return params
}

func ConvertCustomToolFormat(format *types.ToolFormat) map[string]any {
	if format == nil {
		return nil
	}
	out := map[string]any{}
	if v := strings.TrimSpace(format.Type); v != "" {
		out["type"] = v
	}
	if v := strings.TrimSpace(format.Syntax); v != "" {
		out["syntax"] = v
	}
	if v := strings.TrimSpace(format.Definition); v != "" {
		out["definition"] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NewFunctionToolCall(id, name string, arguments json.RawMessage) types.ToolCall {
	return types.ToolCall{
		ID:        strings.TrimSpace(id),
		Type:      types.ToolTypeFunction,
		Name:      strings.TrimSpace(name),
		Arguments: arguments,
	}
}

func NewCustomToolCall(id, name, input string) types.ToolCall {
	return types.ToolCall{
		ID:    strings.TrimSpace(id),
		Type:  types.ToolTypeCustom,
		Name:  strings.TrimSpace(name),
		Input: input,
	}
}

func BuildToolCallTypeIndex(msgs []types.Message) map[string]string {
	out := make(map[string]string)
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) == "" {
				continue
			}
			out[tc.ID] = NormalizeToolType(tc.Type)
		}
	}
	return out
}

func ToolOutputResponseMap(content string) map[string]any {
	var response map[string]any
	if err := json.Unmarshal([]byte(content), &response); err == nil {
		return response
	}
	return map[string]any{"result": content}
}

func AppendToolJSONDelta(existing json.RawMessage, delta string) json.RawMessage {
	if strings.TrimSpace(delta) == "" {
		return existing
	}
	return append(existing, []byte(delta)...)
}

func ToolCallChunk(call types.ToolCall) []types.ToolCall {
	return []types.ToolCall{call}
}
