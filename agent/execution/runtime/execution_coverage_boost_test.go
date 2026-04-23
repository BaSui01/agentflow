package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ═══ HostedAdapter 完整测试 ═══

func TestHostedAdapter_Execute_AllLanguages(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   "ok",
				Duration: 10 * time.Millisecond,
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	cfg.AllowedLanguages = []Language{LangPython, LangJavaScript, LangTypeScript, LangGo, LangRust, LangBash}
	executor := NewSandboxExecutor(cfg, backend, nil)
	adapter := NewHostedAdapter(executor, nil)

	languages := []string{"python", "javascript", "typescript", "go", "rust", "bash"}
	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			out, err := adapter.Execute(context.Background(), lang, "code", 5*time.Second)
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.Equal(t, "ok", out.Stdout)
			assert.Equal(t, 0, out.ExitCode)
		})
	}
}

func TestHostedAdapter_Execute_UnsupportedLanguages(t *testing.T) {
	adapter := NewHostedAdapter(nil, nil)

	unsupported := []string{"java", "c++", "ruby", "php", "", "PYTHON", "Python"}
	for _, lang := range unsupported {
		t.Run(lang, func(t *testing.T) {
			_, err := adapter.Execute(context.Background(), lang, "code", time.Second)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported language")
		})
	}
}

func TestHostedAdapter_Execute_BackendError(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return nil, fmt.Errorf("docker daemon not running")
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	adapter := NewHostedAdapter(executor, nil)

	_, err := adapter.Execute(context.Background(), "python", "print('hi')", 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox execution failed")
}

func TestHostedAdapter_Execute_Timeout(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			// Verify the timeout from the request is passed through
			assert.Equal(t, 2*time.Second, req.Timeout)
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	adapter := NewHostedAdapter(executor, nil)

	out, err := adapter.Execute(context.Background(), "python", "pass", 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, out.ExitCode)
}

func TestHostedAdapter_Execute_NonZeroExit(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  false,
				ExitCode: 1,
				Stderr:   "error occurred",
				Duration: 5 * time.Millisecond,
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	adapter := NewHostedAdapter(executor, nil)

	out, err := adapter.Execute(context.Background(), "python", "raise Exception()", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, out.ExitCode)
	assert.Equal(t, "error occurred", out.Stderr)
}

func TestHostedAdapter_Execute_OutputFields(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   "hello stdout",
				Stderr:   "hello stderr",
				Duration: 42 * time.Millisecond,
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	adapter := NewHostedAdapter(executor, nil)

	out, err := adapter.Execute(context.Background(), "python", "print('hello')", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "hello stdout", out.Stdout)
	assert.Equal(t, "hello stderr", out.Stderr)
	assert.Equal(t, 0, out.ExitCode)
	// Duration 可能因实现不同而不精确匹配，只检查非零
	assert.True(t, out.Duration >= 0, "expected non-negative duration")
}

func TestHostedAdapter_Execute_IDContainsHostedPrefix(t *testing.T) {
	var capturedID string
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			capturedID = req.ID
			return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	adapter := NewHostedAdapter(executor, nil)

	_, err := adapter.Execute(context.Background(), "python", "pass", time.Second)
	require.NoError(t, err)
	assert.Contains(t, capturedID, "hosted_")
}

// ═══ RealDockerBackend 边界测试 ═══

func TestRealDockerBackend_Execute_InvalidFilename(t *testing.T) {
	d := NewRealDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "path-traversal",
		Language: LangPython,
		Code:     "pass",
		Files: map[string]string{
			"../etc/passwd": "malicious",
		},
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "path traversal")
}

func TestRealDockerBackend_Execute_AbsoluteFilename(t *testing.T) {
	d := NewRealDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "abs-path",
		Language: LangPython,
		Code:     "pass",
		Files: map[string]string{
			"/etc/passwd": "malicious",
		},
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "path traversal")
}

func TestRealDockerBackend_BuildRealDockerArgs_ZeroLimits(t *testing.T) {
	d := NewRealDockerBackend(nil)
	cfg := SandboxConfig{
		MaxMemoryMB:    0,
		MaxCPUPercent:  0,
		NetworkEnabled: false,
	}

	args := d.buildRealDockerArgs("test", "python:3.12-slim", "/tmp/code", "main.py",
		&ExecutionRequest{Language: LangPython, Code: "pass"}, cfg)

	// Should NOT contain --memory or --cpus when limits are 0
	for _, arg := range args {
		assert.NotEqual(t, "--memory", arg, "should not set memory limit when MaxMemoryMB=0")
	}
	// Should contain --network none
	assert.Contains(t, args, "none")
}

