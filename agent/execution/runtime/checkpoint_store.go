package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// CheckpointStore abstracts checkpoint persistence.
// Default implementation uses filesystem (existing behavior).
// Persistence-backed implementation uses TaskStoreAdapter.
type CheckpointStore interface {
	SaveCheckpoint(ctx context.Context, exec *Execution) error
	LoadCheckpoint(ctx context.Context, execID string) (*Execution, error)
	ListCheckpoints(ctx context.Context) ([]*Execution, error)
	DeleteCheckpoint(ctx context.Context, execID string) error
}

// FileCheckpointStore is the default filesystem-based implementation.
// This extracts the existing os.WriteFile/os.ReadFile behavior from executor.go.
type FileCheckpointStore struct {
	dir    string
	logger *zap.Logger
}

// NewFileCheckpointStore creates a new filesystem-based checkpoint store.
func NewFileCheckpointStore(dir string, logger *zap.Logger) *FileCheckpointStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FileCheckpointStore{
		dir:    dir,
		logger: logger,
	}
}

// SaveCheckpoint persists execution state to a JSON file on disk.
func (s *FileCheckpointStore) SaveCheckpoint(_ context.Context, exec *Execution) error {
	exec.mu.Lock()
	data, err := json.Marshal(exec)
	exec.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshaling checkpoint: %w", err)
	}

	path := filepath.Join(s.dir, exec.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing checkpoint file: %w", err)
	}
	return nil
}

// LoadCheckpoint reads an execution from a JSON file on disk.
func (s *FileCheckpointStore) LoadCheckpoint(_ context.Context, execID string) (*Execution, error) {
	path := filepath.Join(s.dir, execID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading checkpoint file: %w", err)
	}

	var exec Execution
	if err := json.Unmarshal(data, &exec); err != nil {
		return nil, fmt.Errorf("unmarshaling checkpoint: %w", err)
	}
	return &exec, nil
}

// ListCheckpoints reads all checkpoint files from the directory.
func (s *FileCheckpointStore) ListCheckpoints(_ context.Context) ([]*Execution, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("reading checkpoint directory: %w", err)
	}

	var execs []*Execution
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			s.logger.Warn("skipping unreadable checkpoint file",
				zap.String("path", path), zap.Error(err))
			continue
		}
		var exec Execution
		if err := json.Unmarshal(data, &exec); err != nil {
			s.logger.Warn("skipping malformed checkpoint file",
				zap.String("path", path), zap.Error(err))
			continue
		}
		execs = append(execs, &exec)
	}
	return execs, nil
}

// DeleteCheckpoint removes a checkpoint file from disk.
func (s *FileCheckpointStore) DeleteCheckpoint(_ context.Context, execID string) error {
	path := filepath.Join(s.dir, execID+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing checkpoint file: %w", err)
	}
	return nil
}
