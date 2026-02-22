package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles (function callback pattern, §30) ---

type testBackend struct {
	executeFn func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error)
	cleanupFn func() error
	nameFn    func() string
}

func (b *testBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	if b.executeFn != nil {
		return b.executeFn(ctx, req, config)
	}
	return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0}, nil
}

func (b *testBackend) Cleanup() error {
	if b.cleanupFn != nil {
		return b.cleanupFn()
	}
	return nil
}

func (b *testBackend) Name() string {
	if b.nameFn != nil {
		return b.nameFn()
	}
	return "test"
}

// PLACEHOLDER_EXEC_TESTS

// --- DefaultSandboxConfig ---

func TestDefaultSandboxConfig(t *testing.T) {
	cfg := DefaultSandboxConfig()
	assert.Equal(t, ModeDocker, cfg.Mode)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 512, cfg.MaxMemoryMB)
	assert.Equal(t, 50, cfg.MaxCPUPercent)
	assert.False(t, cfg.NetworkEnabled)
	assert.Equal(t, 1024*1024, cfg.MaxOutputBytes)
	assert.Contains(t, cfg.AllowedLanguages, LangPython)
	assert.Contains(t, cfg.AllowedLanguages, LangJavaScript)
}

// --- NewSandboxExecutor ---

func TestNewSandboxExecutor(t *testing.T) {
	backend := &testBackend{}
	cfg := DefaultSandboxConfig()

	t.Run("nil logger defaults to nop", func(t *testing.T) {
		exec := NewSandboxExecutor(cfg, backend, nil)
		require.NotNil(t, exec)
		assert.NotNil(t, exec.logger)
	})
}

// --- SandboxExecutor.Execute ---

func TestSandboxExecutorExecuteSuccess(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   "hello world\n",
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	result, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-1",
		Language: LangPython,
		Code:     "print('hello world')",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello world\n", result.Stdout)

	stats := exec.Stats()
	assert.Equal(t, int64(1), stats.TotalExecutions)
	assert.Equal(t, int64(1), stats.SuccessExecutions)
	assert.Equal(t, int64(0), stats.FailedExecutions)
}

func TestSandboxExecutorExecuteFailure(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  false,
				ExitCode: 1,
				Stderr:   "error occurred",
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	result, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-2",
		Language: LangPython,
		Code:     "raise Exception('fail')",
	})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.ExitCode)

	stats := exec.Stats()
	assert.Equal(t, int64(1), stats.FailedExecutions)
}

func TestSandboxExecutorExecuteBackendError(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return nil, fmt.Errorf("docker daemon not running")
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	result, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-3",
		Language: LangPython,
		Code:     "print('hi')",
	})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker daemon")

	stats := exec.Stats()
	assert.Equal(t, int64(1), stats.FailedExecutions)
}

// --- Validation ---

func TestSandboxExecutorValidation(t *testing.T) {
	backend := &testBackend{}
	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	t.Run("empty code", func(t *testing.T) {
		_, err := exec.Execute(context.Background(), &ExecutionRequest{
			ID:       "test-empty",
			Language: LangPython,
			Code:     "",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "code is required")
	})

	t.Run("disallowed language", func(t *testing.T) {
		_, err := exec.Execute(context.Background(), &ExecutionRequest{
			ID:       "test-lang",
			Language: LangRust,
			Code:     "fn main() {}",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})
}

// --- Output truncation ---

func TestSandboxExecutorOutputTruncation(t *testing.T) {
	longOutput := make([]byte, 2*1024*1024) // 2MB
	for i := range longOutput {
		longOutput[i] = 'x'
	}

	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   string(longOutput),
				Stderr:   string(longOutput),
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	result, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-trunc",
		Language: LangPython,
		Code:     "print('x' * 2000000)",
	})
	require.NoError(t, err)
	assert.True(t, result.Truncated)
	assert.Equal(t, cfg.MaxOutputBytes, len(result.Stdout))
	assert.Equal(t, cfg.MaxOutputBytes, len(result.Stderr))
}

// --- Timeout uses smaller of config and request ---

func TestSandboxExecutorTimeoutSelection(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			// The timeout should be the request timeout (shorter)
			remaining := time.Until(deadline)
			assert.Less(t, remaining, 2*time.Second)
			return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	cfg.Timeout = 30 * time.Second
	exec := NewSandboxExecutor(cfg, backend, nil)

	_, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-timeout",
		Language: LangPython,
		Code:     "pass",
		Timeout:  1 * time.Second,
	})
	require.NoError(t, err)
}

// --- Cleanup delegates to backend ---

