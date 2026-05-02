package types

import (
	"fmt"
	"strings"
)

// ModelCapabilityRequirement describes a capability required by a runtime
// request or agent execution option.
type ModelCapabilityRequirement struct {
	Capability ModelCapability
	Reason     string
}

// RequiredModelCapabilities derives provider-neutral model capabilities needed
// by the supplied execution options. It intentionally checks only features that
// have stable cross-provider catalog semantics; provider-specific limits remain
// in provider validation hooks.
func RequiredModelCapabilities(options ExecutionOptions) []ModelCapabilityRequirement {
	var out []ModelCapabilityRequirement
	add := func(capability ModelCapability, reason string) {
		for _, existing := range out {
			if existing.Capability == capability {
				return
			}
		}
		out = append(out, ModelCapabilityRequirement{Capability: capability, Reason: reason})
	}

	if toolsRequireModelCapability(options.Tools) {
		add(ModelCapabilityToolCalling, "tools requested")
	}
	if options.Model.ResponseFormat != nil && options.Model.ResponseFormat.Type != ResponseFormatText {
		add(ModelCapabilityStructuredOutput, "structured response format requested")
	}
	if modelOptionsRequireReasoning(options.Model) {
		add(ModelCapabilityReasoning, "reasoning or thinking requested")
	}
	if options.Model.WebSearchOptions != nil {
		add(ModelCapabilityWebSearch, "web search requested")
	}
	return out
}

// ValidateModelCapabilities checks whether a catalog descriptor supports all
// capabilities required by the supplied execution options. Missing catalog
// entries are treated as unknown and therefore non-blocking; callers can decide
// whether unknown models should be allowed by policy.
func ValidateModelCapabilities(catalog *ModelCatalog, options ExecutionOptions) error {
	if catalog == nil {
		return nil
	}
	provider := strings.TrimSpace(options.Model.Provider)
	model := strings.TrimSpace(options.Model.Model)
	if provider == "" || model == "" {
		return nil
	}
	descriptor, ok := catalog.Lookup(provider, model)
	if !ok {
		return nil
	}
	for _, requirement := range RequiredModelCapabilities(options) {
		if !descriptor.Supports(requirement.Capability) {
			return fmt.Errorf("model %s/%s does not declare capability %q required by %s", descriptor.Provider, descriptor.ID, requirement.Capability, requirement.Reason)
		}
	}
	return nil
}

func toolsRequireModelCapability(options ToolProtocolOptions) bool {
	if options.DisableTools {
		return false
	}
	if len(options.AllowedTools) > 0 || len(options.ToolWhitelist) > 0 || len(options.Handoffs) > 0 {
		return true
	}
	if options.ToolChoice != nil && options.ToolChoice.Mode != "" && options.ToolChoice.Mode != ToolChoiceModeNone {
		return true
	}
	if options.ToolCallMode != "" {
		return true
	}
	if options.ParallelToolCalls != nil {
		return true
	}
	return false
}

func modelOptionsRequireReasoning(options ModelOptions) bool {
	return strings.TrimSpace(options.ReasoningEffort) != "" ||
		strings.TrimSpace(options.ReasoningSummary) != "" ||
		strings.TrimSpace(options.ReasoningDisplay) != "" ||
		strings.TrimSpace(options.ReasoningMode) != "" ||
		strings.TrimSpace(options.ThinkingType) != "" ||
		strings.TrimSpace(options.ThinkingLevel) != "" ||
		options.ThinkingBudget != nil ||
		options.IncludeThoughts != nil
}
