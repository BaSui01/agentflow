package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agenthandoff "github.com/BaSui01/agentflow/agent/handoff"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
)

type runtimeHandoffTargetsKey struct{}
type runtimeConversationMessagesKey struct{}

const (
	internalContextHandoffMessages = "_agentflow_handoff_messages"
	internalContextParentHandoff   = "_agentflow_parent_handoff"
	internalContextFromAgentID     = "_agentflow_from_agent_id"
	internalContextHandoffTool     = "_agentflow_handoff_tool"
)

type RuntimeHandoffTarget struct {
	Agent       Agent
	ToolName    string
	Description string
}

type runtimeHandoffCallArgs struct {
	Input   string `json:"input,omitempty"`
	Task    string `json:"task,omitempty"`
	Message string `json:"message,omitempty"`
}

func WithRuntimeHandoffTargets(ctx context.Context, targets []RuntimeHandoffTarget) context.Context {
	filtered := cloneRuntimeHandoffTargets(targets)
	if len(filtered) == 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeHandoffTargetsKey{}, filtered)
}

func runtimeHandoffTargetsFromContext(ctx context.Context, currentAgentID string) []RuntimeHandoffTarget {
	if ctx == nil {
		return nil
	}
	raw, _ := ctx.Value(runtimeHandoffTargetsKey{}).([]RuntimeHandoffTarget)
	if len(raw) == 0 {
		return nil
	}
	currentAgentID = strings.TrimSpace(currentAgentID)
	out := make([]RuntimeHandoffTarget, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, target := range raw {
		if target.Agent == nil {
			continue
		}
		if currentAgentID != "" && strings.TrimSpace(target.Agent.ID()) == currentAgentID {
			continue
		}
		name := runtimeHandoffToolName(target.ToolName, target.Agent.ID())
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, RuntimeHandoffTarget{
			Agent:       target.Agent,
			ToolName:    name,
			Description: runtimeHandoffToolDescription(target.Description, target.Agent),
		})
	}
	return out
}

func cloneRuntimeHandoffTargets(targets []RuntimeHandoffTarget) []RuntimeHandoffTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]RuntimeHandoffTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target.Agent == nil {
			continue
		}
		name := runtimeHandoffToolName(target.ToolName, target.Agent.ID())
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, RuntimeHandoffTarget{
			Agent:       target.Agent,
			ToolName:    name,
			Description: runtimeHandoffToolDescription(target.Description, target.Agent),
		})
	}
	return out
}

func WithRuntimeConversationMessages(ctx context.Context, messages []types.Message) context.Context {
	if len(messages) == 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cloned := make([]types.Message, len(messages))
	copy(cloned, messages)
	return context.WithValue(ctx, runtimeConversationMessagesKey{}, cloned)
}

func runtimeConversationMessagesFromContext(ctx context.Context) []types.Message {
	if ctx == nil {
		return nil
	}
	raw, _ := ctx.Value(runtimeConversationMessagesKey{}).([]types.Message)
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]types.Message, len(raw))
	copy(cloned, raw)
	return cloned
}

func runtimeHandoffToolName(override string, agentID string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	s := strings.ToLower(strings.TrimSpace(agentID))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	if s == "" {
		s = "agent"
	}
	return "transfer_to_" + s
}

func runtimeHandoffToolDescription(override string, target Agent) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	if target == nil {
		return "Handoff to another agent to handle the request."
	}
	name := strings.TrimSpace(target.Name())
	if name == "" {
		name = strings.TrimSpace(target.ID())
	}
	if name == "" {
		return "Handoff to another agent to handle the request."
	}
	return fmt.Sprintf("Handoff to the %s agent to continue handling the request.", name)
}

