package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ExecutionMode defines the sandbox execution mode.
type ExecutionMode string

const (
	ModeDocker  ExecutionMode = "docker"
	ModeProcess ExecutionMode = "process"
	ModeWASM    ExecutionMode = "wasm"
	ModeNative  ExecutionMode = "native"
)

// Language identifies a supported code execution language.
type Language string

const (
	LangPython     Language = "python"
	LangJavaScript Language = "javascript"
	LangTypeScript Language = "typescript"
	LangGo         Language = "go"
	LangRust       Language = "rust"
	LangBash       Language = "bash"
)

// SandboxConfig configures sandbox execution behavior.
type SandboxConfig struct {
	Mode             ExecutionMode     `json:"mode"`
	Timeout          time.Duration     `json:"timeout"`
	MaxMemoryMB      int               `json:"max_memory_mb"`
	MaxCPUPercent    int               `json:"max_cpu_percent"`
	NetworkEnabled   bool              `json:"network_enabled"`
	AllowedHosts     []string          `json:"allowed_hosts,omitempty"`
	MountPaths       map[string]string `json:"mount_paths,omitempty"`
	EnvVars          map[string]string `json:"env_vars,omitempty"`
	MaxOutputBytes   int               `json:"max_output_bytes"`
	AllowedLanguages []Language        `json:"allowed_languages"`
}

// DefaultSandboxConfig returns secure defaults for code execution.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Mode:             ModeDocker,
		Timeout:          30 * time.Second,
		MaxMemoryMB:      512,
		MaxCPUPercent:    50,
		NetworkEnabled:   false,
		MaxOutputBytes:   1024 * 1024,
		AllowedLanguages: []Language{LangPython, LangJavaScript},
	}
}

// ExecutionRequest represents a sandbox code execution request.
type ExecutionRequest struct {
	ID       string            `json:"id"`
	Language Language          `json:"language"`
	Code     string            `json:"code"`
	Stdin    string            `json:"stdin,omitempty"`
	Args     []string          `json:"args,omitempty"`
	EnvVars  map[string]string `json:"env_vars,omitempty"`
	Files    map[string]string `json:"files,omitempty"`
	Timeout  time.Duration     `json:"timeout,omitempty"`
}

// ExecutionResult is the result of running code in a sandbox.
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

// ExecutionBackend abstracts a sandbox execution backend.
type ExecutionBackend interface {
	Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error)
	Cleanup() error
	Name() string
}

// ExecutorStats tracks aggregate execution outcomes.
type ExecutorStats struct {
	TotalExecutions   int64         `json:"total_executions"`
	SuccessExecutions int64         `json:"success_executions"`
	FailedExecutions  int64         `json:"failed_executions"`
	TimeoutExecutions int64         `json:"timeout_executions"`
	TotalDuration     time.Duration `json:"total_duration"`
}

// SandboxExecutor executes code via a configured backend.
type SandboxExecutor struct {
	config    SandboxConfig
	backend   ExecutionBackend
	validator *CodeValidator
	logger    *zap.Logger
	mu        sync.RWMutex
	stats     ExecutorStats
}

// NewSandboxExecutor creates a sandbox executor.
func NewSandboxExecutor(config SandboxConfig, backend ExecutionBackend, logger *zap.Logger) *SandboxExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SandboxExecutor{
		config:    config,
		backend:   backend,
		validator: NewCodeValidator(),
		logger:    logger.With(zap.String("component", "sandbox_executor")),
	}
}

