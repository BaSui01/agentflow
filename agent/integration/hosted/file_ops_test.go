package hosted

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fp, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := FileOpsConfig{AllowedPaths: []string{dir}, MaxFileSize: 10 << 20}
	tool := NewReadFileTool(cfg)
	args, _ := json.Marshal(map[string]any{"path": fp})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	if out["content"] != "hello world" {
		t.Errorf("got content %q, want %q", out["content"], "hello world")
	}
}

func TestReadFileTool_PathNotAllowed(t *testing.T) {
	dir := t.TempDir()
	cfg := FileOpsConfig{AllowedPaths: []string{dir}}
	tool := NewReadFileTool(cfg)
	args, _ := json.Marshal(map[string]any{"path": "/etc/passwd"})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected path not allowed error")
	}
}

func TestWriteFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "out.txt")
	cfg := FileOpsConfig{AllowedPaths: []string{dir}}
	tool := NewWriteFileTool(cfg)
	args, _ := json.Marshal(map[string]any{"path": fp, "content": "written"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := os.ReadFile(fp)
	if string(content) != "written" {
		t.Errorf("got file content %q, want %q", string(content), "written")
	}
	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	if out["bytes_written"].(float64) != 7 {
		t.Errorf("got bytes_written %v, want 7", out["bytes_written"])
	}
}

func TestEditFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(fp, []byte("foo bar baz"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := FileOpsConfig{AllowedPaths: []string{dir}}
	tool := NewEditFileTool(cfg)
	args, _ := json.Marshal(map[string]any{"path": fp, "old_string": "bar", "new_string": "qux"})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := os.ReadFile(fp)
	if string(content) != "foo qux baz" {
		t.Errorf("got %q, want %q", string(content), "foo qux baz")
	}
}

func TestEditFileTool_OldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(fp, []byte("foo"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := FileOpsConfig{AllowedPaths: []string{dir}}
	tool := NewEditFileTool(cfg)
	args, _ := json.Marshal(map[string]any{"path": fp, "old_string": "missing", "new_string": "x"})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected old_string not found error")
	}
}

func TestListDirectoryTool_Execute(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := FileOpsConfig{AllowedPaths: []string{dir}}
	tool := NewListDirectoryTool(cfg)
	args, _ := json.Marshal(map[string]any{"path": dir})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	entries := out["entries"].([]any)
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}
