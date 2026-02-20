package execution

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RealDockerBackend使用实际的多克CLI执行ExecutiveBackend.
type RealDockerBackend struct {
	*DockerBackend
}

// NewReal DockerBackend创建了一个实际执行代码的Docker后端.
func NewRealDockerBackend(logger *zap.Logger) *RealDockerBackend {
	return &RealDockerBackend{
		DockerBackend: NewDockerBackend(logger),
	}
}

// 执行在真正的多克容器中运行代码 。
func (d *RealDockerBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
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

	// 生成唯一容器名称
	containerName := fmt.Sprintf("%s%s_%d", d.containerPrefix, sanitizeID(req.ID), time.Now().UnixNano())

	// 为代码文件创建临时目录
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp dir: %v", err)
		return result, nil
	}
	defer os.RemoveAll(tempDir)

	// 将代码写入临时文件
	codeFile, err := d.writeCodeFile(tempDir, req)
	if err != nil {
		result.Error = fmt.Sprintf("failed to write code file: %v", err)
		return result, nil
	}

	// 写入额外文件
	for filename, content := range req.Files {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			result.Error = fmt.Sprintf("failed to write file %s: %v", filename, err)
			return result, nil
		}
	}

	// 构建嵌入命令
	args := d.buildRealDockerArgs(containerName, image, tempDir, codeFile, req, config)

	d.logger.Debug("executing docker command",
		zap.String("container", containerName),
		zap.String("image", image),
		zap.Strings("args", args),
	)

	// 跟踪容器
	d.mu.Lock()
	d.activeContainers[containerName] = struct{}{}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.activeContainers, containerName)
		d.mu.Unlock()

		if d.cleanupOnExit {
			d.forceRemoveContainer(containerName)
		}
	}()

	// 执行嵌入器运行
	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	err = cmd.Run()

	result.Duration = time.Since(start)
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "execution timeout"
			d.forceKillContainer(containerName)
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			// 非零退出不一定是一个错误
		} else {
			result.Error = err.Error()
		}
	}

	result.Success = result.ExitCode == 0
	return result, nil
}

func (d *RealDockerBackend) writeCodeFile(tempDir string, req *ExecutionRequest) (string, error) {
	var filename string
	switch req.Language {
	case LangPython:
		filename = "main.py"
	case LangJavaScript:
		filename = "main.js"
	case LangTypeScript:
		filename = "main.ts"
	case LangGo:
		filename = "main.go"
	case LangRust:
		filename = "main.rs"
	case LangBash:
		filename = "script.sh"
	default:
		filename = "code.txt"
	}

	filePath := filepath.Join(tempDir, filename)
	if err := os.WriteFile(filePath, []byte(req.Code), 0644); err != nil {
		return "", err
	}
	return filename, nil
}

func (d *RealDockerBackend) buildRealDockerArgs(containerName, image, tempDir, codeFile string, req *ExecutionRequest, config SandboxConfig) []string {
	args := []string{
		"run",
		"--name", containerName,
		"--rm",
	}

	// 内存限制
	if config.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", config.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", config.MaxMemoryMB))
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
		"--pids-limit", "100",
	)

	// 挂载代码目录
	args = append(args, "-v", fmt.Sprintf("%s:/code:ro", tempDir))
	args = append(args, "-w", "/code")

	// 环境变量
	for k, v := range config.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range req.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// 图像
	args = append(args, image)

	// 基于语言的命令
	cmd := d.buildRealCommand(codeFile, req)
	args = append(args, cmd...)

	return args
}

func (d *RealDockerBackend) buildRealCommand(codeFile string, req *ExecutionRequest) []string {
	switch req.Language {
	case LangPython:
		return []string{"python3", codeFile}
	case LangJavaScript:
		return []string{"node", codeFile}
	case LangTypeScript:
		// 需要 ts- 节点或转接
		return []string{"npx", "ts-node", codeFile}
	case LangGo:
		return []string{"go", "run", codeFile}
	case LangRust:
		return []string{"sh", "-c", fmt.Sprintf("rustc %s -o /tmp/main && /tmp/main", codeFile)}
	case LangBash:
		return []string{"bash", codeFile}
	default:
		return []string{"cat", codeFile}
	}
}

func (d *RealDockerBackend) forceKillContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "kill", name)
	cmd.Run()

	d.logger.Debug("killed container", zap.String("name", name))
}

func (d *RealDockerBackend) forceRemoveContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", name)
	cmd.Run()

	d.logger.Debug("removed container", zap.String("name", name))
}