func runtimeHandoffToolSchema(target RuntimeHandoffTarget) types.ToolSchema {
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        runtimeHandoffToolName(target.ToolName, target.Agent.ID()),
		Description: runtimeHandoffToolDescription(target.Description, target.Agent),
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"input":{"type":"string","description":"Optional transfer prompt for the next agent."},
				"task":{"type":"string","description":"Optional concise task description for the next agent."},
				"message":{"type":"string","description":"Optional handoff note for the next agent."}
			},
			"additionalProperties":false
		}`),
	}
}

func handoffMessagesFromInputContext(values map[string]any) []types.Message {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values[internalContextHandoffMessages]
	if !ok {
		return nil
	}
	messages, ok := raw.([]types.Message)
	if !ok || len(messages) == 0 {
		return nil
	}
	cloned := make([]types.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func publicInputContext(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		switch key {
		case internalContextHandoffMessages, internalContextParentHandoff, internalContextFromAgentID, internalContextHandoffTool:
			continue
		case "memory_context", "retrieval_context", "tool_state", "skill_context", "checkpoint_id":
			continue
		default:
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type runtimeHandoffExecutor struct {
	base    toolManagerExecutor
	owner   *BaseAgent
	targets map[string]RuntimeHandoffTarget
}

func newRuntimeHandoffExecutor(owner *BaseAgent, base toolManagerExecutor, targets []RuntimeHandoffTarget) *runtimeHandoffExecutor {
	targetMap := make(map[string]RuntimeHandoffTarget, len(targets))
	for _, target := range targets {
		name := runtimeHandoffToolName(target.ToolName, target.Agent.ID())
		target.ToolName = name
		target.Description = runtimeHandoffToolDescription(target.Description, target.Agent)
		targetMap[name] = target
	}
	return &runtimeHandoffExecutor{
		base:    base,
		owner:   owner,
		targets: targetMap,
	}
}

func (e *runtimeHandoffExecutor) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	if len(calls) == 0 {
		return nil
	}
	results := make([]types.ToolResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, e.ExecuteOne(ctx, call))
	}
	return results
}

func (e *runtimeHandoffExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	if target, ok := e.targets[call.Name]; ok {
		return e.executeRuntimeHandoff(ctx, target, call)
	}
	return e.base.ExecuteOne(ctx, call)
}

func (e *runtimeHandoffExecutor) ExecuteOneStream(ctx context.Context, call types.ToolCall) <-chan llmtools.ToolStreamEvent {
	ch := make(chan llmtools.ToolStreamEvent, 1)
	go func() {
		defer close(ch)
		result := e.ExecuteOne(ctx, call)
		if result.Error != "" {
			ch <- llmtools.ToolStreamEvent{
				Type:     llmtools.ToolStreamError,
				ToolName: call.Name,
				Error:    fmt.Errorf("%s", result.Error),
			}
			return
		}
		ch <- llmtools.ToolStreamEvent{
			Type:     llmtools.ToolStreamComplete,
			ToolName: call.Name,
			Data:     result,
		}
	}()
	return ch
}

func (e *runtimeHandoffExecutor) executeRuntimeHandoff(ctx context.Context, target RuntimeHandoffTarget, call types.ToolCall) types.ToolResult {
	if e.owner == nil || target.Agent == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "handoff target is not configured"}
	}

	manager := agenthandoff.NewHandoffManager(e.owner.logger)
	manager.RegisterAgent(runtimeHandoffAgentAdapter{agent: target.Agent})

	taskInput, taskDescription := parseRuntimeHandoffCall(call)
	conversationMessages := runtimeConversationMessagesFromContext(ctx)
	if len(conversationMessages) > 0 {
		conversationMessages = append(conversationMessages, types.Message{
			Role:      types.RoleAssistant,
			ToolCalls: []types.ToolCall{call},
		})
	}

	ho, err := manager.Handoff(ctx, agenthandoff.HandoffOptions{
		FromAgentID: e.owner.ID(),
		ToAgentID:   target.Agent.ID(),
		Task: agenthandoff.Task{
			Type:        "agent_handoff",
			Description: taskDescription,
			Input:       taskInput,
			Metadata: map[string]any{
				"tool_call_id": call.ID,
				"tool_name":    call.Name,
			},
		},
		Context: agenthandoff.HandoffContext{
			ConversationID: e.owner.ID(),
			Messages:       conversationMessages,
		},
		Wait:                    true,
		ToolNameOverride:        target.ToolName,
		ToolDescriptionOverride: target.Description,
		OnHandoff: func(ctx context.Context, handoff *agenthandoff.Handoff) error {
			emitRuntimeHandoffAgentUpdated(ctx, target.Agent, handoff)
			return nil
		},
	})
	if err != nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
	}
	if ho == nil || ho.Result == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "handoff completed without a result"}
	}
	if ho.Result.Error != "" {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: ho.Result.Error}
	}

	payload := types.ToolResultControl{
		Type: types.ToolResultControlTypeHandoff,
		Handoff: &types.ToolResultHandoff{
			HandoffID:       ho.ID,
			FromAgentID:     ho.FromAgentID,
			ToAgentID:       ho.ToAgentID,
			ToAgentName:     target.Agent.Name(),
			TransferMessage: ho.TransferMessage,
			Output:          fmt.Sprintf("%v", ho.Result.Output),
		},
	}
	if runtimePayload, ok := ho.Result.Output.(runtimeHandoffOutput); ok {
		payload.Handoff.Output = runtimePayload.Content
		payload.Handoff.Metadata = runtimePayload.Metadata
		payload.Handoff.Provider = runtimePayload.Provider
		payload.Handoff.Model = runtimePayload.Model
		payload.Handoff.TokensUsed = runtimePayload.TokensUsed
		payload.Handoff.FinishReason = runtimePayload.FinishReason
		payload.Handoff.ReasoningContent = runtimePayload.ReasoningContent
	}
	raw, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: marshalErr.Error()}
	}

	return types.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Result:     raw,
	}
}

func parseRuntimeHandoffCall(call types.ToolCall) (string, string) {
	var args runtimeHandoffCallArgs
	if len(call.Arguments) > 0 {
		_ = json.Unmarshal(call.Arguments, &args)
	}
	input := strings.TrimSpace(firstNonEmpty(args.Input, args.Task, args.Message, call.Input))
	if input == "" {
		input = fmt.Sprintf("Continue handling the request via %s.", call.Name)
	}
	description := strings.TrimSpace(firstNonEmpty(args.Task, args.Message, input))
	if description == "" {
		description = input
	}
	return input, description
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type runtimeHandoffAgentAdapter struct {
	agent Agent
}

func (a runtimeHandoffAgentAdapter) ID() string { return a.agent.ID() }

func (a runtimeHandoffAgentAdapter) Capabilities() []agenthandoff.AgentCapability {
	return []agenthandoff.AgentCapability{{
		Name:      a.agent.Name(),
		TaskTypes: []string{"agent_handoff"},
		Priority:  1,
	}}
}

func (a runtimeHandoffAgentAdapter) CanHandle(agenthandoff.Task) bool { return true }
func (a runtimeHandoffAgentAdapter) AcceptHandoff(context.Context, *agenthandoff.Handoff) error {
	return nil
}

func (a runtimeHandoffAgentAdapter) ExecuteHandoff(ctx context.Context, ho *agenthandoff.Handoff) (*agenthandoff.HandoffResult, error) {
	traceID, _ := types.TraceID(ctx)
	input := &Input{
		TraceID:   traceID,
		ChannelID: ho.Context.ConversationID,
		Content:   strings.TrimSpace(fmt.Sprintf("%v", ho.Task.Input)),
		Context: map[string]any{
			internalContextHandoffMessages: ho.Context.Messages,
			internalContextParentHandoff:   ho.ID,
			internalContextFromAgentID:     ho.FromAgentID,
			internalContextHandoffTool:     ho.ToolName,
		},
	}
	if input.Content == "" {
		input.Content = ho.Task.Description
	}
	output, err := a.agent.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	if output == nil {
		return &agenthandoff.HandoffResult{Output: ""}, nil
	}
	return &agenthandoff.HandoffResult{
		Output: runtimeHandoffOutput{
			Content:          output.Content,
			Metadata:         cloneMetadata(output.Metadata),
			TokensUsed:       output.TokensUsed,
			FinishReason:     output.FinishReason,
			ReasoningContent: output.ReasoningContent,
		},
	}, nil
}

type runtimeHandoffOutput struct {
	Content          string         `json:"content"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	TokensUsed       int            `json:"tokens_used,omitempty"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	ReasoningContent *string        `json:"reasoning_content,omitempty"`
}

func emitRuntimeHandoffAgentUpdated(ctx context.Context, target Agent, handoff *agenthandoff.Handoff) {
	emit, ok := runtimeStreamEmitterFromContext(ctx)
	if !ok || target == nil {
		return
	}
	emit(RuntimeStreamEvent{
		Type:         RuntimeStreamStatus,
		SDKEventType: SDKAgentUpdatedEvent,
		Timestamp:    time.Now(),
		CurrentStage: "handoff",
		Data: map[string]any{
			"status": "agent_updated",
			"new_agent": map[string]any{
				"id":   target.ID(),
				"name": target.Name(),
				"type": target.Type(),
			},
			"handoff_id":       handoff.ID,
			"from_agent_id":    handoff.FromAgentID,
			"to_agent_id":      handoff.ToAgentID,
			"transfer_message": handoff.TransferMessage,
		},
	})
}