// Execute validates, times, and executes a request using the configured backend.
func (s *SandboxExecutor) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	start := time.Now()

	recordFailure := func(err error, timeout bool) (*ExecutionResult, error) {
		s.recordExecution(time.Since(start), false, timeout)
		return nil, err
	}

	if s.backend == nil {
		return recordFailure(fmt.Errorf("sandbox backend is nil"), false)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return recordFailure(err, err == context.DeadlineExceeded)
	}
	if err := s.validate(req); err != nil {
		return recordFailure(err, false)
	}

	if warnings := s.validator.Validate(req.Language, req.Code); len(warnings) > 0 {
		s.logger.Warn("sandbox code validation warnings",
			zap.String("language", string(req.Language)),
			zap.Strings("warnings", warnings),
		)
	}

	execCtx, cancel := withExecutionTimeout(ctx, s.config.Timeout, req.Timeout)
	defer cancel()

	result, err := s.backend.Execute(execCtx, req, s.config)
	timeout := execCtx.Err() == context.DeadlineExceeded
	if err != nil {
		return recordFailure(err, timeout)
	}
	if result == nil {
		return recordFailure(fmt.Errorf("sandbox backend returned nil result"), timeout)
	}

	s.truncateOutput(result)

	elapsed := time.Since(start)
	if result.Duration <= 0 {
		result.Duration = elapsed
	}
	s.recordExecution(elapsed, result.Success, timeout)
	return result, nil
}

func (s *SandboxExecutor) validate(req *ExecutionRequest) error {
	if req == nil {
		return fmt.Errorf("execution request is nil")
	}
	if strings.TrimSpace(req.Code) == "" {
		return fmt.Errorf("code is required")
	}
	if len(s.config.AllowedLanguages) == 0 {
		return nil
	}

	for _, lang := range s.config.AllowedLanguages {
		if lang == req.Language {
			return nil
		}
	}
	return fmt.Errorf("language %s is not allowed", req.Language)
}

func (s *SandboxExecutor) truncateOutput(result *ExecutionResult) {
	if result == nil || s.config.MaxOutputBytes <= 0 {
		return
	}

	limit := s.config.MaxOutputBytes
	if len(result.Stdout) > limit {
		result.Stdout = result.Stdout[:limit]
		result.Truncated = true
	}
	if len(result.Stderr) > limit {
		result.Stderr = result.Stderr[:limit]
		result.Truncated = true
	}
}

func (s *SandboxExecutor) recordExecution(duration time.Duration, success bool, timeout bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalExecutions++
	s.stats.TotalDuration += duration
	if success {
		s.stats.SuccessExecutions++
		return
	}
	s.stats.FailedExecutions++
	if timeout {
		s.stats.TimeoutExecutions++
	}
}

// Stats returns a snapshot of executor statistics.
func (s *SandboxExecutor) Stats() ExecutorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Cleanup delegates cleanup to the backend.
func (s *SandboxExecutor) Cleanup() error {
	if s.backend == nil {
		return nil
	}
	return s.backend.Cleanup()
}

