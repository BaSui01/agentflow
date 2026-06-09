package execution

// CalculateExecutionOrder returns a dependency-first topological order for capabilities.
// The dependencies map uses capability -> dependencies: a key depends on each listed value.
func CalculateExecutionOrder(capabilities []string, dependencies map[string][]string) []string {
	if len(capabilities) == 0 && len(dependencies) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	nodes := make([]string, 0, len(capabilities))
	for _, cap := range capabilities {
		if !seen[cap] {
			seen[cap] = true
			nodes = append(nodes, cap)
		}
	}
	for cap, deps := range dependencies {
		if !seen[cap] {
			seen[cap] = true
			nodes = append(nodes, cap)
		}
		for _, dep := range deps {
			if !seen[dep] {
				seen[dep] = true
				nodes = append(nodes, dep)
			}
		}
	}

	state := make(map[string]uint8, len(nodes))
	order := make([]string, 0, len(nodes))
	var visit func(string)
	visit = func(cap string) {
		switch state[cap] {
		case 1, 2:
			return
		}
		state[cap] = 1
		for _, dep := range dependencies[cap] {
			visit(dep)
		}
		state[cap] = 2
		order = append(order, cap)
	}
	for _, cap := range nodes {
		visit(cap)
	}
	return order
}

// HasCircularDependency reports whether start reaches itself through dependency edges.
func HasCircularDependency(dependencyGraph map[string][]string, start string) bool {
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var visit func(string) bool
	visit = func(cap string) bool {
		if visiting[cap] {
			return true
		}
		if visited[cap] {
			return false
		}
		visiting[cap] = true
		for _, dep := range dependencyGraph[cap] {
			if visit(dep) {
				return true
			}
		}
		delete(visiting, cap)
		visited[cap] = true
		return false
	}

	return visit(start)
}
