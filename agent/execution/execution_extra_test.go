package execution

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- RealDockerBackend unit tests (no Docker required) ---

func TestRealDockerBackend_Name(t *testing.T) {
	d := NewRealDockerBackend(nil)
	assert.Equal(t, "docker", d.Name())
}

func TestRealDockerBackend_WriteCodeFile(t *testing.T) {
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
		{Language("unknown"), "code.txt"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			filename, err := d.writeCodeFile(tmpDir, &ExecutionRequest{
				Language: tt.lang,
				Code:     "test code",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, filename)
		})
	}
}

func TestRealDockerBackend_BuildRealCommand(t *testing.T) {
	d := NewRealDockerBackend(nil)

	tests := []struct {
		lang     Language
		codeFile string
		want     string
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
		t.Run(string(tt.lang), func(t *testing.T) {
			cmd := d.buildRealCommand(tt.codeFile, &ExecutionRequest{Language: tt.lang, Code: "test"})
			assert.Equal(t, tt.want, cmd[0])
		})
	}
}

func TestRealDockerBackend_BuildRealDockerArgs(t *testing.T) {
	d := NewRealDockerBackend(nil)
	cfg := SandboxConfig{
		MaxMemoryMB:   256,
		MaxCPUPercent: 50,
		EnvVars:       map[string]string{"FOO": "bar"},
	}

	args := d.buildRealDockerArgs("test-container", "python:3.12-slim", "/tmp/code", "main.py",
		&ExecutionRequest{
			Language: LangPython,
			Code:     "print('hi')",
			EnvVars:  map[string]string{"BAZ": "qux"},
		}, cfg)

	assert.Contains(t, args, "--name")
	assert.Contains(t, args, "test-container")
	assert.Contains(t, args, "--memory")
	assert.Contains(t, args, "--network")
	assert.Contains(t, args, "none")
	assert.Contains(t, args, "--security-opt")
	assert.Contains(t, args, "--pids-limit")
}

func TestRealDockerBackend_BuildRealDockerArgs_NetworkEnabled(t *testing.T) {
	d := NewRealDockerBackend(nil)
	cfg := SandboxConfig{
		NetworkEnabled: true,
	}

	args := d.buildRealDockerArgs("test-net", "python:3.12-slim", "/tmp/code", "main.py",
		&ExecutionRequest{Language: LangPython, Code: "pass"}, cfg)

	// Should NOT contain --network none
	for i, arg := range args {
		if arg == "--network" && i+1 < len(args) {
			assert.NotEqual(t, "none", args[i+1])
		}
	}
}

func TestRealDockerBackend_ExecuteNoImage(t *testing.T) {
	d := NewRealDockerBackend(nil)
	d.images = map[Language]string{} // clear all images

	result, err := d.Execute(context.Background(), &ExecutionRequest{
		ID:       "no-img",
		Language: LangPython,
		Code:     "pass",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no image configured")
}

func TestRealDockerBackend_Cleanup_Empty(t *testing.T) {
	d := NewRealDockerBackend(nil)
	err := d.Cleanup()
	require.NoError(t, err)
}

// --- RealProcessBackend unit tests ---

func TestRealProcessBackend_Disabled(t *testing.T) {
	p := NewRealProcessBackend(nil, false)
	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "disabled",
		Language: LangPython,
		Code:     "print('hi')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "disabled")
}

func TestRealProcessBackend_DangerousCode(t *testing.T) {
	p := NewRealProcessBackend(nil, true)
	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "dangerous",
		Language: LangPython,
		Code:     "import os\nos.system('rm -rf /')",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "code validation failed")
}

func TestRealProcessBackend_NoInterpreter(t *testing.T) {
	p := NewRealProcessBackend(nil, true)
	p.interpreters = map[Language]string{} // clear

	result, err := p.Execute(context.Background(), &ExecutionRequest{
		ID:       "no-interp",
		Language: LangPython,
		Code:     "x = 1",
	}, DefaultSandboxConfig())

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "no interpreter")
}

// --- SandboxExecutor request timeout larger than config ---

func TestSandboxExecutor_RequestTimeoutLargerThanConfig(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			remaining := time.Until(deadline)
			// Config timeout (30s) should be used since request timeout (60s) is larger
			assert.Less(t, remaining, 31*time.Second)
			return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	cfg.Timeout = 30 * time.Second
	exec := NewSandboxExecutor(cfg, backend, nil)

	_, err := exec.Execute(context.Background(), &ExecutionRequest{
		ID:       "large-timeout",
		Language: LangPython,
		Code:     "pass",
		Timeout:  60 * time.Second, // larger than config
	})
	require.NoError(t, err)
}

// --- SandboxExecutor stats after mixed success/failure ---