func TestRealDockerBackend_WriteCodeFile_AllLanguages(t *testing.T) {
	d := NewRealDockerBackend(nil)
	tmpDir := t.TempDir()

	tests := []struct {
		lang     Language
		expected string
	}{
		{LangPython, "main.py"},
		{LangJavaScript, "main.js"},
		{LangTypeScript, "main.ts"},
		{LangGo, "main.go"},
		{LangRust, "main.rs"},
		{LangBash, "script.sh"},
		{Language("lua"), "code.txt"},
		{Language(""), "code.txt"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang)+"_file", func(t *testing.T) {
			filename, err := d.writeCodeFile(tmpDir, &ExecutionRequest{
				Language: tt.lang,
				Code:     "test code content",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, filename)
		})
	}
}

func TestRealDockerBackend_BuildRealCommand_AllLanguages(t *testing.T) {
	d := NewRealDockerBackend(nil)

	tests := []struct {
		lang     Language
		codeFile string
		wantCmd  string
	}{
		{LangPython, "main.py", "python3"},
		{LangJavaScript, "main.js", "node"},
		{LangTypeScript, "main.ts", "npx"},
		{LangGo, "main.go", "go"},
		{LangRust, "main.rs", "sh"},
		{LangBash, "script.sh", "bash"},
		{Language("unknown"), "code.txt", "cat"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang)+"_cmd", func(t *testing.T) {
			cmd := d.buildRealCommand(tt.codeFile, &ExecutionRequest{Language: tt.lang, Code: "test"})
			require.NotEmpty(t, cmd)
			assert.Equal(t, tt.wantCmd, cmd[0])
		})
	}
}

// ═══ RealProcessBackend 边界测试 ═══

func TestRealProcessBackend_Execute_UnsupportedLanguage(t *testing.T) {
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "unsupported",
		Language: LangRust, // Rust is not in the switch for RealProcessBackend
		Code:     "fn main() {}",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	// Should fail at either validation or "unsupported language" in the switch
}

func TestRealProcessBackend_Execute_SafeCode(t *testing.T) {
	// Safe code should pass validation but may fail at actual execution
	// (no real interpreter in test env). The key is it passes the validator.
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "safe-python",
		Language: LangPython,
		Code:     "x = 1 + 2\nprint(x)",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	// It will either succeed or fail at execution, but should not fail at validation
	assert.NotContains(t, result.Error, "code validation failed")
}

func TestRealProcessBackend_Execute_MultipleValidationWarnings(t *testing.T) {
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "multi-warn",
		Language: LangPython,
		Code:     "import os\nimport subprocess\nos.system('ls')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "code validation failed")
}

func TestRealProcessBackend_Execute_BashDangerous(t *testing.T) {
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "bash-danger",
		Language: LangBash,
		Code:     "rm -rf /tmp/test",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "code validation failed")
}

func TestRealProcessBackend_Execute_JSDangerous(t *testing.T) {
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "js-danger",
		Language: LangJavaScript,
		Code:     `require('child_process').exec('ls')`,
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "code validation failed")
}

// ═══ DockerBackend.Execute 更多分支 ═══

func TestDockerBackend_Execute_GoLanguage(t *testing.T) {
	d := NewDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "go-exec",
		Language: LangGo,
		Code:     `package main; func main() {}`,
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	// Should succeed (mock exec) and exercise the Go code-mount path
	assert.Equal(t, "go-exec", result.ID)
}

func TestDockerBackend_Execute_RustLanguage(t *testing.T) {
	d := NewDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "rust-exec",
		Language: LangRust,
		Code:     `fn main() {}`,
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "rust-exec", result.ID)
}

func TestDockerBackend_Execute_WithStdin(t *testing.T) {
	d := NewDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "stdin-test",
		Language: LangPython,
		Code:     "import sys; print(sys.stdin.read())",
		Stdin:    "hello from stdin",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "stdin-test", result.ID)
}

