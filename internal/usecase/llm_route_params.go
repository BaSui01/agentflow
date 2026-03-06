package usecase

import (
	"regexp"
	"strings"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

var providerHintPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// SupportedRoutePolicies returns the list of valid route policy values.
func SupportedRoutePolicies() []string {
	return []string{
		string(llmcore.RoutePolicyBalanced),
		string(llmcore.RoutePolicyCostFirst),
		string(llmcore.RoutePolicyHealthFirst),
		string(llmcore.RoutePolicyLatencyFirst),
	}
}

// NormalizeProviderHint validates and normalizes the provider hint.
func NormalizeProviderHint(raw string) (string, *types.Error) {
	provider := strings.TrimSpace(raw)
	if provider == "" {
		return "", nil
	}
	if !providerHintPattern.MatchString(provider) {
		return "", types.NewInvalidRequestError("provider has invalid format")
	}
	return provider, nil
}

// NormalizeRoutePolicy validates and normalizes the route policy.
func NormalizeRoutePolicy(raw string) (llmcore.RoutePolicy, *types.Error) {
	policy := strings.ToLower(strings.TrimSpace(raw))
	if policy == "" {
		return llmcore.RoutePolicyBalanced, nil
	}
	switch llmcore.RoutePolicy(policy) {
	case llmcore.RoutePolicyBalanced,
		llmcore.RoutePolicyCostFirst,
		llmcore.RoutePolicyHealthFirst,
		llmcore.RoutePolicyLatencyFirst:
		return llmcore.RoutePolicy(policy), nil
	default:
		return "", types.NewInvalidRequestError("route_policy must be one of: balanced, cost_first, health_first, latency_first")
	}
}

// NormalizeEndpointMode validates and normalizes the endpoint mode.
func NormalizeEndpointMode(raw string) (string, *types.Error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "", nil
	}
	switch mode {
	case "auto", "responses", "chat_completions":
		return mode, nil
	default:
		return "", types.NewInvalidRequestError("endpoint_mode must be one of: auto, responses, chat_completions")
	}
}

// NormalizeRouteTags deduplicates and trims route tags.
func NormalizeRouteTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, tag := range in {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeRouteMetadata trims metadata keys.
func NormalizeRouteMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ApplyChatRouteMetadata merges provider, policy, and endpoint mode into metadata.
func ApplyChatRouteMetadata(metadata map[string]string, provider string, policy llmcore.RoutePolicy, endpointMode string) map[string]string {
	out := NormalizeRouteMetadata(metadata)
	if out == nil {
		out = make(map[string]string)
	}
	if provider != "" {
		out[llmcore.MetadataKeyChatProvider] = provider
	}
	if policy != "" {
		out["route_policy"] = string(policy)
	}
	if endpointMode != "" {
		out["endpoint_mode"] = endpointMode
	}
	return out
}
