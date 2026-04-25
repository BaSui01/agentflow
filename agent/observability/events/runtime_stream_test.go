package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeStreamEventRunEventMapsToolCall(t *testing.T) {
	args := json.RawMessage(`{"query":"agent"}`)
	timestamp := time.Unix(10, 0)
	event := RuntimeStreamEvent{
		Type:         RuntimeStreamToolCall,
		SDKEventType: SDKRunItemEvent,
		SDKEventName: SDKToolCalled,
		Timestamp:    timestamp,
		ToolCall: &RuntimeToolCall{
			ID:        "call-1",
			Name:      "search",
			Arguments: args,
		},
	}

	runEvent := event.RunEvent()

	assert.Equal(t, types.RunEventToolCall, runEvent.Type)
	assert.Equal(t, types.RunScopeAgent, runEvent.Scope)
	assert.Equal(t, timestamp, runEvent.Timestamp)
	require.NotNil(t, runEvent.ToolCall)
	assert.Equal(t, "call-1", runEvent.ToolCall.ID)
	assert.Equal(t, "search", runEvent.ToolName)
	assert.Equal(t, "run_item_stream_event", runEvent.Metadata["sdk_event_type"])
	assert.Equal(t, "tool_called", runEvent.Metadata["sdk_event_name"])
}

func TestRuntimeStreamEventRunEventMapsTokenData(t *testing.T) {
	event := RuntimeStreamEvent{
		Type:      RuntimeStreamToken,
		Token:     "he",
		Delta:     "hello",
		Resumable: true,
	}

	runEvent := event.RunEvent()

	assert.Equal(t, types.RunEventLLMChunk, runEvent.Type)
	require.IsType(t, map[string]any{}, runEvent.Data)
	data := runEvent.Data.(map[string]any)
	assert.Equal(t, "he", data["token"])
	assert.Equal(t, "hello", data["delta"])
	assert.Equal(t, true, data["resumable"])
}