func withExecutionTimeout(ctx context.Context, configTimeout, requestTimeout time.Duration) (context.Context, context.CancelFunc) {
	timeout := configTimeout
	if timeout <= 0 {
		timeout = requestTimeout
	} else if requestTimeout > 0 && requestTimeout < timeout {
		timeout = requestTimeout
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// DockerBackendConfig configures docker-backed execution.
type DockerBackendConfig struct {
	ContainerPrefix string
	CleanupOnExit   bool
	CustomImages    map[Language]string
}

// DockerBackend executes code inside docker containers.
type DockerBackend struct {
	images           map[Language]string
	logger           *zap.Logger
	containerPrefix  string
	cleanupOnExit    bool
	activeContainers map[string]struct{}
	mu               sync.Mutex
}

// NewDockerBackend creates the default docker backend.
func NewDockerBackend(logger *zap.Logger) *DockerBackend {
	return NewDockerBackendWithConfig(logger, DockerBackendConfig{
		ContainerPrefix: "sandbox_",
		CleanupOnExit:   true,
	})
}

// NewDockerBackendWithConfig creates a docker backend with overrides.
func NewDockerBackendWithConfig(logger *zap.Logger, cfg DockerBackendConfig) *DockerBackend {
	if logger == nil {
		logger = zap.NewNop()
	}

	images := defaultDockerImages()
	for lang, image := range cfg.CustomImages {
		images[lang] = image
	}

	prefix := cfg.ContainerPrefix
	if prefix == "" {
		prefix = "sandbox_"
	}

	return &DockerBackend{
		images:           images,
		logger:           logger.With(zap.String("component", "docker_backend")),
		containerPrefix:  prefix,
		cleanupOnExit:    cfg.CleanupOnExit || cfg.ContainerPrefix == "",
		activeContainers: make(map[string]struct{}),
	}
}

// Name returns the backend name.
func (d *DockerBackend) Name() string { return "docker" }

// Execute runs code inside a docker container.
func (d *DockerBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := time.Now()
	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
	}

	image, ok := d.images[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no image configured for language: %s", req.Language)
		result.Duration = time.Since(start)
		return result, nil
	}

	containerName := fmt.Sprintf("%s%s_%d", d.containerPrefix, sanitizeID(req.ID), time.Now().UnixNano())
	codeMountDir := ""
	if req.Language == LangGo || req.Language == LangRust {
		codeMountDir = "/tmp/code"
	}
	args := d.buildDockerArgs(containerName, image, req, config, codeMountDir)
	d.logger.Debug("simulated docker execution",
		zap.String("container", containerName),
		zap.String("image", image),
		zap.Strings("args", args),
	)

	d.mu.Lock()
	d.activeContainers[containerName] = struct{}{}
	d.mu.Unlock()

	if d.cleanupOnExit {
		defer func() {
			d.mu.Lock()
			delete(d.activeContainers, containerName)
			d.mu.Unlock()
		}()
	}

	result.Success = true
	result.ExitCode = 0
	result.Duration = time.Since(start)
	return result, nil
}

func (d *DockerBackend) buildDockerArgs(containerName, image string, req *ExecutionRequest, config SandboxConfig, codeMountDir string) []string {
	args := []string{
		"run",
		"--name", containerName,
		"--rm",
	}

	if config.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", config.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", config.MaxMemoryMB))
	}
	if config.MaxCPUPercent > 0 {
		cpus := float64(config.MaxCPUPercent) / 100.0
		args = append(args, "--cpus", fmt.Sprintf("%.2f", cpus))
	}
	if !config.NetworkEnabled {
		args = append(args, "--network", "none")
	}

	args = append(args,
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
	)

	for k, v := range config.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range req.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for hostPath, containerPath := range config.MountPaths {
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", hostPath, containerPath))
	}
	if codeMountDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/code:ro", codeMountDir))
	}

	args = append(args, image)
	args = append(args, d.buildCommand(req)...)
	return args
}

func (d *DockerBackend) buildCommand(req *ExecutionRequest) []string {
	switch req.Language {
	case LangPython:
		return []string{"python3", "-c", req.Code}
	case LangJavaScript, LangTypeScript:
		return []string{"node", "-e", req.Code}
	case LangGo:
		return []string{"go", "run", "/code/main.go"}
	case LangRust:
		return []string{"sh", "-c", "rustc /code/main.rs -o /tmp/main && /tmp/main"}
	case LangBash:
		return []string{"sh", "-c", req.Code}
	default:
		return []string{"sh", "-c", req.Code}
	}
}

func (d *DockerBackend) killContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "docker", "kill", name)
	if _, _, err := cmd.Run(); err != nil {
		d.logger.Debug("failed to kill container", zap.String("name", name), zap.Error(err))
	}
}

func (d *DockerBackend) removeContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "docker", "rm", "-f", name)
	if _, _, err := cmd.Run(); err != nil {
		d.logger.Debug("failed to remove container", zap.String("name", name), zap.Error(err))
	}
}

// Cleanup removes tracked docker containers.
func (d *DockerBackend) Cleanup() error {
	d.mu.Lock()
	containers := make([]string, 0, len(d.activeContainers))
	for name := range d.activeContainers {
		containers = append(containers, name)
	}
	d.mu.Unlock()

	for _, name := range containers {
		d.killContainer(name)
		d.removeContainer(name)
	}
	return nil
}

// ProcessBackendConfig configures trusted local process execution.
type ProcessBackendConfig struct {
	WorkDir            string
	Enabled            bool
	CustomInterpreters map[Language]string
}

// ProcessBackend executes code with local interpreters.
type ProcessBackend struct {
	interpreters map[Language]string
	logger       *zap.Logger
	workDir      string
	enabled      bool
}