func TestSandboxExecutorCleanup(t *testing.T) {
	cleanupCalled := false
	backend := &testBackend{
		cleanupFn: func() error {
			cleanupCalled = true
			return nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	err := exec.Cleanup()
	require.NoError(t, err)
	assert.True(t, cleanupCalled)
}

// PLACEHOLDER_DOCKER_TESTS

// --- DockerBackend ---

func TestDockerBackendDefaults(t *testing.T) {
	d := NewDockerBackend(nil)
	assert.Equal(t, "docker", d.Name())
	assert.Equal(t, "sandbox_", d.containerPrefix)
	assert.True(t, d.cleanupOnExit)
	assert.Contains(t, d.images, LangPython)
	assert.Contains(t, d.images, LangJavaScript)
}

func TestDockerBackendWithConfig(t *testing.T) {
	d := NewDockerBackendWithConfig(nil, DockerBackendConfig{
		ContainerPrefix: "test_",
		CleanupOnExit:   false,
		CustomImages:    map[Language]string{LangPython: "python:3.11"},
	})
	assert.Equal(t, "test_", d.containerPrefix)
	assert.False(t, d.cleanupOnExit)
	assert.Equal(t, "python:3.11", d.images[LangPython])
	// Other images still have defaults
	assert.Equal(t, "node:20-slim", d.images[LangJavaScript])
}

func TestDockerBackendExecuteNoImage(t *testing.T) {
	d := NewDockerBackend(nil)
	// Remove all images to test "no image" path
	d.images = map[Language]string{}

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-no-img",
		Language: LangPython,
		Code:     "print('hi')",
	}, DefaultSandboxConfig())
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no image configured")
}

func TestDockerBackendBuildCommand(t *testing.T) {
	d := NewDockerBackend(nil)

	tests := []struct {
		lang Language
		code string
		want string
	}{
		{LangPython, "print('hi')", "python3"},
		{LangJavaScript, "console.log('hi')", "node"},
		{LangBash, "echo hi", "sh"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			cmd := d.buildCommand(&ExecutionRequest{Language: tt.lang, Code: tt.code})
			assert.Equal(t, tt.want, cmd[0])
		})
	}
}

func TestDockerBackendBuildDockerArgs(t *testing.T) {
	d := NewDockerBackend(nil)
	cfg := SandboxConfig{
		MaxMemoryMB:   256,
		MaxCPUPercent: 25,
		MountPaths:    map[string]string{"/host": "/container"},
		EnvVars:       map[string]string{"FOO": "bar"},
	}

	args := d.buildDockerArgs("test-container", "python:3.12-slim", &ExecutionRequest{
		Language: LangPython,
		Code:     "pass",
		EnvVars:  map[string]string{"BAZ": "qux"},
	}, cfg)

	assert.Contains(t, args, "--name")
	assert.Contains(t, args, "--memory")
	assert.Contains(t, args, "--network")
	assert.Contains(t, args, "none") // network disabled by default
}

func TestDockerBackendCleanup(t *testing.T) {
	d := NewDockerBackend(nil)
	// No active containers, cleanup should succeed
	err := d.Cleanup()
	require.NoError(t, err)
}

// --- ProcessBackend ---

func TestProcessBackendDefaults(t *testing.T) {
	p := NewProcessBackend(nil)
	assert.Equal(t, "process", p.Name())
	assert.False(t, p.enabled)
}

func TestProcessBackendDisabledByDefault(t *testing.T) {
	p := NewProcessBackend(nil)
	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-disabled",
		Language: LangPython,
		Code:     "print('hi')",
	}, DefaultSandboxConfig())
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "disabled")
}

func TestProcessBackendNoInterpreter(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{Enabled: true})
	// Remove all interpreters
	p.interpreters = map[Language]string{}

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "test-no-interp",
		Language: LangPython,
		Code:     "print('hi')",
	}, DefaultSandboxConfig())
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no interpreter")
}

func TestProcessBackendBuildArgs(t *testing.T) {
	p := NewProcessBackend(nil)

	tests := []struct {
		lang Language
		want string
	}{
		{LangPython, "-c"},
		{LangJavaScript, "-e"},
		{LangBash, "-c"},
		{LangGo, "run"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			args := p.buildArgs(&ExecutionRequest{Language: tt.lang, Code: "test"})
			assert.Equal(t, tt.want, args[0])
		})
	}
}

func TestProcessBackendCleanup(t *testing.T) {
	p := NewProcessBackend(nil)
	err := p.Cleanup()
	require.NoError(t, err)
}

// PLACEHOLDER_VALIDATOR_TESTS

// --- CodeValidator ---

