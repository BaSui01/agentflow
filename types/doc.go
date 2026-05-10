// Package types defines the zero-dependency core type contracts for AgentFlow.
//
// This package sits at the leaf of the dependency tree — no other AgentFlow
// package may import types while types itself must not depend on any other
// AgentFlow package. All cross-layer data structures and error codes that
// multiple layers need to share belong here.
//
// Key abstractions:
//   - MemoryKind / MemoryRecord — memory subsystem type contract
//   - AgentConfig — agent configuration struct
//   - Error / ErrorCode — structured error types
//   - LLM request/response types shared between llm and agent layers
package types
