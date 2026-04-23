package hosted

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShellTool_Execute_DisabledByDefault(t *testing.T) {
	tool := NewShellTool(ShellConfig{})
	args, _ := json.Marshal(map[string]any{"command": "echo test"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled error, got: %v", err)
	}
}

func TestShellTool_Execute_Echo(t *testing.T) {
	cmd := "echo hello"
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	}
	tool := NewShellTool(ShellConfig{Enabled: true})
	args, _ := json.Marshal(map[string]any{"command": cmd})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out runCommandResult
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 0 {
		t.Errorf("got exit_code %d, want 0", out.ExitCode)
	}
	if !strings.Contains(out.Stdout, "hello") && !strings.Contains(out.Stderr, "hello") {
		t.Errorf("expected 'hello' in output, got stdout=%q stderr=%q", out.Stdout, out.Stderr)
	}
}

func TestShellTool_Execute_BlockedCommand(t *testing.T) {
	tool := NewShellTool(ShellConfig{Enabled: true})
	args, _ := json.Marshal(map[string]any{"command": "rm -rf /"})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected blocked command error")
	}
}

func TestShellTool_Execute_Timeout(t *testing.T) {
	cmd := "sleep 10"
	if runtime.GOOS == "windows" {
		cmd = "powershell -Command \"Start-Sleep 10\""
	}
	tool := NewShellTool(ShellConfig{Enabled: true, Timeout: 50 * time.Millisecond})
	args, _ := json.Marshal(map[string]any{"command": cmd})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestShellTool_Execute_EmptyCommand(t *testing.T) {
	tool := NewShellTool(ShellConfig{Enabled: true})
	args, _ := json.Marshal(map[string]any{"command": ""})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected command required error")
	}
}
