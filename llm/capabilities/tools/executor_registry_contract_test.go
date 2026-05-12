package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type contractRegistry struct {
	fn             ToolFunc
	meta           ToolMetadata
	streamingFn    StreamingToolFunc
	rateLimitCalls atomic.Int32
	streamingCalls atomic.Int32
}

func (r *contractRegistry) Register(name string, fn ToolFunc, metadata ToolMetadata) error {
	return nil
}
func (r *contractRegistry) Unregister(name string) error { return nil }
func (r *contractRegistry) Get(name string) (ToolFunc, ToolMetadata, error) {
	if r.fn == nil {
		return nil, ToolMetadata{}, fmt.Errorf("not found")
	}
	return r.fn, r.meta, nil
}
func (r *contractRegistry) List() []llmpkg.ToolSchema { return []llmpkg.ToolSchema{r.meta.Schema} }
func (r *contractRegistry) Has(name string) bool      { return r.fn != nil }
func (r *contractRegistry) CheckRateLimit(name string) error {
	r.rateLimitCalls.Add(1)
	return nil
}
func (r *contractRegistry) GetStreaming(name string) (StreamingToolFunc, bool) {
	r.streamingCalls.Add(1)
	return r.streamingFn, r.streamingFn != nil
}

func TestDefaultExecutor_UsesRegistryRateLimitContractWithoutConcreteType(t *testing.T) {
	registry := &contractRegistry{
		fn: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
		meta: ToolMetadata{Schema: llmpkg.ToolSchema{Name: "custom"}, Timeout: time.Second},
	}
	executor := NewDefaultExecutor(registry, zap.NewNop())

	result := executor.ExecuteOne(context.Background(), llmpkg.ToolCall{Name: "custom", Arguments: json.RawMessage(`{}`)})

	require.Empty(t, result.Error)
	assert.Equal(t, int32(1), registry.rateLimitCalls.Load())
}

func TestDefaultExecutor_UsesRegistryStreamingContractWithoutConcreteType(t *testing.T) {
	registry := &contractRegistry{
		fn: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"fallback":true}`), nil
		},
		meta: ToolMetadata{Schema: llmpkg.ToolSchema{Name: "custom_stream"}, Timeout: time.Second},
		streamingFn: func(ctx context.Context, args json.RawMessage, emit ToolProgressEmitter) (json.RawMessage, error) {
			emit(ToolStreamEvent{Type: ToolStreamOutput, Data: "streamed"})
			return json.RawMessage(`{"streamed":true}`), nil
		},
	}
	executor := NewDefaultExecutor(registry, zap.NewNop())

	var events []ToolStreamEvent
	for event := range executor.ExecuteOneStream(context.Background(), llmpkg.ToolCall{Name: "custom_stream", Arguments: json.RawMessage(`{}`)}) {
		events = append(events, event)
	}

	assert.Equal(t, int32(1), registry.streamingCalls.Load())
	assert.Equal(t, int32(1), registry.rateLimitCalls.Load())
	require.NotEmpty(t, events)
	assert.Equal(t, ToolStreamOutput, events[1].Type)
}