func TestCodeValidator(t *testing.T) {
	v := NewCodeValidator()

	t.Run("Python dangerous patterns", func(t *testing.T) {
		warnings := v.Validate(LangPython, "import os\nos.system('rm -rf /')")
		assert.NotEmpty(t, warnings)
		assert.True(t, len(warnings) >= 2) // "import os" and "os.system"
	})

	t.Run("Python safe code", func(t *testing.T) {
		warnings := v.Validate(LangPython, "x = 1 + 2\nprint(x)")
		assert.Empty(t, warnings)
	})

	t.Run("JavaScript dangerous patterns", func(t *testing.T) {
		warnings := v.Validate(LangJavaScript, `require('child_process').exec('ls')`)
		assert.NotEmpty(t, warnings)
	})

	t.Run("JavaScript safe code", func(t *testing.T) {
		warnings := v.Validate(LangJavaScript, "const x = 1 + 2; console.log(x);")
		assert.Empty(t, warnings)
	})

	t.Run("Bash dangerous patterns", func(t *testing.T) {
		warnings := v.Validate(LangBash, "rm -rf /")
		assert.NotEmpty(t, warnings)
	})

	t.Run("Go dangerous patterns", func(t *testing.T) {
		warnings := v.Validate(LangGo, `import "os/exec"`)
		assert.NotEmpty(t, warnings)
	})

	t.Run("Rust dangerous patterns", func(t *testing.T) {
		warnings := v.Validate(LangRust, "unsafe { std::ptr::null() }")
		assert.NotEmpty(t, warnings)
	})

	t.Run("unknown language returns empty", func(t *testing.T) {
		warnings := v.Validate(Language("lua"), "print('hi')")
		assert.Empty(t, warnings)
	})
}

// --- escapeShellArg ---

func TestEscapeShellArg(t *testing.T) {
	assert.Equal(t, "hello", escapeShellArg("hello"))
	assert.Equal(t, "it'\\''s", escapeShellArg("it's"))
	assert.Equal(t, "", escapeShellArg(""))
}

// --- containsPattern / findPattern ---

func TestContainsPattern(t *testing.T) {
	assert.True(t, containsPattern("import os", "import os"))
	assert.True(t, containsPattern("x = import os; y", "import os"))
	assert.False(t, containsPattern("import o", "import os"))
	assert.False(t, containsPattern("", "import os"))
}

// --- SandboxTool ---

func TestSandboxTool(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{
				ID:       req.ID,
				Success:  true,
				ExitCode: 0,
				Stdout:   "42",
			}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	executor := NewSandboxExecutor(cfg, backend, nil)
	tool := NewSandboxTool(executor, nil)

	t.Run("valid request", func(t *testing.T) {
		args, _ := json.Marshal(ExecutionRequest{
			ID:       "tool-1",
			Language: LangPython,
			Code:     "print(42)",
		})

		result, err := tool.Execute(context.Background(), args)
		require.NoError(t, err)

		var execResult ExecutionResult
		err = json.Unmarshal(result, &execResult)
		require.NoError(t, err)
		assert.True(t, execResult.Success)
		assert.Equal(t, "42", execResult.Stdout)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid arguments")
	})
}

// --- sanitizeID (docker_exec.go) ---

func TestSanitizeID(t *testing.T) {
	assert.Equal(t, "abc123", sanitizeID("abc123"))
	assert.Equal(t, "abc_def-123", sanitizeID("abc_def-123"))
	assert.Equal(t, "abcdef", sanitizeID("abc!@#def"))
	// Truncation to 32 chars
	long := "abcdefghijklmnopqrstuvwxyz1234567890"
	assert.Equal(t, 32, len(sanitizeID(long)))
}

// --- execCommand mock ---

func TestExecCommand(t *testing.T) {
	cmd := execCommandContext(context.Background(), "echo", "hello")
	cmd.SetStdin("input")
	stdout, stderr, err := cmd.Run()
	require.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
	assert.Equal(t, 0, cmd.ExitCode())
}

// --- Stats concurrency ---

func TestSandboxExecutorStatsConcurrency(t *testing.T) {
	backend := &testBackend{}
	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	done := make(chan struct{})
	const n = 50

	for i := 0; i < n; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			exec.Execute(context.Background(), &ExecutionRequest{
				ID:       fmt.Sprintf("concurrent-%d", i),
				Language: LangPython,
				Code:     "pass",
			})
		}()
	}

	for i := 0; i < n; i++ {
		<-done
	}

	stats := exec.Stats()
	assert.Equal(t, int64(n), stats.TotalExecutions)
}
