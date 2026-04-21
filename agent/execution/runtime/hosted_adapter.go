package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/hosted"
	"go.uber.org/zap"
)

// HostedAdapter adapts SandboxExecutor to the hosted.CodeExecutor interface.
//
// Usage:
//
//	sandbox := execution.NewSandboxExecutor(cfg, backend, logger)
//	adapter := execution.NewHostedAdapter(sandbox, logger)
//	tool := hosted.NewCodeExecTool(hosted.CodeExecConfig{Executor: adapter})
type HostedAdapter struct {
	executor *SandboxExecutor
	logger   *zap.Logger
}

// Compile-time check: HostedAdapter implements hosted.CodeExecutor.
var _ hosted.CodeExecutor = (*HostedAdapter)(nil)

// NewHostedAdapter creates a HostedAdapter wrapping the given SandboxExecutor.
func NewHostedAdapter(executor *SandboxExecutor, logger *zap.Logger) *HostedAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HostedAdapter{executor: executor, logger: logger}
}

// Execute satisfies the hosted.CodeExecutor interface.
func (a *HostedAdapter) Execute(ctx context.Context, language string, code string, timeout time.Duration) (*hosted.CodeExecOutput, error) {
	lang, ok := mapLanguage(language)
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	req := &ExecutionRequest{
		ID:       fmt.Sprintf("hosted_%d", time.Now().UnixNano()),
		Language: lang,
		Code:     code,
		Timeout:  timeout,
	}

	result, err := a.executor.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sandbox execution failed: %w", err)
	}

	return &hosted.CodeExecOutput{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Duration: result.Duration,
	}, nil
}

// mapLanguage converts a string language name to the execution.Language type.
func mapLanguage(lang string) (Language, bool) {
	switch lang {
	case "python":
		return LangPython, true
	case "javascript":
		return LangJavaScript, true
	case "typescript":
		return LangTypeScript, true
	case "go":
		return LangGo, true
	case "rust":
		return LangRust, true
	case "bash":
		return LangBash, true
	default:
		return "", false
	}
}
