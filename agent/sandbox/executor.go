// Package sandbox provides secure code execution for AI-generated code.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ExecutionMode defines the sandbox execution mode.
type ExecutionMode string

const (
	ModeDocker ExecutionMode = "docker"
	ModeWASM   ExecutionMode = "wasm"
	ModeNative ExecutionMode = "native" // For trusted environments only
)

// Language represents supported programming languages.
type Language string

const (
	LangPython     Language = "python"
	LangJavaScript Language = "javascript"
	LangTypeScript Language = "typescript"
	LangGo         Language = "go"
	LangRust       Language = "rust"
	LangBash       Language = "bash"
)

// SandboxConfig configures the sandbox executor.
type SandboxConfig struct {
	Mode             ExecutionMode     `json:"mode"`
	Timeout          time.Duration     `json:"timeout"`
	MaxMemoryMB      int               `json:"max_memory_mb"`
	MaxCPUPercent    int               `json:"max_cpu_percent"`
	NetworkEnabled   bool              `json:"network_enabled"`
	AllowedHosts     []string          `json:"allowed_hosts,omitempty"`
	MountPaths       map[string]string `json:"mount_paths,omitempty"` // host:container
	EnvVars          map[string]string `json:"env_vars,omitempty"`
	MaxOutputBytes   int               `json:"max_output_bytes"`
	AllowedLanguages []Language        `json:"allowed_languages"`
}

// DefaultSandboxConfig returns secure defaults.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Mode:             ModeDocker,
		Timeout:          30 * time.Second,
		MaxMemoryMB:      512,
		MaxCPUPercent:    50,
		NetworkEnabled:   false,
		MaxOutputBytes:   1024 * 1024, // 1MB
		AllowedLanguages: []Language{LangPython, LangJavaScript},
	}
}

// ExecutionRequest represents a code execution request.
type ExecutionRequest struct {
	ID       string            `json:"id"`
	Language Language          `json:"language"`
	Code     string            `json:"code"`
	Stdin    string            `json:"stdin,omitempty"`
	Args     []string          `json:"args,omitempty"`
	EnvVars  map[string]string `json:"env_vars,omitempty"`
	Files    map[string]string `json:"files,omitempty"` // filename -> content
	Timeout  time.Duration     `json:"timeout,omitempty"`
}

// ExecutionResult represents the result of code execution.
type ExecutionResult struct {
	ID         string        `json:"id"`
	Success    bool          `json:"success"`
	ExitCode   int           `json:"exit_code"`
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
	MemoryUsed int64         `json:"memory_used_bytes,omitempty"`
	Truncated  bool          `json:"truncated,omitempty"`
}

// SandboxExecutor executes code in an isolated environment.
type SandboxExecutor struct {
	config  SandboxConfig
	backend ExecutionBackend
	logger  *zap.Logger
	mu      sync.RWMutex
	stats   ExecutorStats
}

// ExecutorStats tracks execution statistics.
type ExecutorStats struct {
	TotalExecutions   int64         `json:"total_executions"`
	SuccessExecutions int64         `json:"success_executions"`
	FailedExecutions  int64         `json:"failed_executions"`
	TimeoutExecutions int64         `json:"timeout_executions"`
	TotalDuration     time.Duration `json:"total_duration"`
}

// ExecutionBackend defines the interface for execution backends.
type ExecutionBackend interface {
	Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error)
	Cleanup() error
	Name() string
}

// NewSandboxExecutor creates a new sandbox executor.
func NewSandboxExecutor(config SandboxConfig, backend ExecutionBackend, logger *zap.Logger) *SandboxExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SandboxExecutor{
		config:  config,
		backend: backend,
		logger:  logger,
	}
}

