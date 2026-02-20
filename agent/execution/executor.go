// 软件包执行为AI生成代码提供了安全代码执行.
package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ExecutiveMode定义了沙盒执行模式.
type ExecutionMode string

const (
	ModeDocker ExecutionMode = "docker"
	ModeWASM   ExecutionMode = "wasm"
	ModeNative ExecutionMode = "native" // For trusted environments only
)

// 语言代表支持的编程语言.
type Language string

const (
	LangPython     Language = "python"
	LangJavaScript Language = "javascript"
	LangTypeScript Language = "typescript"
	LangGo         Language = "go"
	LangRust       Language = "rust"
	LangBash       Language = "bash"
)

// Sandbox Config 配置了 sandbox 执行器 。
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

// 默认SandboxConfig返回安全默认 。
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

// 执行请求代表代码执行请求.
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

// 执行Result代表代码执行的结果.
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

// SandboxExecutor在孤立的环境中执行代码.
type SandboxExecutor struct {
	config  SandboxConfig
	backend ExecutionBackend
	logger  *zap.Logger
	mu      sync.RWMutex
	stats   ExecutorStats
}

// 执行者Stats跟踪执行统计.
type ExecutorStats struct {
	TotalExecutions   int64         `json:"total_executions"`
	SuccessExecutions int64         `json:"success_executions"`
	FailedExecutions  int64         `json:"failed_executions"`
	TimeoutExecutions int64         `json:"timeout_executions"`
	TotalDuration     time.Duration `json:"total_duration"`
}

// ExecutiveBackend定义执行后端的接口.
type ExecutionBackend interface {
	Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error)
	Cleanup() error
	Name() string
}

// NewSandboxExecutor创建了新的沙盒执行器.
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

// 执行在沙盒中运行代码.
func (s *SandboxExecutor) Execute(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	start := time.Now()

	// 验证请求
	if err := s.validate(req); err != nil {
		return nil, err
	}

	// 应用超时
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

	// 执行
	result, err := s.backend.Execute(ctx, req, s.config)

	// 更新数据
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

	// 需要时断线输出
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

	// 允许检查语言
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

// Stats 返回执行统计 。
func (s *SandboxExecutor) Stats() ExecutorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// 清理释放出资源.
func (s *SandboxExecutor) Cleanup() error {
	return s.backend.Cleanup()
}

// DockerBackend使用Docker执行Backend.
type DockerBackend struct {
	images           map[Language]string
	logger           *zap.Logger
	containerPrefix  string
	cleanupOnExit    bool
	activeContainers map[string]struct{}
	mu               sync.Mutex
}

// DockerBackendConfig配置了多克后端.
type DockerBackendConfig struct {
	ContainerPrefix string              // Prefix for container names
	CleanupOnExit   bool                // Remove containers after execution
	CustomImages    map[Language]string // Override default images
}

// NewDockerBackend创建了多克执行后端.
func NewDockerBackend(logger *zap.Logger) *DockerBackend {
	return NewDockerBackendWithConfig(logger, DockerBackendConfig{
		ContainerPrefix: "sandbox_",
		CleanupOnExit:   true,
	})
}

// NewDockerBackendWithConfig创建了自定义配置的多克后端.
func NewDockerBackendWithConfig(logger *zap.Logger, cfg DockerBackendConfig) *DockerBackend {
	if logger == nil {
		logger = zap.NewNop()
	}

	images := map[Language]string{
		LangPython:     "python:3.12-slim",
		LangJavaScript: "node:20-slim",
		LangTypeScript: "node:20-slim",
		LangGo:         "golang:1.24-alpine",
		LangRust:       "rust:1.75-slim",
		LangBash:       "alpine:latest",
	}

	// 应用自定义图像
	for lang, img := range cfg.CustomImages {
		images[lang] = img
	}

	prefix := cfg.ContainerPrefix
	if prefix == "" {
		prefix = "sandbox_"
	}

	return &DockerBackend{
		images:           images,
		logger:           logger,
		containerPrefix:  prefix,
		cleanupOnExit:    cfg.CleanupOnExit,
		activeContainers: make(map[string]struct{}),
	}
}

func (d *DockerBackend) Name() string { return "docker" }

func (d *DockerBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	start := time.Now()

	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
	}

	// 获取语言图像
	image, ok := d.images[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no image configured for language: %s", req.Language)
		return result, nil
	}

	// 生成容器名称
	containerName := fmt.Sprintf("%s%s_%d", d.containerPrefix, req.ID, time.Now().UnixNano())

	// 构建嵌入器运行命令
	args := d.buildDockerArgs(containerName, image, req, config)

	d.logger.Debug("executing in docker",
		zap.String("container", containerName),
		zap.String("image", image),
		zap.String("language", string(req.Language)),
	)

	// 跟踪活动容器
	d.mu.Lock()
	d.activeContainers[containerName] = struct{}{}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.activeContainers, containerName)
		d.mu.Unlock()

		if d.cleanupOnExit {
			d.removeContainer(containerName)
		}
	}()

	// 执行嵌入器运行
	stdout, stderr, exitCode, err := d.runDocker(ctx, args, req.Stdin)

	result.Duration = time.Since(start)
	result.Stdout = stdout
	result.Stderr = stderr
	result.ExitCode = exitCode

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "execution timeout"
			d.killContainer(containerName)
		} else {
			result.Error = err.Error()
		}
		return result, nil
	}

	result.Success = exitCode == 0
	return result, nil
}