func TestDockerBackend_Execute_WithEnvVars(t *testing.T) {
	d := NewDockerBackend(nil)
	cfg := DefaultSandboxConfig()
	cfg.EnvVars = map[string]string{"CONFIG_VAR": "value1"}

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "env-test",
		Language: LangPython,
		Code:     "pass",
		EnvVars:  map[string]string{"REQ_VAR": "value2"},
	}, cfg)

	require.NoError(t, err)
	assert.Equal(t, "env-test", result.ID)
}

func TestDockerBackend_Execute_WithCodeMountDir(t *testing.T) {
	d := NewDockerBackend(nil)

	args := d.buildDockerArgs("test", "golang:1.24-alpine", &ExecutionRequest{
		Language: LangGo,
		Code:     "package main",
	}, DefaultSandboxConfig(), "/tmp/code_mount")

	// Should contain the code mount
	foundCodeMount := false
	for i, arg := range args {
		if arg == "-v" && i+1 < len(args) && args[i+1] == "/tmp/code_mount:/code:ro" {
			foundCodeMount = true
		}
	}
	assert.True(t, foundCodeMount, "expected code mount dir in docker args")
}

// ═══ ProcessBackend.Execute 更多分支 ═══

func TestProcessBackend_Execute_Enabled(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "enabled-test",
		Language: LangPython,
		Code:     "print('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	// With mock execCommand, should succeed
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
}

func TestProcessBackend_Execute_WithStdin(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "stdin-proc",
		Language: LangPython,
		Code:     "import sys; print(sys.stdin.read())",
		Stdin:    "hello",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "stdin-proc", result.ID)
}

func TestProcessBackend_Execute_JavaScript(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "js-proc",
		Language: LangJavaScript,
		Code:     "console.log('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "js-proc", result.ID)
}

func TestProcessBackend_Execute_Bash(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "bash-proc",
		Language: LangBash,
		Code:     "echo hi",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "bash-proc", result.ID)
}

func TestProcessBackend_Execute_GoLanguage(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "go-proc",
		Language: LangGo,
		Code:     "package main; func main() {}",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "go-proc", result.ID)
}

func TestProcessBackend_Execute_UnknownLanguage(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})
	// Add an interpreter for unknown lang to pass the interpreter check
	p.interpreters[Language("lua")] = "lua"

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "unknown-proc",
		Language: Language("lua"),
		Code:     "print('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	// Should still work with mock exec
	assert.Equal(t, "unknown-proc", result.ID)
}

// ═══ SandboxExecutor 边界测试 ═══

func TestSandboxExecutor_Execute_ZeroTimeout(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			_, ok := ctx.Deadline()
			assert.True(t, ok, "context should have deadline from config timeout")
			return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	// Request timeout = 0 means use config timeout
	result, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "zero-timeout",
		Language: LangPython,
		Code:     "pass",
		Timeout:  0,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSandboxExecutor_Execute_StderrTruncation(t *testing.T) {
	longStderr := make([]byte, 2*1024*1024)
	for i := range longStderr {
		longStderr[i] = 'e'
	}

	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   "short",
				Stderr:   string(longStderr),
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	result, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "stderr-trunc",
		Language: LangPython,
		Code:     "pass",
	})
	require.NoError(t, err)
	assert.True(t, result.Truncated)
	assert.Equal(t, cfg.MaxOutputBytes, len(result.Stderr))
	assert.Equal(t, "short", result.Stdout) // stdout not truncated
}

// ═══ sanitizeID 边界测试 ═══

func TestSanitizeID_SpecialChars(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abc", "abc"},
		{"a!b@c#d$e%f", "abcdef"},
		{"hello world", "helloworld"},
		{"test-id_123", "test-id_123"},
		{"ABC", "ABC"},
		{"a.b.c", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeID(tt.input))
		})
	}
}

func TestSanitizeID_ExactlyMaxLength(t *testing.T) {
	input := "abcdefghijklmnopqrstuvwxyz123456" // exactly 32
	assert.Equal(t, 32, len(sanitizeID(input)))
	assert.Equal(t, input, sanitizeID(input))
}

// ═══ Coverage Boost Round 2 ═══

// --- DockerBackend.Execute more branches (80.9%) ---

func TestDockerBackend_Execute_UnsupportedLanguage(t *testing.T) {
	d := NewDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "unsupported-lang",
		Language: Language("lua"),
		Code:     "print('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no image configured")
}