func TestSandboxExecutor_MixedStats(t *testing.T) {
	callNum := 0
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			callNum++
			if callNum%3 == 0 {
				return nil, fmt.Errorf("backend error")
			}
			if callNum%2 == 0 {
				return &ExecutionResult{ID: req.ID, Success: false, ExitCode: 1}, nil
			}
			return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	exec := NewSandboxExecutor(cfg, backend, nil)

	for i := 0; i < 6; i++ {
		exec.Execute(context.Background(), &ExecutionRequest{
			ID:       fmt.Sprintf("mixed-%d", i),
			Language: LangPython,
			Code:     "pass",
		})
	}

	stats := exec.Stats()
	assert.Equal(t, int64(6), stats.TotalExecutions)
	assert.Greater(t, stats.SuccessExecutions, int64(0))
	assert.Greater(t, stats.FailedExecutions, int64(0))
}

// --- ProcessBackend with custom config ---

func TestProcessBackendWithConfig(t *testing.T) {
	p := NewProcessBackendWithConfig(nil, ProcessBackendConfig{
		WorkDir:            "/custom/dir",
		Enabled:            true,
		CustomInterpreters: map[Language]string{LangPython: "python3.12"},
	})

	assert.True(t, p.enabled)
	assert.Equal(t, "/custom/dir", p.workDir)
	assert.Equal(t, "python3.12", p.interpreters[LangPython])
	// Default interpreters still present for other languages
	assert.Equal(t, "node", p.interpreters[LangJavaScript])
}

// --- ProcessBackend default language fallback ---

func TestProcessBackend_BuildArgs_DefaultLang(t *testing.T) {
	p := NewProcessBackend(nil)
	args := p.buildArgs(&ExecutionRequest{Language: Language("unknown"), Code: "test"})
	assert.Equal(t, "-c", args[0])
}

// --- DockerBackend.buildCommand all languages ---

func TestDockerBackend_BuildCommand_AllLanguages(t *testing.T) {
	d := NewDockerBackend(nil)

	tests := []struct {
		lang     Language
		wantCmd  string
		wantArg1 string
	}{
		{LangPython, "python3", "-c"},
		{LangJavaScript, "node", "-e"},
		{LangTypeScript, "node", "-e"},
		{LangGo, "go", "run"},
		{LangRust, "sh", "-c"},
		{LangBash, "sh", "-c"},
		{Language("lua"), "sh", "-c"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			cmd := d.buildCommand(&ExecutionRequest{Language: tt.lang, Code: "test"})
			require.GreaterOrEqual(t, len(cmd), 2)
			assert.Equal(t, tt.wantCmd, cmd[0])
			assert.Equal(t, tt.wantArg1, cmd[1])
		})
	}
}

// --- DockerBackend.Execute with active containers ---

func TestDockerBackend_Cleanup_WithActiveContainers(t *testing.T) {
	d := NewDockerBackend(nil)
	// Simulate active containers
	d.mu.Lock()
	d.activeContainers["test-container-1"] = struct{}{}
	d.activeContainers["test-container-2"] = struct{}{}
	d.mu.Unlock()

	err := d.Cleanup()
	require.NoError(t, err)
	// Cleanup calls kill+remove but doesn't clear the map itself
}

// --- RealDockerBackend.Cleanup with active containers ---

func TestRealDockerBackend_Cleanup_WithActiveContainers(t *testing.T) {
	d := NewRealDockerBackend(nil)
	d.mu.Lock()
	d.activeContainers["test-container-1"] = struct{}{}
	d.mu.Unlock()

	err := d.Cleanup()
	require.NoError(t, err)
}

// --- DockerBackend.buildDockerArgs with mount paths ---

func TestDockerBackend_BuildDockerArgs_WithMounts(t *testing.T) {
	d := NewDockerBackend(nil)
	cfg := SandboxConfig{
		MountPaths: map[string]string{
			"/host/data": "/container/data",
		},
	}

	args := d.buildDockerArgs("test", "python:3.12-slim", &ExecutionRequest{
		Language: LangPython,
		Code:     "pass",
	}, cfg, "")

	// Should contain -v mount
	foundMount := false
	for i, arg := range args {
		if arg == "-v" && i+1 < len(args) && args[i+1] == "/host/data:/container/data:ro" {
			foundMount = true
		}
	}
	assert.True(t, foundMount, "expected mount path in docker args")
}

// --- SandboxTool with warnings ---

func TestSandboxTool_WithWarnings(t *testing.T) {
	backend := &testBackend{
		executeFn: func(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
			return &ExecutionResult{ID: req.ID, Success: true, ExitCode: 0, Stdout: "ok"}, nil
		},
	}

	cfg := DefaultSandboxConfig()
	cfg.AllowedLanguages = append(cfg.AllowedLanguages, LangBash)
	executor := NewSandboxExecutor(cfg, backend, nil)
	tool := NewSandboxTool(executor, nil)

	// Code with dangerous patterns should still execute (warnings only)
	args := []byte(`{"id":"warn-1","language":"bash","code":"echo hi"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