// 清除所有活动容器。
func (d *RealDockerBackend) Cleanup() error {
	d.mu.Lock()
	containers := make([]string, 0, len(d.activeContainers))
	for name := range d.activeContainers {
		containers = append(containers, name)
	}
	d.mu.Unlock()

	for _, name := range containers {
		d.forceKillContainer(name)
		d.forceRemoveContainer(name)
	}

	d.logger.Info("cleaned up containers", zap.Int("count", len(containers)))
	return nil
}

func sanitizeID(id string) string {
	// 删除容器名称中不允许的字符
	var result strings.Builder
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			result.WriteRune(c)
		}
	}
	s := result.String()
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

// RealProcessBackend 执行 ExecutiveBackend 使用实际的 os/ exec.
type RealProcessBackend struct {
	*ProcessBackend
	validator *CodeValidator
}

// New Real ProcessBackend 创建一个进程后端,可以实际执行代码.
// 警告(Warning):只在可信任的环境中使用,在OS级别有适当的沙箱.
func NewRealProcessBackend(logger *zap.Logger, enabled bool) *RealProcessBackend {
	return &RealProcessBackend{
		ProcessBackend: NewProcessBackendWithConfig(logger, ProcessBackendConfig{
			Enabled: enabled,
		}),
		validator: NewCodeValidator(),
	}
}

// 使用本地进程执行代码 。
func (p *RealProcessBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	start := time.Now()

	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
	}

	if !p.enabled {
		result.Error = "process backend disabled - enable explicitly with NewRealProcessBackend(logger, true)"
		return result, nil
	}

	// 验证危险模式的代码
	warnings := p.validator.Validate(req.Language, req.Code)
	if len(warnings) > 0 {
		p.logger.Warn("code validation warnings",
			zap.Strings("warnings", warnings),
			zap.String("language", string(req.Language)),
		)
		// 如果找到危险模式, 将执行封杀
		result.Error = fmt.Sprintf("code validation failed: %v", warnings)
		return result, nil
	}

	interpreter, ok := p.interpreters[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no interpreter for language: %s", req.Language)
		return result, nil
	}

	// 创建代码的临时文件
	tempDir, err := os.MkdirTemp("", "sandbox_proc_")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp dir: %v", err)
		return result, nil
	}
	defer os.RemoveAll(tempDir)

	codeFile := filepath.Join(tempDir, "code")
	if err := os.WriteFile(codeFile, []byte(req.Code), 0644); err != nil {
		result.Error = fmt.Sprintf("failed to write code: %v", err)
		return result, nil
	}

	// 构建命令
	var cmd *exec.Cmd
	switch req.Language {
	case LangPython:
		cmd = exec.CommandContext(ctx, interpreter, codeFile)
	case LangJavaScript:
		cmd = exec.CommandContext(ctx, interpreter, codeFile)
	case LangBash:
		cmd = exec.CommandContext(ctx, interpreter, codeFile)
	default:
		cmd = exec.CommandContext(ctx, interpreter, "-c", req.Code)
	}

	cmd.Dir = tempDir

	// 设置环境
	cmd.Env = os.Environ()
	for k, v := range config.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range req.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	p.logger.Debug("executing process",
		zap.String("interpreter", interpreter),
		zap.String("language", string(req.Language)),
	)

	err = cmd.Run()

	result.Duration = time.Since(start)
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "execution timeout"
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err.Error()
		}
	}

	result.Success = result.ExitCode == 0
	return result, nil
}

// PullImage 若不显示则会拉出一个多克图像 。
func PullImage(ctx context.Context, image string, logger *zap.Logger) error {
	// 检查图像是否存在
	checkCmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if err := checkCmd.Run(); err == nil {
		return nil // Image exists
	}

	logger.Info("pulling docker image", zap.String("image", image))

	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	return cmd.Run()
}

// 保证图像为沙盒拉出所有所需的图像 。
func EnsureImages(ctx context.Context, languages []Language, logger *zap.Logger) error {
	images := map[Language]string{
		LangPython:     "python:3.12-slim",
		LangJavaScript: "node:20-slim",
		LangTypeScript: "node:20-slim",
		LangGo:         "golang:1.24-alpine",
		LangRust:       "rust:1.75-slim",
		LangBash:       "alpine:latest",
	}

	for _, lang := range languages {
		image, ok := images[lang]
		if !ok {
			continue
		}
		if err := PullImage(ctx, image, logger); err != nil {
			return fmt.Errorf("failed to pull image %s: %w", image, err)
		}
	}

	return nil
}
