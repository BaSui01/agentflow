package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

const defaultMaxFileSize = 10 << 20

type FileOpsConfig struct {
	AllowedPaths []string
	MaxFileSize  int64
}

func (c *FileOpsConfig) maxFileSize() int64 {
	if c.MaxFileSize <= 0 {
		return defaultMaxFileSize
	}
	return c.MaxFileSize
}

func (c *FileOpsConfig) resolveAndValidate(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	abs = filepath.Clean(abs)
	if len(c.AllowedPaths) == 0 {
		return "", fmt.Errorf("file operations require at least one allowed path to be configured")
	}
	for _, p := range c.AllowedPaths {
		prefix := filepath.Clean(p)
		if prefix == "" || prefix == "." {
			continue
		}
		absPrefix, err := filepath.Abs(prefix)
		if err != nil {
			continue
		}
		if abs == absPrefix {
			return abs, nil
		}
		if strings.HasPrefix(abs, absPrefix+string(filepath.Separator)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("path not allowed: %s", path)
}

type ReadFileTool struct {
	cfg *FileOpsConfig
}

func NewReadFileTool(cfg FileOpsConfig) *ReadFileTool {
	return &ReadFileTool{cfg: &cfg}
}

func (t *ReadFileTool) Type() HostedToolType { return ToolTypeFileOps }
func (t *ReadFileTool) Name() string         { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read file content" }

func (t *ReadFileTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "File path"},
			"offset": map[string]any{"type": "integer", "description": "Byte offset"},
			"limit":  map[string]any{"type": "integer", "description": "Max bytes to read"},
		},
		"required": []string{"path"},
	})
	if err != nil {
		return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: []byte("{}")}
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	resolved, err := t.cfg.resolveAndValidate(a.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat failed: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	if info.Size() > t.cfg.maxFileSize() {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), t.cfg.maxFileSize())
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}
	defer f.Close()
	offset := a.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > 0 {
		_, err = f.Seek(int64(offset), io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("seek failed: %w", err)
		}
	}
	limit := a.Limit
	if limit <= 0 {
		limit = int(t.cfg.maxFileSize())
	}
	if limit > int(t.cfg.maxFileSize()) {
		limit = int(t.cfg.maxFileSize())
	}
	content, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	data, err := json.Marshal(map[string]any{"content": string(content), "size": len(content)})
	if err != nil {
		return nil, fmt.Errorf("marshal read_file result: %w", err)
	}
	return data, nil
}

type WriteFileTool struct {
	cfg *FileOpsConfig
}

func NewWriteFileTool(cfg FileOpsConfig) *WriteFileTool {
	return &WriteFileTool{cfg: &cfg}
}

func (t *WriteFileTool) Type() HostedToolType { return ToolTypeFileOps }
func (t *WriteFileTool) Name() string         { return "write_file" }
func (t *WriteFileTool) Description() string { return "Write content to file" }

func (t *WriteFileTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path"},
			"content": map[string]any{"type": "string", "description": "Content to write"},
		},
		"required": []string{"path", "content"},
	})
	if err != nil {
		return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: []byte("{}")}
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	resolved, err := t.cfg.resolveAndValidate(a.Path)
	if err != nil {
		return nil, err
	}
	if int64(len(a.Content)) > t.cfg.maxFileSize() {
		return nil, fmt.Errorf("content too large: %d bytes (max %d)", len(a.Content), t.cfg.maxFileSize())
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir failed: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(a.Content), 0644); err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}
	data, err := json.Marshal(map[string]any{"path": resolved, "bytes_written": len(a.Content)})
	if err != nil {
		return nil, fmt.Errorf("marshal write_file result: %w", err)
	}
	return data, nil
}

type EditFileTool struct {
	cfg *FileOpsConfig
}

func NewEditFileTool(cfg FileOpsConfig) *EditFileTool {
	return &EditFileTool{cfg: &cfg}
}

func (t *EditFileTool) Type() HostedToolType { return ToolTypeFileOps }
func (t *EditFileTool) Name() string         { return "edit_file" }
func (t *EditFileTool) Description() string { return "Replace old_string with new_string in file" }

func (t *EditFileTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "File path"},
			"old_string": map[string]any{"type": "string", "description": "Exact string to replace"},
			"new_string": map[string]any{"type": "string", "description": "Replacement string"},
		},
		"required": []string{"path", "old_string", "new_string"},
	})
	if err != nil {
		return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: []byte("{}")}
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

type editFileArgs struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	resolved, err := t.cfg.resolveAndValidate(a.Path)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	if int64(len(content)) > t.cfg.maxFileSize() {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", len(content), t.cfg.maxFileSize())
	}
	s := string(content)
	if !strings.Contains(s, a.OldString) {
		return nil, fmt.Errorf("old_string not found in file")
	}
	newContent := strings.Replace(s, a.OldString, a.NewString, 1)
	if int64(len(newContent)) > t.cfg.maxFileSize() {
		return nil, fmt.Errorf("result too large")
	}
	if err := os.WriteFile(resolved, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}
	data, err := json.Marshal(map[string]any{"path": resolved, "replacements": 1})
	if err != nil {
		return nil, fmt.Errorf("marshal edit_file result: %w", err)
	}
	return data, nil
}

type ListDirectoryTool struct {
	cfg *FileOpsConfig
}

func NewListDirectoryTool(cfg FileOpsConfig) *ListDirectoryTool {
	return &ListDirectoryTool{cfg: &cfg}
}

func (t *ListDirectoryTool) Type() HostedToolType { return ToolTypeFileOps }
func (t *ListDirectoryTool) Name() string         { return "list_directory" }
func (t *ListDirectoryTool) Description() string { return "List directory contents" }

func (t *ListDirectoryTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Directory path"},
			"recursive": map[string]any{"type": "boolean", "description": "List recursively"},
			"max_depth": map[string]any{"type": "integer", "description": "Max recursion depth"},
		},
		"required": []string{"path"},
	})
	if err != nil {
		return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: []byte("{}")}
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

type listDirArgs struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
	MaxDepth  int    `json:"max_depth,omitempty"`
}

type listEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

func (t *ListDirectoryTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a listDirArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	resolved, err := t.cfg.resolveAndValidate(a.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat failed: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}
	maxDepth := a.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 1
	}
	var entries []listEntry
	if a.Recursive {
		err = filepath.Walk(resolved, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(resolved, p)
			depth := strings.Count(rel, string(filepath.Separator))
			if rel != "." {
				depth++
			}
			if depth > maxDepth {
				if fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			entries = append(entries, listEntry{Name: fi.Name(), Path: p, IsDir: fi.IsDir()})
			return nil
		})
	} else {
		items, err := os.ReadDir(resolved)
		if err != nil {
			return nil, fmt.Errorf("readdir failed: %w", err)
		}
		for _, item := range items {
			full := filepath.Join(resolved, item.Name())
			entries = append(entries, listEntry{Name: item.Name(), Path: full, IsDir: item.IsDir()})
		}
	}
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]any{"entries": entries})
	if err != nil {
		return nil, fmt.Errorf("marshal list_directory result: %w", err)
	}
	return data, nil
}
