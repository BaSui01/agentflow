package hosted

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// CodeExecutor is the interface for sandboxed code execution.
// This is a local interface following the workflow-local interface pattern (§15)
// to avoid importing agent/execution directly.
type CodeExecutor interface {
	Execute(ctx context.Context, language string, code string, timeout time.Duration) (*CodeExecOutput, error)
}

// CodeExecOutput holds the result of a code execution.
type CodeExecOutput struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// CodeExecTool provides sandboxed code execution as a hosted tool.
type CodeExecTool struct {
	executor CodeExecutor
	timeout  time.Duration
	logger   *zap.Logger
}

// CodeExecConfig configures the code execution tool.
type CodeExecConfig struct {
	Executor CodeExecutor
	Timeout  time.Duration
	Logger   *zap.Logger
}

const (
	defaultCodeExecTimeout = 30 * time.Second
	maxCodeExecTimeout     = 300 * time.Second
)

// NewCodeExecTool creates a new code execution tool.
func NewCodeExecTool(config CodeExecConfig) *CodeExecTool {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultCodeExecTimeout
	}
	if timeout > maxCodeExecTimeout {
		timeout = maxCodeExecTimeout
	}
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CodeExecTool{
		executor: config.Executor,
		timeout:  timeout,
		logger:   logger.With(zap.String("tool", "code_execution")),
	}
}

func (t *CodeExecTool) Type() HostedToolType { return ToolTypeCodeExec }
func (t *CodeExecTool) Name() string         { return "code_execution" }
func (t *CodeExecTool) Description() string {
	return "Execute code in a sandboxed environment"
}

func (t *CodeExecTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language": map[string]any{
				"type":        "string",
				"enum":        []string{"python", "javascript", "bash", "go"},
				"description": "Programming language to execute",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Code to execute",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Execution timeout in seconds (default 30, max 300)",
			},
		},
		"required": []string{"language", "code"},
	})
	if err != nil {
		params = []byte("{}")
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

// codeExecArgs represents the arguments for code execution.
type codeExecArgs struct {
	Language       string `json:"language"`
	Code           string `json:"code"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

var validLanguages = map[string]bool{
	"python":     true,
	"javascript": true,
	"bash":       true,
	"go":         true,
}

func (t *CodeExecTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var execArgs codeExecArgs
	if err := json.Unmarshal(args, &execArgs); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if execArgs.Code == "" {
		return nil, fmt.Errorf("code is required")
	}
	if !validLanguages[execArgs.Language] {
		return nil, fmt.Errorf("unsupported language: %s", execArgs.Language)
	}

	timeout := t.timeout
	if execArgs.TimeoutSeconds > 0 {
		timeout = time.Duration(execArgs.TimeoutSeconds) * time.Second
		if timeout > maxCodeExecTimeout {
			timeout = maxCodeExecTimeout
		}
	}

	t.logger.Debug("executing code",
		zap.String("language", execArgs.Language),
		zap.Duration("timeout", timeout),
	)

	output, err := t.executor.Execute(ctx, execArgs.Language, execArgs.Code, timeout)
	if err != nil {
		return nil, fmt.Errorf("code execution failed: %w", err)
	}

	return json.Marshal(output)
}