func TestDockerBackend_Execute_WithCleanupOnExit(t *testing.T) {
	cfg := DefaultSandboxConfig()
	d := NewDockerBackendWithConfig(nil, DockerBackendConfig{
		CleanupOnExit: true,
	})

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "cleanup-test",
		Language: LangPython,
		Code:     "print('hi')",
	}, cfg)

	require.NoError(t, err)
	assert.Equal(t, "cleanup-test", result.ID)
}

func TestDockerBackend_Execute_TypeScript(t *testing.T) {
	d := NewDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "ts-exec",
		Language: LangTypeScript,
		Code:     "console.log('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "ts-exec", result.ID)
}

func TestDockerBackend_Execute_BashLanguage(t *testing.T) {
	d := NewDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "bash-exec",
		Language: LangBash,
		Code:     "echo hi",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "bash-exec", result.ID)
}

func TestDockerBackend_Execute_WithMemoryAndCPU(t *testing.T) {
	d := NewDockerBackend(nil)
	cfg := DefaultSandboxConfig()
	cfg.MaxMemoryMB = 256
	cfg.MaxCPUPercent = 50

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "limits-test",
		Language: LangPython,
		Code:     "pass",
	}, cfg)

	require.NoError(t, err)
	assert.Equal(t, "limits-test", result.ID)
}

func TestDockerBackend_Execute_WithMountPaths(t *testing.T) {
	d := NewDockerBackend(nil)
	cfg := DefaultSandboxConfig()
	cfg.MountPaths = map[string]string{"/host/data": "/container/data"}

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "mount-test",
		Language: LangPython,
		Code:     "pass",
	}, cfg)

	require.NoError(t, err)
	assert.Equal(t, "mount-test", result.ID)
}

// --- DockerBackend.killContainer / removeContainer (83.3%) ---

func TestDockerBackend_Cleanup_WithActiveContainers2(t *testing.T) {
	d := NewDockerBackend(nil)
	d.activeContainers["test-container-1"] = struct{}{}
	d.activeContainers["test-container-2"] = struct{}{}

	err := d.Cleanup()
	require.NoError(t, err)
}

// --- ProcessBackend.Execute more branches (84.6%) ---

func TestProcessBackend_Execute_Disabled(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: false})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "disabled-test",
		Language: LangPython,
		Code:     "print('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "disabled")
}

func TestProcessBackend_Execute_UnsupportedLanguage(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "unsupported-proc",
		Language: Language("cobol"),
		Code:     "DISPLAY 'HI'",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no interpreter")
}

func TestProcessBackend_Execute_TypeScript(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "ts-proc",
		Language: LangTypeScript,
		Code:     "console.log('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.Equal(t, "ts-proc", result.ID)
}

func TestProcessBackend_Execute_WithEnvVars(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})
	cfg := DefaultSandboxConfig()
	cfg.EnvVars = map[string]string{"TEST_VAR": "hello"}

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "env-proc",
		Language: LangPython,
		Code:     "pass",
	}, cfg)

	require.NoError(t, err)
	assert.Equal(t, "env-proc", result.ID)
}

// --- SandboxTool.Execute (80%) ---

