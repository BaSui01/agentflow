package agent

import (
	"context"
	"encoding/json"
	"time"
)

type runtimeStreamEmitterKey struct{}

type RuntimeStreamEventType string

const (
	RuntimeStreamToken      RuntimeStreamEventType = "token"
	RuntimeStreamToolCall   RuntimeStreamEventType = "tool_call"
	RuntimeStreamToolResult RuntimeStreamEventType = "tool_result"
)

type RuntimeToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type RuntimeToolResult struct {
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration,omitempty"`
}

type RuntimeStreamEvent struct {
	Type       RuntimeStreamEventType `json:"type"`
	Timestamp  time.Time              `json:"timestamp"`
	Token      string                 `json:"token,omitempty"`
	Delta      string                 `json:"delta,omitempty"`
	ToolCall   *RuntimeToolCall       `json:"tool_call,omitempty"`
	ToolResult *RuntimeToolResult     `json:"tool_result,omitempty"`
}

type RuntimeStreamEmitter func(RuntimeStreamEvent)

func WithRuntimeStreamEmitter(ctx context.Context, emit RuntimeStreamEmitter) context.Context {
	if emit == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeStreamEmitterKey{}, emit)
}

func runtimeStreamEmitterFromContext(ctx context.Context) (RuntimeStreamEmitter, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(runtimeStreamEmitterKey{})
	if v == nil {
		return nil, false
	}
	emit, ok := v.(RuntimeStreamEmitter)
	return emit, ok && emit != nil
}
