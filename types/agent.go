package types

// =============================================================================
// Minimal Agent Execution Interfaces
// =============================================================================
// Shared tiny interfaces that are safe to place in the lowest-level package.
// =============================================================================

// Named is an optional interface for components that have a display name.
// Use a type assertion to check if a value also implements Named:
//
//	if named, ok := executor.(types.Named); ok {
//	    fmt.Println(named.Name())
//	}
type Named interface {
	// Name returns the agent's human-readable display name.
	Name() string
}