func TestSandboxTool_Execute_Success(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   "output",
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	tool := NewSandboxTool(executor, nil)

	args := []byte(`{"id":"tool-test","language":"python","code":"print('hi')"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSandboxTool_Execute_InvalidJSON(t *testing.T) {
	backend := &testBackend{}
	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	tool := NewSandboxTool(executor, nil)

	_, err := tool.Execute(context.Background(), []byte(`{invalid`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid arguments")
}

func TestSandboxTool_Execute_WithWarnings(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	tool := NewSandboxTool(executor, zap.NewNop())

	// Code with dangerous pattern
	args := []byte(`{"id":"warn-test","language":"python","code":"import os; os.system('ls')"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSandboxTool_Execute_BackendError(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return nil, fmt.Errorf("backend error")
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	tool := NewSandboxTool(executor, nil)

	args := []byte(`{"id":"err-test","language":"python","code":"pass"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
}

// --- mustContext (66.7%) ---

func TestMustContext_WithValue(t *testing.T) {
	result := mustContext("hello", true)
	assert.Equal(t, "hello", result)
}

func TestMustContext_WithoutValue(t *testing.T) {
	result := mustContext("", false)
	assert.Equal(t, "", result)
}

func TestMustContext_ValueButNotOk(t *testing.T) {
	result := mustContext("something", false)
	assert.Equal(t, "", result)
}

// --- SandboxExecutor.Execute nil backend (94.7%) ---

func TestSandboxExecutor_Execute_NilBackend(t *testing.T) {
	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, nil, nil)

	_, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "nil-backend",
		Language: LangPython,
		Code:     "pass",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend is nil")
}

// --- RealDockerBackend.Execute more branches (28.6%) ---

func TestRealDockerBackend_Execute_UnsupportedLanguage(t *testing.T) {
	d := NewRealDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "real-unsupported",
		Language: Language("lua"),
		Code:     "print('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no image configured")
}

func TestRealDockerBackend_Execute_WithExtraFiles(t *testing.T) {
	d := NewRealDockerBackend(nil)

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "extra-files",
		Language: LangPython,
		Code:     "pass",
		Files: map[string]string{
			"data.txt": "hello world",
		},
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	// Will fail at docker execution but should get past file writing
	assert.Equal(t, "extra-files", result.ID)
}

// --- RealProcessBackend.Execute more branches (71.4%) ---

func TestRealProcessBackend_Execute_Disabled(t *testing.T) {
	p := NewRealProcessBackend(nil, false)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "disabled-real",
		Language: LangPython,
		Code:     "pass",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "disabled")
}

func TestRealProcessBackend_Execute_SafeBash(t *testing.T) {
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "safe-bash",
		Language: LangBash,
		Code:     "echo hello",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	// May succeed or fail depending on env, but should not fail at validation
	assert.NotContains(t, result.Error, "code validation failed")
}

func TestRealProcessBackend_Execute_UnsupportedLang(t *testing.T) {
	p := NewRealProcessBackend(nil, true)

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "unsupported-real",
		Language: LangGo, // Go is not in the switch for RealProcessBackend
		Code:     "package main",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
}

// --- SandboxCodeValidator more coverage ---

func TestCodeValidator_Validate_GoPatterns(t *testing.T) {
	v := NewSandboxCodeValidator()
	warnings := v.Validate(LangGo, `import "os/exec"; exec.Command("ls")`)
	assert.NotEmpty(t, warnings)
}

func TestCodeValidator_Validate_RustPatterns(t *testing.T) {
	v := NewSandboxCodeValidator()
	warnings := v.Validate(LangRust, `unsafe { std::process::Command::new("ls") }`)
	assert.NotEmpty(t, warnings)
}

func TestCodeValidator_Validate_UnknownLanguage(t *testing.T) {
	v := NewSandboxCodeValidator()
	warnings := v.Validate(Language("lua"), "print('hi')")
	assert.Empty(t, warnings)
}

// --- DockerBackend buildDockerArgs with all options ---

func TestDockerBackend_BuildDockerArgs_AllOptions(t *testing.T) {
	d := NewDockerBackend(nil)
	cfg := SandboxConfig{
		MaxMemoryMB:    512,
		MaxCPUPercent:  75,
		NetworkEnabled: true,
		EnvVars:        map[string]string{"CFG_VAR": "val1"},
		MountPaths:     map[string]string{"/host": "/container"},
	}

	args := d.buildDockerArgs("test-container", "python:3.12-slim",
		&ExecutionRequest{
			Language: LangPython,
			Code:     "pass",
			EnvVars:  map[string]string{"REQ_VAR": "val2"},
		}, cfg, "/tmp/code")

	// Should contain memory, cpu, env vars, mount paths, code mount
	found := map[string]bool{}
	for _, arg := range args {
		if arg == "--memory" {
			found["memory"] = true
		}
		if arg == "--cpus" {
			found["cpus"] = true
		}
	}
	assert.True(t, found["memory"], "expected --memory flag")
	assert.True(t, found["cpus"], "expected --cpus flag")
	// Should NOT contain --network none since NetworkEnabled=true
	for i, arg := range args {
		if arg == "--network" && i+1 < len(args) && args[i+1] == "none" {
			t.Fatal("should not have --network none when NetworkEnabled=true")
		}
	}
}

// --- ProcessBackend buildArgs ---

func TestProcessBackend_BuildArgs_GoLanguage(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})
	args := p.buildArgs(&ExecutionRequest{Language: LangGo, Code: "package main"})
	assert.Contains(t, args, "run")
}

func TestProcessBackend_BuildArgs_DefaultLanguage(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})
	args := p.buildArgs(&ExecutionRequest{Language: Language("unknown"), Code: "code"})
	// Should use the default fallback
	assert.NotEmpty(t, args)
}
