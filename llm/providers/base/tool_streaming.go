package providerbase

import (
	"encoding/json"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

type ToolCallDeltaAccumulator struct {
	names    map[string]string
	callIDs  map[string]string
	payloads map[string]json.RawMessage
}

func NewToolCallDeltaAccumulator() *ToolCallDeltaAccumulator {
	return &ToolCallDeltaAccumulator{
		names:    make(map[string]string),
		callIDs:  make(map[string]string),
		payloads: make(map[string]json.RawMessage),
	}
}

func (a *ToolCallDeltaAccumulator) Register(itemID, toolType, name, callID string) {
	if a == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	if strings.TrimSpace(name) != "" {
		a.names[itemID] = strings.TrimSpace(name)
	}
	if strings.TrimSpace(callID) != "" {
		a.callIDs[itemID] = strings.TrimSpace(callID)
	}
}

func (a *ToolCallDeltaAccumulator) Append(itemID, delta string) {
	if a == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	a.payloads[itemID] = AppendToolJSONDelta(a.payloads[itemID], delta)
}

func (a *ToolCallDeltaAccumulator) CompleteFunction(itemID string) (types.ToolCall, bool) {
	if a == nil || strings.TrimSpace(itemID) == "" {
		return types.ToolCall{}, false
	}
	callID := a.callIDs[itemID]
	if callID == "" {
		callID = itemID
	}
	name := a.names[itemID]
	payload := a.payloads[itemID]
	a.delete(itemID)
	if strings.TrimSpace(name) == "" {
		return types.ToolCall{}, false
	}
	return NewFunctionToolCall(callID, name, payload), true
}

func (a *ToolCallDeltaAccumulator) CompleteCustom(itemID string) (types.ToolCall, bool) {
	if a == nil || strings.TrimSpace(itemID) == "" {
		return types.ToolCall{}, false
	}
	callID := a.callIDs[itemID]
	if callID == "" {
		callID = itemID
	}
	name := a.names[itemID]
	input := string(a.payloads[itemID])
	a.delete(itemID)
	if strings.TrimSpace(name) == "" {
		return types.ToolCall{}, false
	}
	return NewCustomToolCall(callID, name, input), true
}

func (a *ToolCallDeltaAccumulator) delete(itemID string) {
	delete(a.names, itemID)
	delete(a.callIDs, itemID)
	delete(a.payloads, itemID)
}

type ToolOutputWriteback struct {
	ToolType string
	CallID   string
	Name     string
	Content  string
	IsError  bool
}

func ToolOutputFromMessage(msg types.Message, toolCallTypes map[string]string) (ToolOutputWriteback, bool) {
	callID := strings.TrimSpace(msg.ToolCallID)
	if callID == "" {
		return ToolOutputWriteback{}, false
	}
	return ToolOutputWriteback{
		ToolType: NormalizeToolType(toolCallTypes[msg.ToolCallID]),
		CallID:   callID,
		Name:     strings.TrimSpace(msg.Name),
		Content:  msg.Content,
		IsError:  msg.IsToolError,
	}, true
}

func BuildOpenAIResponsesToolOutputItem(writeback ToolOutputWriteback, idMapper func(string) string) map[string]any {
	callID := writeback.CallID
	if idMapper != nil {
		callID = idMapper(callID)
	}
	switch NormalizeToolType(writeback.ToolType) {
	case types.ToolTypeCustom:
		return map[string]any{
			"type":    "custom_tool_call_output",
			"call_id": callID,
			"output":  writeback.Content,
		}
	default:
		return map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  writeback.Content,
		}
	}
}

func BuildAnthropicToolResultBlock(writeback ToolOutputWriteback) map[string]any {
	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": writeback.CallID,
		"content":     writeback.Content,
	}
	if writeback.IsError {
		block["is_error"] = true
	}
	return block
}

func BuildGeminiFunctionResponse(writeback ToolOutputWriteback) map[string]any {
	return ToolOutputResponseMap(writeback.Content)
}
