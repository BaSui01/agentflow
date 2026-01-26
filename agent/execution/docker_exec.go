// package execution provides the actual Docker execution implementation.
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

// RealDockerBackend implements ExecutionBackend using actual Docker CLI.
type RealDockerBackend struct {
	*DockerBackend
}

// NewRealDockerBackend creates a Docker backend that actually executes code.
func NewRealDockerBackend(logger *zap.Logger) *RealDockerBackend {
	return &RealDockerBackend{
		DockerBackend: NewDockerBackend(logger),
	}
}

// Execute runs code in a real Docker container.
func (d *RealDockerBackend) Execute(ctx context.Context, req *ExecutionRequest, config SandboxConfig) (*ExecutionResult, error) {
	start := time.Now()

	result := &ExecutionResult{
		ID:       req.ID,
		Success:  false,
		ExitCode: -1,
	}

	// Get image for language
	image, ok := d.images[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no image configured for language: %s", req.Language)
		return result, nil
	}

	// Generate unique container name
	containerName := fmt.Sprintf("%s%s_%d", d.containerPrefix, sanitizeID(req.ID), time.Now().UnixNano())

	// Create temp directory for code files
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp dir: %v", err)
		return result, nil
	}
	defer os.RemoveAll(tempDir)

	// Write code to temp file
	codeFile, err := d.writeCodeFile(tempDir, req)
	if err != nil {
		result.Error = fmt.Sprintf("failed to write code file: %v", err)
		return result, nil
	}

	// Write additional files
	for filename, content := range req.Files {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			result.Error = fmt.Sprintf("failed to write file %s: %v", filename, err)
			return result, nil
		}
	}

	// Build docker command
	args := d.buildRealDockerArgs(containerName, image, tempDir, codeFile, req, config)

	d.logger.Debug("executing docker command",
		zap.String("container", containerName),
		zap.String("image", image),
		zap.Strings("args", args),
	)

	// Track container
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

	// Execute docker run
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
			// Non-zero exit is not necessarily an error
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

	// Memory limit
	if config.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", config.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", config.MaxMemoryMB))
	}

	// CPU limit
	if config.MaxCPUPercent > 0 {
		cpus := float64(config.MaxCPUPercent) / 100.0
		args = append(args, "--cpus", fmt.Sprintf("%.2f", cpus))
	}

	// Network
	if !config.NetworkEnabled {
		args = append(args, "--network", "none")
	}

	// Security options
	args = append(args,
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--pids-limit", "100",
	)

	// Mount code directory
	args = append(args, "-v", fmt.Sprintf("%s:/code:ro", tempDir))
	args = append(args, "-w", "/code")

	// Environment variables
	for k, v := range config.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range req.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Image
	args = append(args, image)

	// Command based on language
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
		// Requires ts-node or transpilation
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

// Cleanup removes all active containers.
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
	// Remove characters not allowed in container names
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

// RealProcessBackend implements ExecutionBackend using actual os/exec.
type RealProcessBackend struct {
	*ProcessBackend
	validator *CodeValidator
}

// NewRealProcessBackend creates a process backend that actually executes code.
// WARNING: Only use in trusted environments with proper sandboxing at OS level.
func NewRealProcessBackend(logger *zap.Logger, enabled bool) *RealProcessBackend {
	return &RealProcessBackend{
		ProcessBackend: NewProcessBackendWithConfig(logger, ProcessBackendConfig{
			Enabled: enabled,
		}),
		validator: NewCodeValidator(),
	}
}

// Execute runs code using local process.
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

	// Validate code for dangerous patterns
	warnings := p.validator.Validate(req.Language, req.Code)
	if len(warnings) > 0 {
		p.logger.Warn("code validation warnings",
			zap.Strings("warnings", warnings),
			zap.String("language", string(req.Language)),
		)
		// Block execution if dangerous patterns found
		result.Error = fmt.Sprintf("code validation failed: %v", warnings)
		return result, nil
	}

	interpreter, ok := p.interpreters[req.Language]
	if !ok {
		result.Error = fmt.Sprintf("no interpreter for language: %s", req.Language)
		return result, nil
	}

	// Create temp file for code
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

	// Build command
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

	// Set environment
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

// PullImage pulls a Docker image if not present.
func PullImage(ctx context.Context, image string, logger *zap.Logger) error {
	// Check if image exists
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

// EnsureImages pulls all required images for the sandbox.
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