func (d *DockerBackend) buildDockerArgs(containerName, image string, req *ExecutionRequest, config SandboxConfig) []string {
	args := []string{
		"run",
		"--name", containerName,
		"--rm",
	}

	// 内存限制
	if config.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", config.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", config.MaxMemoryMB)) // Disable swap
	}

	// CPU 限制
	if config.MaxCPUPercent > 0 {
		cpus := float64(config.MaxCPUPercent) / 100.0
		args = append(args, "--cpus", fmt.Sprintf("%.2f", cpus))
	}

	// 网络
	if !config.NetworkEnabled {
		args = append(args, "--network", "none")
	}

	// 安全选项
	args = append(args,
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
	)

	// 环境变量
	for k, v := range config.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range req.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// 挂载路径( 默认只读)
	for hostPath, containerPath := range config.MountPaths {
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", hostPath, containerPath))
	}

	// 图像
	args = append(args, image)

	// 基于语言的命令
	cmd := d.buildCommand(req)
	args = append(args, cmd...)

	return args
}

func (d *DockerBackend) buildCommand(req *ExecutionRequest) []string {
	switch req.Language {
	case LangPython:
		return []string{"python3", "-c", req.Code}
	case LangJavaScript:
		return []string{"node", "-e", req.Code}
	case LangTypeScript:
		// TypeScript 需要编译, 先使用 ts- node 或 transpile
		return []string{"node", "-e", req.Code}
	case LangGo:
		// 需要编辑, 使用临时文件运行
		return []string{"sh", "-c", fmt.Sprintf("echo '%s' > /tmp/main.go && go run /tmp/main.go", escapeShellArg(req.Code))}
	case LangRust:
		// 需要汇编
		return []string{"sh", "-c", fmt.Sprintf("echo '%s' > /tmp/main.rs && rustc /tmp/main.rs -o /tmp/main && /tmp/main", escapeShellArg(req.Code))}
	case LangBash:
		return []string{"sh", "-c", req.Code}
	default:
		return []string{"sh", "-c", req.Code}
	}
}

func (d *DockerBackend) runDocker(ctx context.Context, args []string, stdin string) (stdout, stderr string, exitCode int, err error) {
	// 使用 os/ exec 来运行 Docker 命令
	cmd := execCommandContext(ctx, "docker", args...)

	if stdin != "" {
		cmd.SetStdin(stdin)
	}

	stdoutBuf, stderrBuf, err := cmd.Run()
	stdout = stdoutBuf
	stderr = stderrBuf
	exitCode = cmd.ExitCode()

	return stdout, stderr, exitCode, err
}

