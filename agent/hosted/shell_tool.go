package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
)

const (
	defaultShellTimeout     = 30 * time.Second
	defaultMaxOutputSize    = 1 << 20
	maxShellTimeout = 300 * time.Second
)

var defaultBlockedCmds = []string{
	"rm -rf", "rm -fr", "rm -r -f", "format ", "mkfs", "dd if=",
	":(){ :|:& };:", "> /dev/sda",
}

type ShellConfig struct {
	Enabled       bool
	WorkDir       string
	Env           []string
	Timeout       time.Duration
	MaxOutputSize int
	AllowedCmds   []string
	BlockedCmds   []string
}

func (c *ShellConfig) timeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultShellTimeout
	}
	if c.Timeout > maxShellTimeout {
		return maxShellTimeout
	}
	return c.Timeout
}

func (c *ShellConfig) maxOutputSize() int {
	if c.MaxOutputSize <= 0 {
		return defaultMaxOutputSize
	}
	return c.MaxOutputSize
}

func (c *ShellConfig) blockedCmds() []string {
	if len(c.BlockedCmds) > 0 {
		return c.BlockedCmds
	}
	return defaultBlockedCmds
}

func (c *ShellConfig) isBlocked(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, b := range c.blockedCmds() {
		if strings.Contains(lower, strings.ToLower(b)) {
			return true
		}
	}
	return false
}

var dangerousShellPatterns = []string{
	";", "&&", "||", "|", "`",
	"$(", "${", "<(", ">(", "\n",
	">>", "2>", "&>",
}

func (c *ShellConfig) isAllowed(cmd string) bool {
	if len(c.AllowedCmds) == 0 {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, a := range c.AllowedCmds {
		if strings.HasPrefix(lower, strings.ToLower(a)) {
			return true
		}
	}
	return false
}

func (c *ShellConfig) containsDangerousPatterns(cmd string) bool {
	for _, p := range dangerousShellPatterns {
		if strings.Contains(cmd, p) {
			return true
		}
	}
	return false
}

type ShellTool struct {
	cfg *ShellConfig
}

func NewShellTool(cfg ShellConfig) *ShellTool {
	return &ShellTool{cfg: &cfg}
}

func (t *ShellTool) Type() HostedToolType { return ToolTypeShell }
func (t *ShellTool) Name() string         { return "run_command" }
func (t *ShellTool) Description() string  { return "Execute a shell command" }

func (t *ShellTool) Schema() types.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute",
			},
			"working_directory": map[string]any{
				"type":        "string",
				"description": "Working directory for the command",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds",
			},
		},
		"required": []string{"command"},
	})
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

type runCommandArgs struct {
	Command           string `json:"command"`
	WorkingDirectory  string `json:"working_directory,omitempty"`
	TimeoutSeconds    int    `json:"timeout_seconds,omitempty"`
}

type runCommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func (t *ShellTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if !t.cfg.Enabled {
		return nil, fmt.Errorf("shell tool is disabled; set Enabled=true in ShellConfig to use")
	}
	var a runCommandArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if t.cfg.isBlocked(a.Command) {
		return nil, fmt.Errorf("command is blocked: %s", a.Command)
	}
	if t.cfg.containsDangerousPatterns(a.Command) {
		return nil, fmt.Errorf("command contains dangerous shell patterns")
	}
	if !t.cfg.isAllowed(a.Command) {
		return nil, fmt.Errorf("command not in allowed list")
	}

	timeout := t.cfg.timeout()
	if a.TimeoutSeconds > 0 {
		timeout = time.Duration(a.TimeoutSeconds) * time.Second
		if timeout > maxShellTimeout {
			timeout = maxShellTimeout
		}
	}

	workDir := t.cfg.WorkDir
	if a.WorkingDirectory != "" {
		if strings.Contains(a.WorkingDirectory, "..") {
			return nil, fmt.Errorf("working directory must not contain path traversal")
		}
		workDir = a.WorkingDirectory
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", a.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", a.Command)
	}
	cmd.Dir = workDir
	cmd.Env = t.cfg.Env

	var stdout, stderr bytes.Buffer
	maxOut := t.cfg.maxOutputSize()
	cmd.Stdout = &limitedWriter{w: &stdout, max: maxOut}
	cmd.Stderr = &limitedWriter{w: &stderr, max: maxOut}

	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("command timed out: %w", ctx.Err())
	}
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("command failed: %w", err)
		}
	}

	return json.Marshal(runCommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	})
}

type limitedWriter struct {
	w   *bytes.Buffer
	max int
	n   int
}

func (l *limitedWriter) Write(p []byte) (n int, err error) {
	remain := l.max - l.n
	if remain <= 0 {
		return len(p), nil
	}
	if len(p) > remain {
		p = p[:remain]
	}
	n, err = l.w.Write(p)
	l.n += n
	return n, err
}
