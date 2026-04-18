package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestPrepareChatRequest_AppendsRuntimeHandoffTools(t *testing.T) {
	source := NewBaseAgent(testAgentConfig("source-agent", "Source", "gpt-4"), testGatewayFromProvider(&testProvider{
		name:           "source",
		supportsNative: true,
	}),

		nil, &testToolManager{
			getAllowedToolsFn: func(agentID string) []types.ToolSchema {
				return []types.ToolSchema{{
					Type:        types.ToolTypeFunction,
					Name:        "search",
					Description: "search",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				}}
			},
		}, nil, zap.NewNop(), nil)

	source.config.Runtime.Tools = []string{"search"}
	target := NewBaseAgent(testAgentConfig("target-agent", "Target", "gpt-4"), testGatewayFromProvider(&testProvider{
		name:           "target",
		supportsNative: true,
	}),

		nil, nil, nil, zap.NewNop(), nil)

	pr, err := source.prepareChatRequest(
		WithRuntimeHandoffTargets(context.Background(), []RuntimeHandoffTarget{{Agent: target}}),
		[]types.Message{{Role: llm.RoleUser, Content: "delegate"}},
	)
	if err != nil {
		t.Fatalf("prepareChatRequest returned error: %v", err)
	}

	if len(pr.req.Tools) != 2 {
		t.Fatalf("expected 2 tools (regular + handoff), got %d", len(pr.req.Tools))
	}
	if _, ok := pr.handoffTools["transfer_to_target_agent"]; !ok {
		t.Fatalf("expected transfer_to_target_agent handoff tool, got %#v", pr.handoffTools)
	}
}

func TestBaseAgent_Execute_WithRuntimeHandoff_EmitsSDKEvents(t *testing.T) {
	sourceProvider := &testProvider{
		name:           "source",
		supportsNative: true,
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				ID:       "src-plan-1",
				Provider: "source",
				Model:    "gpt-4o-mini",
				Choices: []llm.ChatChoice{{
					Message: types.Message{
						Role: llm.RoleAssistant,
						ToolCalls: []types.ToolCall{{
							ID:        "call_source_plan",
							Name:      submitNumberedPlanTool,
							Arguments: json.RawMessage(`{"steps":["delegate to the target agent"]}`),
						}},
					},
					FinishReason: "tool_calls",
				}},
				Usage: llm.ChatUsage{CompletionTokens: 5, TotalTokens: 5},
			}, nil
		},
		streamFn: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			ch := make(chan llm.StreamChunk, 1)
			go func() {
				defer close(ch)
				ch <- llm.StreamChunk{
					ID:       "src-turn-1",
					Provider: "source",
					Model:    "gpt-4o-mini",
					Delta: types.Message{
						Role: llm.RoleAssistant,
						ToolCalls: []types.ToolCall{{
							Index:     0,
							ID:        "call_handoff",
							Name:      "transfer_to_target_agent",
							Arguments: json.RawMessage(`{"input":"Please take over"}`),
						}},
					},
					FinishReason: "tool_calls",
				}
			}()
			return ch, nil
		},
	}
	targetProvider := &testProvider{
		name:           "target",
		supportsNative: true,
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				ID:       "target-plan-1",
				Provider: "target",
				Model:    "gpt-4.1",
				Choices: []llm.ChatChoice{{
					Message: types.Message{
						Role: llm.RoleAssistant,
						ToolCalls: []types.ToolCall{{
							ID:        "call_target_plan",
							Name:      submitNumberedPlanTool,
							Arguments: json.RawMessage(`{"steps":["produce the delegated answer"]}`),
						}},
					},
					FinishReason: "tool_calls",
				}},
				Usage: llm.ChatUsage{CompletionTokens: 5, TotalTokens: 5},
			}, nil
		},
		streamFn: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			ch := make(chan llm.StreamChunk, 1)
			go func() {
				defer close(ch)
				ch <- llm.StreamChunk{
					ID:       "target-turn-1",
					Provider: "target",
					Model:    "gpt-4.1",
					Delta: types.Message{
						Role:    llm.RoleAssistant,
						Content: "delegated answer",
					},
					FinishReason: "stop",
					Usage: &llm.ChatUsage{
						CompletionTokens: 11,
						TotalTokens:      11,
					},
				}
			}()
			return ch, nil
		},
	}

	source := NewBaseAgent(testAgentConfig("source-agent", "Source", "gpt-4o-mini"), testGatewayFromProvider(sourceProvider), nil, nil, nil, zap.NewNop(), nil)
	target := NewBaseAgent(testAgentConfig("target-agent", "Target", "gpt-4.1"), testGatewayFromProvider(targetProvider), nil, nil, nil, zap.NewNop(), nil)
	if err := source.Init(context.Background()); err != nil {
		t.Fatalf("source init failed: %v", err)
	}
	if err := target.Init(context.Background()); err != nil {
		t.Fatalf("target init failed: %v", err)
	}

	recorder := &loopRuntimeEventRecorder{}
	ctx := WithRuntimeStreamEmitter(
		WithRuntimeHandoffTargets(context.Background(), []RuntimeHandoffTarget{{Agent: target}}),
		recorder.emit,
	)
	output, err := source.Execute(ctx, &Input{
		TraceID: "trace-handoff",
		Content: "delegate this request",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if output.Content != "delegated answer" {
		t.Fatalf("expected delegated answer, got %q", output.Content)
	}

	var requested, occured, updated bool
	for _, event := range recorder.events {
		if event.SDKEventType == SDKRunItemEvent && event.SDKEventName == SDKHandoffRequested {
			requested = true
		}
		if event.SDKEventType == SDKRunItemEvent && event.SDKEventName == SDKHandoffOccured {
			occured = true
		}
		if event.SDKEventType == SDKAgentUpdatedEvent {
			updated = true
		}
	}

	if !requested {
		t.Fatalf("expected handoff_requested stream event")
	}
	if !occured {
		t.Fatalf("expected handoff_occured stream event")
	}
	if !updated {
		t.Fatalf("expected agent_updated_stream_event")
	}
}