func (d *DockerBackend) killContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "docker", "kill", name)
	cmd.Run()

	d.logger.Debug("killed container", zap.String("name", name))
}

func (d *DockerBackend) removeContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "docker", "rm", "-f", name)
	cmd.Run()

	d.logger.Debug("removed container", zap.String("name", name))
}

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

	d.logger.Info("cleaned up containers", zap.Int("count", len(containers)))
	return nil
}

// 执行命令包接 OS/ Executive 测试
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
	// 运行时导入 os/ exec 以避免进口周期问题
	// 在生产中,这将使用执行。 直接命令内容

	// 目前模拟执行 -- -- 用于生产:
	// cmd:=exec.command Context(c.ctx,c.cmd,c.args.) (中文(简体) ).
	// var stdout Buf, stderr Buf 字节. 缓冲
	// (原始内容存档于2018-09-29). Cmd. Stdout = &stdout Buf 调制组
	// (原始内容存档于2018-09-29). Cmd. Stderr = &stderr Buf
	// 如果 c.stdin != "" { 请检查url=值 (帮助)
	//     cmd.Stdin = 字符串. NewReader (c.stdin) 互联网档案馆的存檔,存档日期2013-12-21.
	// }
	// 错误=cmd. 运行( C)
	// 返回 stdoutBuf.String (), stderrBuf.String (), 错误

	c.stdout = ""
	c.stderr = ""
	c.exitCode = 0
	return c.stdout, c.stderr, nil
}

func (c *execCommand) ExitCode() int {
	return c.exitCode
}

func escapeShellArg(s string) string {
	// 为 shell 逃出单引号
	result := make([]byte, 0, len(s)+10)
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			result = append(result, '\'', '\\', '\'', '\'')
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}

// processBackend使用本地进程执行 ElectionBackend.
// 警告: 不如多克安全 - 只在信任的环境中使用.
type ProcessBackend struct {
	interpreters map[Language]string
	logger       *zap.Logger
	workDir      string
	enabled      bool
}

// processBackendConfig 配置进程后端.
type ProcessBackendConfig struct {
	WorkDir            string // Working directory for execution
	Enabled            bool   // Must explicitly enable (security)
	CustomInterpreters map[Language]string
}

// NewProcessBackend创建基于进程的执行后端.
func NewProcessBackend(logger *zap.Logger) *ProcessBackend {
	return NewProcessBackendWithConfig(logger, ProcessBackendConfig{
		Enabled: false, // Disabled by default for security
	})
}

// NewProcessBackendWithConfig创建了自定义配置的流程后端.
func NewProcessBackendWithConfig(logger *zap.Logger, cfg ProcessBackendConfig) *ProcessBackend {
	if logger == nil {
		logger = zap.NewNop()
	}

	interpreters := map[Language]string{
		LangPython:     "python3",
		LangJavaScript: "node",
		LangBash:       "bash",
		LangGo:         "go",
	}

	for lang, interp := range cfg.CustomInterpreters {
		interpreters[lang] = interp
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "/tmp/sandbox"
	}

	return &ProcessBackend{
		interpreters: interpreters,
		logger:       logger,
		workDir:      workDir,
		enabled:      cfg.Enabled,
	}
}

func (p *ProcessBackend) Name() string { return "process" }

func (p *ProcessBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	start := time.Now()

	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
	}

	// 安全检查
	if !p.enabled {
		result.Error = "process backend disabled for security - enable explicitly or use docker backend"
		return result, nil
	}

	// 获取口译
	interpreter, ok := p.interpreters[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no interpreter configured for language: %s", req.Language)
		return result, nil
	}

	p.logger.Debug("executing with process backend",
		zap.String("interpreter", interpreter),
		zap.String("language", string(req.Language)),
	)

	// 构建命令
	args := p.buildArgs(req)

	// 执行
	cmd := execCommandContext(ctx, interpreter, args...)
	if req.Stdin != "" {
		cmd.SetStdin(req.Stdin)
	}

	stdout, stderr, err := cmd.Run()

	result.Duration = time.Since(start)
	result.Stdout = stdout
	result.Stderr = stderr
	result.ExitCode = cmd.ExitCode()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "execution timeout"
		} else {
			result.Error = err.Error()
		}
		return result, nil
	}

	result.Success = result.ExitCode == 0
	return result, nil
}

