package types

import "encoding/json"

const ToolResultControlTypeHandoff = "handoff"

// ToolResultControl carries runtime control signals encoded in ToolResult payloads.
// It lets upper layers terminate or redirect a tool loop without depending on agent packages.
type ToolResultControl struct {
	Type    string             `json:"type"`
	Handoff *ToolResultHandoff `json:"handoff,omitempty"`
}

// ToolResultHandoff describes a completed handoff that should become the next active run output.
type ToolResultHandoff struct {
	HandoffID        string         `json:"handoff_id,omitempty"`
	FromAgentID      string         `json:"from_agent_id,omitempty"`
	ToAgentID        string         `json:"to_agent_id,omitempty"`
	ToAgentName      string         `json:"to_agent_name,omitempty"`
	TransferMessage  string         `json:"transfer_message,omitempty"`
	Output           string         `json:"output,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	TokensUsed       int            `json:"tokens_used,omitempty"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	ReasoningContent *string        `json:"reasoning_content,omitempty"`
}

// Control decodes an optional runtime control payload from a tool result.
func (tr ToolResult) Control() *ToolResultControl {
	if len(tr.Result) == 0 {
		return nil
	}
	var control ToolResultControl
	if err := json.Unmarshal(tr.Result, &control); err != nil {
		return nil
	}
	if control.Type == "" {
		return nil
	}
	return &control
}
