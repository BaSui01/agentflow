package execution

// DependenciesSatisfied reports whether cap can run with the current completed
// and failed dependency state.
func DependenciesSatisfied(capability string, deps map[string][]string, completed map[string]bool, failed map[string]error) bool {
	capDeps := deps[capability]
	if len(capDeps) == 0 {
		return true
	}

	for _, dep := range capDeps {
		if completed[dep] {
			continue
		}
		if _, ok := failed[dep]; ok {
			return false
		}
		return false
	}
	return true
}

// BuildCapabilityInput wraps originalInput with upstream dependency results.
func BuildCapabilityInput(
	capability string,
	originalInput any,
	deps map[string][]string,
	lookupResult func(string) (any, bool),
) map[string]any {
	capInput := map[string]any{
		"input": originalInput,
	}
	if lookupResult == nil {
		return capInput
	}

	capDeps := deps[capability]
	if len(capDeps) == 0 {
		return capInput
	}

	upstream := make(map[string]any, len(capDeps))
	for _, dep := range capDeps {
		if result, ok := lookupResult(dep); ok {
			upstream[dep] = result
		}
	}
	if len(upstream) > 0 {
		capInput["upstream"] = upstream
	}
	return capInput
}
