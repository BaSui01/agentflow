package a2a

import (
	"context"
	"time"
)

const (
	AgentTypeGeneric    AgentType = "generic"
	AgentTypeAssistant  AgentType = "assistant"
	AgentTypeAnalyzer   AgentType = "analyzer"
	AgentTypeTranslator AgentType = "translator"
	AgentTypeSummarizer AgentType = "summarizer"
	AgentTypeReviewer   AgentType = "reviewer"
)

// Agent is the minimal execution contract that the A2A server depends on.
// Keeping this protocol-local avoids importing the root agent package and
// prevents protocol/tests from creating reverse dependency cycles.
type Agent interface {
	ID() string
	Name() string
	Type() AgentType
	Execute(ctx context.Context, input *ExecutionInput) (*ExecutionOutput, error)
}

// ExecutionInput is the protocol-local task payload forwarded to registered agents.
type ExecutionInput struct {
	TraceID string         `json:"trace_id"`
	Content string         `json:"content"`
	Context map[string]any `json:"context,omitempty"`
}

// ExecutionOutput is the protocol-local execution result returned by registered agents.
type ExecutionOutput struct {
	TraceID      string        `json:"trace_id,omitempty"`
	Content      string        `json:"content"`
	TokensUsed   int           `json:"tokens_used,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

type agentDescriber interface {
	Description() string
}

type agentToolProvider interface {
	Tools() []string
}

type agentMetadataProvider interface {
	Metadata() map[string]string
}
