// Package runtime provides the agent runtime — the primary execution engine
// for single-agent workflows.
//
// Entry points:
//   - NewBuilder() — construct and configure an agent
//   - Agent.Run() — execute a single conversation turn
//
// The runtime orchestrates LLM calls, tool execution, memory operations,
// reflection, and checkpoint lifecycle. It is the single authoritative
// assembly point for all agent capabilities.
//
// This package may depend on agent/capabilities/, agent/persistence/, and llm/,
// but must not depend on workflow/, api/, cmd/, or internal/.
package runtime