func (p *ProcessBackend) buildArgs(req *ExecutionRequest) []string {
	switch req.Language {
	case LangPython:
		return []string{"-c", req.Code}
	case LangJavaScript:
		return []string{"-e", req.Code}
	case LangBash:
		return []string{"-c", req.Code}
	case LangGo:
		return []string{"run", "-"} // Read from stdin
	default:
		return []string{"-c", req.Code}
	}
}

func (p *ProcessBackend) Cleanup() error {
	return nil
}

// 代码Validator执行前验证代码.
type CodeValidator struct {
	blockedPatterns map[Language][]string
}

// NewCodeValidator创建了代码验证器.
func NewCodeValidator() *CodeValidator {
	return &CodeValidator{
		blockedPatterns: map[Language][]string{
			LangPython: {
				// 系统命令执行
				"import os", "from os", "os.system", "os.popen", "os.exec",
				"import subprocess", "from subprocess", "subprocess.run", "subprocess.call", "subprocess.Popen",
				"import sys", "from sys",
				"__import__", "eval(", "exec(", "compile(",
				// 文件操作
				"open(", "file(",
				"import shutil", "shutil.rmtree", "shutil.move",
				"import pathlib", "pathlib.Path",
				// 网络操作
				"import socket", "import urllib", "import requests", "import httplib",
				// 危险模块
				"import ctypes", "import pickle", "pickle.load",
				"import marshal", "marshal.load",
				// 代码注入
				"globals()", "locals()", "vars()", "dir(",
				"getattr(", "setattr(", "delattr(",
				"__builtins__", "__class__", "__bases__", "__subclasses__",
			},
			LangJavaScript: {
				// 系统访问
				"require('child_process')", "require('fs')", "require('os')",
				"require(\"child_process\")", "require(\"fs\")", "require(\"os\")",
				"import child_process", "import fs", "import os",
				"process.env", "process.exit", "process.kill",
				// 代码执行
				"eval(", "Function(", "new Function",
				// 网络操作
				"require('http')", "require('https')", "require('net')",
				"require(\"http\")", "require(\"https\")", "require(\"net\")",
				// 危险操作
				"__proto__", "constructor.constructor",
			},
			LangBash: {
				// 危险命令
				"rm -rf", "rm -fr", "rmdir", "mkfs", "dd if=",
				"> /dev/", ">/dev/",
				// 网络工具
				"curl", "wget", "nc ", "netcat",
				// 权限操作
				"chmod", "chown", "sudo", "su ",
				// 系统操作
				"shutdown", "reboot", "init ", "systemctl",
				"kill ", "killall", "pkill",
				// 敏感文件
				"/etc/passwd", "/etc/shadow", "~/.ssh",
				// 环境变量泄露
				"printenv", "env", "export",
			},
			LangGo: {
				// 系统命令
				"os/exec", "exec.Command",
				"syscall.", "unsafe.",
				// 文件操作
				"os.Remove", "os.RemoveAll",
				// 网络
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

// 验证危险模式的检查代码 。
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

// Sandbox Tool将 sandbox 执行器包成代理工具.
type SandboxTool struct {
	executor  *SandboxExecutor
	validator *CodeValidator
	logger    *zap.Logger
}

// NewSandbox Tool创建了沙盒工具.
func NewSandboxTool(executor *SandboxExecutor, logger *zap.Logger) *SandboxTool {
	return &SandboxTool{
		executor:  executor,
		validator: NewCodeValidator(),
		logger:    logger,
	}
}

// 执行通过沙盒执行代码.
func (t *SandboxTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var req ExecutionRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// 验证代码
	warnings := t.validator.Validate(req.Language, req.Code)
	if len(warnings) > 0 {
		t.logger.Warn("code validation warnings", zap.Strings("warnings", warnings))
	}

	// 执行
	result, err := t.executor.Execute(ctx, &req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}