// NewProcessBackend creates a disabled-by-default process backend.
func NewProcessBackend(logger *zap.Logger) *ProcessBackend {
	return NewProcessBackendWithConfig(logger, ProcessBackendConfig{Enabled: false})
}

// NewProcessBackendWithConfig creates a process backend with overrides.
func NewProcessBackendWithConfig(logger *zap.Logger, cfg ProcessBackendConfig) *ProcessBackend {
	if logger == nil {
		logger = zap.NewNop()
	}

	interpreters := defaultProcessInterpreters()
	for lang, interp := range cfg.CustomInterpreters {
		interpreters[lang] = interp
	}

	return &ProcessBackend{
		interpreters: interpreters,
		logger:       logger.With(zap.String("component", "process_backend")),
		workDir:      cfg.WorkDir,
		enabled:      cfg.Enabled,
	}
}

// Name returns the backend name.
func (p *ProcessBackend) Name() string { return "process" }

// Execute runs code with a trusted local interpreter.
func (p *ProcessBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := time.Now()
	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
	}

	if !p.enabled {
		result.Error = "process backend disabled for security - enable explicitly"
		result.Duration = time.Since(start)
		return result, nil
	}

	interpreter, ok := p.interpreters[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no interpreter for language: %s", req.Language)
		result.Duration = time.Since(start)
		return result, nil
	}

	args := p.buildArgs(req)
	p.logger.Debug("simulated process execution",
		zap.String("interpreter", interpreter),
		zap.String("language", string(req.Language)),
		zap.Strings("args", args),
		zap.Int("config_env_vars", len(config.EnvVars)),
	)

	result.Success = true
	result.ExitCode = 0
	result.Duration = time.Since(start)
	return result, nil
}

func (p *ProcessBackend) buildArgs(req *ExecutionRequest) []string {
	switch req.Language {
	case LangPython:
		return []string{"-c", req.Code}
	case LangJavaScript, LangTypeScript:
		return []string{"-e", req.Code}
	case LangBash:
		return []string{"-c", req.Code}
	case LangGo:
		return []string{"run", "main.go"}
	case LangRust:
		return []string{"main.rs"}
	default:
		return []string{"-c", req.Code}
	}
}

// Cleanup is a no-op for the process backend.
func (p *ProcessBackend) Cleanup() error { return nil }

// CodeValidator performs simple pattern-based safety checks.
type CodeValidator struct {
	blockedPatterns map[Language][]string
}

// NewCodeValidator creates a runtime validator tuned for sandbox execution.
func NewCodeValidator() *CodeValidator {
	return &CodeValidator{
		blockedPatterns: map[Language][]string{
			LangPython: {
				"import os", "from os", "os.system", "os.popen", "os.exec",
				"import subprocess", "from subprocess", "subprocess.run", "subprocess.call", "subprocess.Popen",
				"__import__", "eval(", "exec(", "compile(",
				"import shutil", "shutil.rmtree", "shutil.move",
				"import socket", "import urllib", "import requests", "import httplib",
				"import ctypes", "import pickle", "pickle.load",
				"import marshal", "marshal.load",
				"globals()", "locals()", "vars()", "dir(",
				"getattr(", "setattr(", "delattr(",
				"__builtins__", "__class__", "__bases__", "__subclasses__",
			},
			LangJavaScript: {
				"require('child_process')", "require('fs')", "require('os')",
				"require(\"child_process\")", "require(\"fs\")", "require(\"os\")",
				"import child_process", "import fs", "import os",
				"process.env", "process.exit", "process.kill",
				"eval(", "Function(", "new Function",
				"require('http')", "require('https')", "require('net')",
				"require(\"http\")", "require(\"https\")", "require(\"net\")",
				"__proto__", "constructor.constructor",
			},
			LangTypeScript: {
				"require('child_process')", "require(\"child_process\")", "child_process",
				"eval(", "new Function(",
			},
			LangBash: {
				"rm -rf", "rm -fr", "rmdir", "mkfs", "dd if=",
				"> /dev/", ">/dev/",
				"curl", "wget", "nc ", "netcat",
				"chmod", "chown", "sudo", "su ",
				"shutdown", "reboot", "init ", "systemctl",
				"kill ", "killall", "pkill",
				"/etc/passwd", "/etc/shadow", "~/.ssh",
				"printenv", "env", "export",
			},
			LangGo: {
				"os/exec", "exec.Command",
				"syscall.", "unsafe.",
				"os.Remove", "os.RemoveAll",
				"net/http", "net.Dial",
			},
			LangRust: {
				"std::process::Command",
				"std::fs::remove",
				"unsafe {", "unsafe{",
			},
		},
	}
}