// Execute runs code in the sandbox.
func (s *SandboxExecutor) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	start := time.Now()

	// Validate request
	if err := s.validate(req); err != nil {
		return nil, err
	}

	// Apply timeout
	timeout := s.config.Timeout
	if req.Timeout > 0 && req.Timeout < timeout {
		timeout = req.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Debug("executing code",
		zap.String("id", req.ID),
		zap.String("language", string(req.Language)),
		zap.Int("code_length", len(req.Code)))

	// Execute
	result, err := s.backend.Execute(ctx, req, s.config)

	// Update stats
	s.mu.Lock()
	s.stats.TotalExecutions++
	s.stats.TotalDuration += time.Since(start)
	if err != nil || !result.Success {
		s.stats.FailedExecutions++
		if ctx.Err() == context.DeadlineExceeded {
			s.stats.TimeoutExecutions++
		}
	} else {
		s.stats.SuccessExecutions++
	}
	s.mu.Unlock()

	if err != nil {
		return nil, err
	}

	// Truncate output if needed
	if len(result.Stdout) > s.config.MaxOutputBytes {
		result.Stdout = result.Stdout[:s.config.MaxOutputBytes]
		result.Truncated = true
	}
	if len(result.Stderr) > s.config.MaxOutputBytes {
		result.Stderr = result.Stderr[:s.config.MaxOutputBytes]
		result.Truncated = true
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (s *SandboxExecutor) validate(req *ExecutionRequest) error {
	if req.Code == "" {
		return fmt.Errorf("code is required")
	}

	// Check language is allowed
	allowed := false
	for _, lang := range s.config.AllowedLanguages {
		if lang == req.Language {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("language %s is not allowed", req.Language)
	}

	return nil
}

// Stats returns execution statistics.
func (s *SandboxExecutor) Stats() ExecutorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Cleanup releases resources.
func (s *SandboxExecutor) Cleanup() error {
	return s.backend.Cleanup()
}

// DockerBackend implements ExecutionBackend using Docker.
type DockerBackend struct {
	images map[Language]string
	logger *zap.Logger
}

// NewDockerBackend creates a Docker execution backend.
func NewDockerBackend(logger *zap.Logger) *DockerBackend {
	return &DockerBackend{
		images: map[Language]string{
			LangPython:     "python:3.12-slim",
			LangJavaScript: "node:20-slim",
			LangTypeScript: "node:20-slim",
			LangGo:         "golang:1.24-alpine",
			LangBash:       "alpine:latest",
		},
		logger: logger,
	}
}

func (d *DockerBackend) Name() string { return "docker" }

func (d *DockerBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	// This is a placeholder - actual Docker implementation would use Docker SDK
	// For production, use github.com/docker/docker/client

	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
		Error:    "docker backend not fully implemented - use actual Docker SDK",
	}

	// Simulate execution for demonstration
	image, ok := d.images[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no image for language: %s", req.Language)
		return result, nil
	}

	d.logger.Debug("would execute in docker",
		zap.String("image", image),
		zap.String("language", string(req.Language)))

	return result, nil
}

func (d *DockerBackend) Cleanup() error {
	return nil
}

// ProcessBackend implements ExecutionBackend using local processes (less secure).
type ProcessBackend struct {
	interpreters map[Language]string
	logger       *zap.Logger
}

// NewProcessBackend creates a process-based execution backend.
func NewProcessBackend(logger *zap.Logger) *ProcessBackend {
	return &ProcessBackend{
		interpreters: map[Language]string{
			LangPython:     "python3",
			LangJavaScript: "node",
			LangBash:       "bash",
		},
		logger: logger,
	}
}

func (p *ProcessBackend) Name() string { return "process" }

func (p *ProcessBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	// This is a placeholder - actual implementation would use os/exec
	// WARNING: Process backend is less secure than Docker

	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
		Error:    "process backend disabled for security - use docker backend",
	}

	return result, nil
}

func (p *ProcessBackend) Cleanup() error {
	return nil
}

// CodeValidator validates code before execution.
type CodeValidator struct {
	blockedPatterns map[Language][]string
}

// NewCodeValidator creates a code validator.
func NewCodeValidator() *CodeValidator {
	return &CodeValidator{
		blockedPatterns: map[Language][]string{
			LangPython: {
				"import os", "import subprocess", "import sys",
				"__import__", "eval(", "exec(",
				"open(", "file(",
			},
			LangJavaScript: {
				"require('child_process')", "require('fs')",
				"process.env", "eval(",
			},
			LangBash: {
				"rm -rf", "mkfs", "dd if=",
				"> /dev/", "curl", "wget",
			},
		},
	}
}

// Validate checks code for dangerous patterns.
func (v *CodeValidator) Validate(lang Language, code string) []string {
	var warnings []string
	patterns, ok := v.blockedPatterns[lang]
	if !ok {
		return warnings
	}

	for _, pattern := range patterns {
		if containsPattern(code, pattern) {
			warnings = append(warnings, fmt.Sprintf("potentially dangerous pattern: %s", pattern))
		}
	}

	return warnings
}

func containsPattern(code, pattern string) bool {
	return len(code) >= len(pattern) && findPattern(code, pattern) >= 0
}

func findPattern(s, pattern string) int {
	for i := 0; i <= len(s)-len(pattern); i++ {
		if s[i:i+len(pattern)] == pattern {
			return i
		}
	}
	return -1
}

// SandboxTool wraps the sandbox executor as an agent tool.
type SandboxTool struct {
	executor  *SandboxExecutor
	validator *CodeValidator
	logger    *zap.Logger
}

// NewSandboxTool creates a sandbox tool.
func NewSandboxTool(executor *SandboxExecutor, logger *zap.Logger) *SandboxTool {
	return &SandboxTool{
		executor:  executor,
		validator: NewCodeValidator(),
		logger:    logger,
	}
}

// Execute runs code through the sandbox.
func (t *SandboxTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var req ExecutionRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate code
	warnings := t.validator.Validate(req.Language, req.Code)
	if len(warnings) > 0 {
		t.logger.Warn("code validation warnings", zap.Strings("warnings", warnings))
	}

	// Execute
	result, err := t.executor.Execute(ctx, &req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}
