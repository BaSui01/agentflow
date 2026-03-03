package handlers

import (
	"regexp"
	"strings"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

var providerHintPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func supportedRoutePolicies() []string {
	return []string{
		string(llmcore.RoutePolicyBalanced),
		string(llmcore.RoutePolicyCostFirst),
		string(llmcore.RoutePolicyHealthFirst),
		string(llmcore.RoutePolicyLatencyFirst),
	}
}

func normalizeProviderHint(raw string) (string, *types.Error) {
	provider := strings.TrimSpace(raw)
	if provider == "" {
		return "", nil
	}
	if !providerHintPattern.MatchString(provider) {
		return "", types.NewInvalidRequestError("provider has invalid format")
	}
	return provider, nil
}

func normalizeRoutePolicy(raw string) (llmcore.RoutePolicy, *types.Error) {
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

func normalizeRouteTags(in []string) []string {
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

func normalizeRouteMetadata(in map[string]string) map[string]string {
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

func applyChatRouteMetadata(metadata map[string]string, provider string, policy llmcore.RoutePolicy) map[string]string {
	out := normalizeRouteMetadata(metadata)
	if out == nil {
		out = make(map[string]string)
	}
	if provider != "" {
		out[llmcore.MetadataKeyChatProvider] = provider
	}
	if policy != "" {
		out["route_policy"] = string(policy)
	}
	return out
}