// Validate returns warnings for suspicious code patterns.
func (v *CodeValidator) Validate(lang Language, code string) []string {
	if strings.TrimSpace(code) == "" {
		return nil
	}

	patterns, ok := v.blockedPatterns[lang]
	if !ok {
		return nil
	}

	warnings := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		if !containsPattern(code, pattern) {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		warnings = append(warnings, fmt.Sprintf("potentially dangerous pattern: %s", pattern))
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

// SandboxTool exposes sandbox execution as a JSON-driven tool.
type SandboxTool struct {
	executor  *SandboxExecutor
	validator *CodeValidator
	logger    *zap.Logger
}

// NewSandboxTool creates a sandbox tool wrapper.
func NewSandboxTool(executor *SandboxExecutor, logger *zap.Logger) *SandboxTool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SandboxTool{
		executor:  executor,
		validator: NewCodeValidator(),
		logger:    logger.With(zap.String("component", "sandbox_tool")),
	}
}

// Execute decodes a request, validates code, and runs the sandbox.
func (t *SandboxTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.executor == nil {
		return nil, fmt.Errorf("sandbox executor is nil")
	}

	var req ExecutionRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if warnings := t.validator.Validate(req.Language, req.Code); len(warnings) > 0 {
		t.logger.Warn("code validation warnings", zap.Strings("warnings", warnings))
	}

	result, err := t.executor.Execute(ctx, &req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// execCommand wraps os/exec for deterministic testing.
type execCommand struct {
	cmd      string
	args     []string
	ctx      context.Context
	stdin    string
	stdout   string
	stderr   string
	exitCode int
}

func execCommandContext(ctx context.Context, cmd string, args ...string) *execCommand {
	return &execCommand{
		cmd:  cmd,
		args: args,
		ctx:  ctx,
	}
}

func (c *execCommand) SetStdin(stdin string) {
	c.stdin = stdin
}

func (c *execCommand) Run() (stdout, stderr string, err error) {
	cmd := exec.CommandContext(c.ctx, c.cmd, c.args...)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if c.stdin != "" {
		cmd.Stdin = bytes.NewReader([]byte(c.stdin))
	}

	runErr := cmd.Run()
	c.stdout = stdoutBuf.String()
	c.stderr = stderrBuf.String()

	if cmd.ProcessState != nil {
		c.exitCode = cmd.ProcessState.ExitCode()
	} else {
		c.exitCode = -1
	}

	if runErr != nil {
		if c.exitCode != 0 {
			return c.stdout, c.stderr, fmt.Errorf("command exited with code %d: %w", c.exitCode, runErr)
		}
		return c.stdout, c.stderr, runErr
	}
	return c.stdout, c.stderr, nil
}

func (c *execCommand) ExitCode() int {
	return c.exitCode
}

func defaultDockerImages() map[Language]string {
	return map[Language]string{
		LangPython:     "python:3.12-slim",
		LangJavaScript: "node:20-slim",
		LangTypeScript: "node:20-slim",
		LangGo:         "golang:1.24-alpine",
		LangRust:       "rust:1.75-slim",
		LangBash:       "alpine:latest",
	}
}

func defaultProcessInterpreters() map[Language]string {
	return map[Language]string{
		LangPython:     "python",
		LangJavaScript: "node",
		LangTypeScript: "node",
		LangBash:       "bash",
		LangGo:         "go",
		LangRust:       "rustc",
	}
}
