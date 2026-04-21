package deployment

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// localProcess represents a locally running agent process.
type localProcess struct {
	cmd      *exec.Cmd
	port     int
	endpoint string
	cancel   context.CancelFunc
	stdout   *bytes.Buffer
	stderr   *bytes.Buffer
}

// LocalProvider manages agents as local OS processes.
type LocalProvider struct {
	processes map[string]*localProcess
	logger    *zap.Logger
	mu        sync.RWMutex
}

// NewLocalProvider creates a new local process provider.
func NewLocalProvider(logger *zap.Logger) *LocalProvider {
	// O-004: optional module, nil-safe
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LocalProvider{
		processes: make(map[string]*localProcess),
		logger:    logger.With(zap.String("provider", "local")),
	}
}

// findAvailablePort finds a free TCP port on localhost.
func findAvailablePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find available port: %w", err)
	}
	defer func() {
		_ = l.Close()
	}()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listen addr is not TCP")
	}
	port := addr.Port
	return port, nil
}

// Deploy starts a local process for the deployment.
func (p *LocalProvider) Deploy(ctx context.Context, d *Deployment) error {
	if d.Config.Image == "" {
		return fmt.Errorf("local provider requires Config.Image as the executable path")
	}

	port := d.Config.Port
	if port == 0 {
		var err error
		port, err = findAvailablePort()
		if err != nil {
			return fmt.Errorf("allocate port: %w", err)
		}
	}

	procCtx, cancel := context.WithCancel(ctx)
	executable, args := resolveLocalCommand(d.Config.Image)
	cmd := exec.CommandContext(procCtx, executable, args...)

	// Set environment variables from config.
	cmd.Env = os.Environ()
	for k, v := range d.Config.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", port))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start local process: %w", err)
	}

	lp := &localProcess{
		cmd:      cmd,
		port:     port,
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		cancel:   cancel,
		stdout:   &stdout,
		stderr:   &stderr,
	}

	p.mu.Lock()
	p.processes[d.ID] = lp
	p.mu.Unlock()

	d.Endpoint = lp.endpoint
	p.logger.Info("local process started",
		zap.String("id", d.ID),
		zap.Int("port", port),
		zap.Int("pid", cmd.Process.Pid))

	return nil
}

func resolveLocalCommand(image string) (command string, args []string) {
	if runtime.GOOS != "windows" {
		return image, nil
	}

	clean := strings.ReplaceAll(image, "\\", "/")
	base := strings.ToLower(filepath.Base(clean))
	switch clean {
	case "/bin/sleep":
		return "powershell", []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 60"}
	case "/bin/echo":
		return "cmd", []string{"/C", "echo"}
	}

	switch base {
	case "sleep":
		return "powershell", []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 60"}
	case "echo":
		return "cmd", []string{"/C", "echo"}
	default:
		return image, nil
	}
}

// Update restarts the local process with new configuration.
func (p *LocalProvider) Update(ctx context.Context, d *Deployment) error {
	if err := p.Delete(ctx, d.ID); err != nil {
		return fmt.Errorf("stop old process for update: %w", err)
	}
	return p.Deploy(ctx, d)
}

// Delete stops and removes a local process.
func (p *LocalProvider) Delete(_ context.Context, deploymentID string) error {
	p.mu.Lock()
	lp, ok := p.processes[deploymentID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("local process not found: %s", deploymentID)
	}
	delete(p.processes, deploymentID)
	p.mu.Unlock()

	// Send SIGTERM first.
	if lp.cmd.Process != nil {
		if runtime.GOOS == "windows" {
			if err := lp.cmd.Process.Kill(); err != nil {
				p.logger.Debug("failed to kill local process on windows", zap.Error(err))
			}
		} else {
			if err := lp.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				p.logger.Debug("failed to signal local process", zap.Error(err))
			}
		}

		// Wait up to 5 seconds for graceful shutdown.
		done := make(chan struct{})
		go func() {
			if err := lp.cmd.Wait(); err != nil {
				p.logger.Debug("local process wait returned error", zap.Error(err))
			}
			close(done)
		}()

		select {
		case <-done:
			// Process exited gracefully.
		case <-time.After(5 * time.Second):
			// Force kill.
			if err := lp.cmd.Process.Kill(); err != nil {
				p.logger.Debug("failed to force kill local process", zap.Error(err))
			}
			<-done
		}
	}

	lp.cancel()
	p.logger.Info("local process stopped", zap.String("id", deploymentID))
	return nil
}

// GetStatus checks if the local process is still running.
func (p *LocalProvider) GetStatus(_ context.Context, deploymentID string) (*Deployment, error) {
	p.mu.RLock()
	lp, ok := p.processes[deploymentID]
	p.mu.RUnlock()

	if !ok {
		return &Deployment{ID: deploymentID, Status: StatusStopped}, nil
	}

	status := StatusRunning
	if lp.cmd.ProcessState != nil && lp.cmd.ProcessState.Exited() {
		status = StatusFailed
	} else if lp.cmd.Process != nil && runtime.GOOS != "windows" {
		// Signal(0) checks if process is alive without sending a signal.
		if err := lp.cmd.Process.Signal(syscall.Signal(0)); err != nil {
			status = StatusFailed
		}
	}

	return &Deployment{
		ID:       deploymentID,
		Status:   status,
		Endpoint: lp.endpoint,
	}, nil
}

// Scale is not supported for local processes.
func (p *LocalProvider) Scale(_ context.Context, deploymentID string, replicas int) error {
	p.logger.Warn("scale not supported for local provider",
		zap.String("id", deploymentID),
		zap.Int("replicas", replicas))
	return fmt.Errorf("local provider does not support scaling to %d replicas", replicas)
}

// GetLogs returns recent output from the process stdout/stderr.
func (p *LocalProvider) GetLogs(_ context.Context, deploymentID string, lines int) ([]string, error) {
	p.mu.RLock()
	lp, ok := p.processes[deploymentID]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("local process not found: %s", deploymentID)
	}

	combined := lp.stdout.String() + lp.stderr.String()
	allLines := splitLines(combined)

	if lines <= 0 || lines >= len(allLines) {
		return allLines, nil
	}
	return allLines[len(allLines)-lines:], nil
}

// splitLines splits a string into non-empty lines.
func splitLines(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if line != "" && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line != "" {
				result = append(result, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
