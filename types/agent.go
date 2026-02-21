package types

import "context"

// =============================================================================
// Minimal Agent Execution Interfaces
// =============================================================================
// These interfaces define the smallest common contract shared by all agent
// variants in the framework (agent.Agent, workflow.AgentExecutor,
// conversation.ConversationAgent, crews.CrewAgent, etc.).
//
// The types package is the lowest-level package with no internal dependencies,
// so placing these interfaces here avoids circular imports.
// =============================================================================

// Executor is the minimal agent execution interface.
// All agent variants share this common contract: an identity (ID) and the
// ability to execute with arbitrary input/output.
//
// Domain-specific agent interfaces (conversation, crew, handoff, evaluation)
// extend or specialize this contract for their own needs.
type Executor interface {
	// ID returns the agent's unique identifier.
	ID() string
	// Execute runs the agent with the given input and returns the result.
	Execute(ctx context.Context, input any) (any, error)
}

// Named is an optional interface for agents that have a display name.
// Use a type assertion to check if an Executor also implements Named:
//
//	if named, ok := executor.(types.Named); ok {
//	    fmt.Println(named.Name())
//	}
type Named interface {
	// Name returns the agent's human-readable display name.
	Name() string
}
