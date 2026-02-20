package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/tools"
	"github.com/BaSui01/agentflow/types"
)

// AgentToolConfig configures how an Agent is exposed as a tool.
type AgentToolConfig struct {
	// Name overrides the default tool name (default: "agent_<agent.Name()>").
	Name string

	// Description overrides the agent's description in the tool schema.
	Description string

	// Timeout limits the agent execution time. Zero means no extra timeout.
	Timeout time.Duration
}

// AgentTool wraps an Agent instance as a callable tool, enabling lightweight
// agent-to-agent delegation via the standard tool-calling interface.
type AgentTool struct {
	agent  Agent
	config AgentToolConfig
	name   string
}

// NewAgentTool creates an AgentTool that wraps the given Agent.
// If config is nil, defaults are used.
func NewAgentTool(agent Agent, config *AgentToolConfig) *AgentTool {
	cfg := AgentToolConfig{}
	if config != nil {
		cfg = *config
	}

	name := cfg.Name
	if name == "" {
		name = "agent_" + agent.Name()
	}

	return &AgentTool{
		agent:  agent,
		config: cfg,
		name:   name,
	}
}

// agentToolArgs is the JSON schema expected in ToolCall.Arguments.
type agentToolArgs struct {
	Input     string            `json:"input"`
	Context   map[string]any    `json:"context,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// Schema returns the ToolSchema describing this agent-as-tool.
func (at *AgentTool) Schema() types.ToolSchema {
	desc := at.config.Description
	if desc == "" {
		desc = fmt.Sprintf("Delegate a task to the %q agent", at.agent.Name())
	}

	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"input": {
				"type": "string",
				"description": "The task or query to send to the agent"
			},
			"context": {
				"type": "object",
				"description": "Optional context key-value pairs"
			},
			"variables": {
				"type": "object",
				"description": "Optional variable substitutions",
				"additionalProperties": {"type": "string"}
			}
		},
		"required": ["input"]
	}`)

	return types.ToolSchema{
		Name:        at.name,
		Description: desc,
		Parameters:  params,
	}
}

// Execute handles a ToolCall by delegating to the wrapped Agent.
func (at *AgentTool) Execute(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	start := time.Now()
	result := llmtools.ToolResult{
		ToolCallID: call.ID,
		Name:       at.name,
	}

	// Parse arguments
	var args agentToolArgs
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			result.Error = fmt.Sprintf("invalid arguments: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}
	if args.Input == "" {
		result.Error = "missing required field: input"
		result.Duration = time.Since(start)
		return result
	}

	// Apply timeout if configured
	execCtx := ctx
	if at.config.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, at.config.Timeout)
		defer cancel()
	}

	// Build agent Input
	input := &Input{
		Content:   args.Input,
		Context:   args.Context,
		Variables: args.Variables,
	}

	// Execute the agent
	output, err := at.agent.Execute(execCtx, input)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	// Marshal the output content as the tool result
	resultJSON, err := json.Marshal(map[string]any{
		"content":       output.Content,
		"tokens_used":   output.TokensUsed,
		"duration":      output.Duration.String(),
		"finish_reason": output.FinishReason,
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output: %s", err.Error())
		result.Duration = time.Since(start)
		return result
	}

	result.Result = resultJSON
	result.Duration = time.Since(start)
	return result
}

// Name returns the tool name.
func (at *AgentTool) Name() string {
	return at.name
}

// Agent returns the underlying Agent instance.
func (at *AgentTool) Agent() Agent {
	return at.agent
}
