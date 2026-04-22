package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// FileCheckpointStore persists agent checkpoints on the local filesystem.
type FileCheckpointStore struct {
	basePath string
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewFileCheckpointStore creates a filesystem-backed checkpoint store.
func NewFileCheckpointStore(basePath string, logger *zap.Logger) (*FileCheckpointStore, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &FileCheckpointStore{
		basePath: basePath,
		logger:   checkpointLogger(logger, "file_checkpoint"),
	}, nil
}

// Save persists a checkpoint.
func (s *FileCheckpointStore) Save(_ context.Context, checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(checkpoint)
}

func (s *FileCheckpointStore) saveLocked(checkpoint *Checkpoint) error {
	if checkpoint.Version == 0 {
		versions, err := s.listVersionsUnlocked(checkpoint.ThreadID)
		if err == nil && len(versions) > 0 {
			maxVersion := 0
			for _, v := range versions {
				if v.Version > maxVersion {
					maxVersion = v.Version
				}
			}
			checkpoint.Version = maxVersion + 1
		} else {
			checkpoint.Version = 1
		}
	}

	threadDir := s.threadDir(checkpoint.ThreadID)
	if err := os.MkdirAll(threadDir, 0o755); err != nil {
		return fmt.Errorf("failed to create thread directory: %w", err)
	}

	checkpointsDir := filepath.Join(threadDir, "checkpoints")
	if err := os.MkdirAll(checkpointsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create checkpoints directory: %w", err)
	}

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	checkpointFile := filepath.Join(checkpointsDir, fmt.Sprintf("%s.json", checkpoint.ID))
	if err := os.WriteFile(checkpointFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	if err := s.updateVersionsIndex(checkpoint.ThreadID, checkpoint); err != nil {
		return fmt.Errorf("failed to update versions index: %w", err)
	}

	latestFile := filepath.Join(threadDir, "latest.txt")
	if err := os.WriteFile(latestFile, []byte(checkpoint.ID), 0o600); err != nil {
		return fmt.Errorf("failed to update latest file: %w", err)
	}

	s.logger.Debug("checkpoint saved to file",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Load retrieves a checkpoint by ID.
func (s *FileCheckpointStore) Load(_ context.Context, checkpointID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threadsDir := filepath.Join(s.basePath, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
		}
		return nil, fmt.Errorf("failed to read threads directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		checkpointFile := filepath.Join(threadsDir, entry.Name(), "checkpoints", fmt.Sprintf("%s.json", checkpointID))
		if _, err := os.Stat(checkpointFile); err == nil {
			return s.loadCheckpointFile(checkpointFile)
		}
	}

	return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
}

// LoadLatest retrieves the latest checkpoint for a thread.
func (s *FileCheckpointStore) LoadLatest(_ context.Context, threadID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threadDir := s.threadDir(threadID)
	latestFile := filepath.Join(threadDir, "latest.txt")

	data, err := os.ReadFile(latestFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no checkpoints found for thread: %s", threadID)
		}
		return nil, fmt.Errorf("failed to read latest file: %w", err)
	}

	checkpointID := string(data)
	checkpointFile := filepath.Join(threadDir, "checkpoints", fmt.Sprintf("%s.json", checkpointID))
	return s.loadCheckpointFile(checkpointFile)
}

// List enumerates checkpoints for a thread.
func (s *FileCheckpointStore) List(_ context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	checkpointsDir := filepath.Join(s.threadDir(threadID), "checkpoints")
	entries, err := os.ReadDir(checkpointsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Checkpoint{}, nil
		}
		return nil, fmt.Errorf("failed to read checkpoints directory: %w", err)
	}

	checkpoints := make([]*Checkpoint, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		checkpointFile := filepath.Join(checkpointsDir, entry.Name())
		checkpoint, err := s.loadCheckpointFile(checkpointFile)
		if err != nil {
			s.logger.Warn("failed to load checkpoint file", zap.String("file", checkpointFile), zap.Error(err))
			continue
		}
		checkpoints = append(checkpoints, checkpoint)
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].CreatedAt.After(checkpoints[j].CreatedAt)
	})

	if limit > 0 && len(checkpoints) > limit {
		checkpoints = checkpoints[:limit]
	}

	return checkpoints, nil
}

// Delete removes a checkpoint by ID.
func (s *FileCheckpointStore) Delete(_ context.Context, checkpointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	threadsDir := filepath.Join(s.basePath, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read threads directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		checkpointFile := filepath.Join(threadsDir, entry.Name(), "checkpoints", fmt.Sprintf("%s.json", checkpointID))
		if _, err := os.Stat(checkpointFile); err == nil {
			if err := os.Remove(checkpointFile); err != nil {
				return fmt.Errorf("failed to delete checkpoint file: %w", err)
			}
			s.logger.Debug("checkpoint deleted",
				zap.String("checkpoint_id", checkpointID),
				zap.String("thread_id", entry.Name()),
			)
			return nil
		}
	}

	return nil
}

// DeleteThread removes all checkpoints for a thread.
func (s *FileCheckpointStore) DeleteThread(_ context.Context, threadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	threadDir := s.threadDir(threadID)
	if err := os.RemoveAll(threadDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to delete thread directory: %w", err)
	}

	s.logger.Debug("thread deleted", zap.String("thread_id", threadID))
	return nil
}

// LoadVersion retrieves a checkpoint by thread/version.
func (s *FileCheckpointStore) LoadVersion(_ context.Context, threadID string, version int) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, err := s.listVersionsUnlocked(threadID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	for _, v := range versions {
		if v.Version == version {
			checkpointFile := filepath.Join(s.threadDir(threadID), "checkpoints", fmt.Sprintf("%s.json", v.ID))
			return s.loadCheckpointFile(checkpointFile)
		}
	}

	return nil, fmt.Errorf("version %d not found for thread %s", version, threadID)
}

// ListVersions lists all versions for a thread.
func (s *FileCheckpointStore) ListVersions(_ context.Context, threadID string) ([]CheckpointVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listVersionsUnlocked(threadID)
}

// Rollback creates a new checkpoint based on a historical version.
func (s *FileCheckpointStore) Rollback(_ context.Context, threadID string, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	versions, err := s.listVersionsUnlocked(threadID)
	if err != nil {
		return fmt.Errorf("list versions for rollback: %w", err)
	}

	var targetCheckpoint *Checkpoint
	for _, v := range versions {
		if v.Version == version {
			checkpointFile := filepath.Join(s.threadDir(threadID), "checkpoints", fmt.Sprintf("%s.json", v.ID))
			targetCheckpoint, err = s.loadCheckpointFile(checkpointFile)
			if err != nil {
				return fmt.Errorf("load checkpoint file for rollback: %w", err)
			}
			break
		}
	}

	if targetCheckpoint == nil {
		return fmt.Errorf("version %d not found for thread %s", version, threadID)
	}

	newCheckpoint := *targetCheckpoint
	newCheckpoint.ID = nextCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = targetCheckpoint.ID

	maxVersion := 0
	for _, v := range versions {
		if v.Version > maxVersion {
			maxVersion = v.Version
		}
	}
	newCheckpoint.Version = maxVersion + 1

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from_version"] = version

	return s.saveLocked(&newCheckpoint)
}

func (s *FileCheckpointStore) threadDir(threadID string) string {
	return filepath.Join(s.basePath, "threads", threadID)
}

func (s *FileCheckpointStore) loadCheckpointFile(filePath string) (*Checkpoint, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

func (s *FileCheckpointStore) updateVersionsIndex(threadID string, checkpoint *Checkpoint) error {
	versionsFile := filepath.Join(s.threadDir(threadID), "versions.json")

	var versions []CheckpointVersion
	if data, err := os.ReadFile(versionsFile); err == nil {
		if err := json.Unmarshal(data, &versions); err != nil {
			return fmt.Errorf("failed to unmarshal versions: %w", err)
		}
	}

	found := false
	for i, v := range versions {
		if v.Version == checkpoint.Version {
			versions[i] = CheckpointVersion{
				Version:   checkpoint.Version,
				ID:        checkpoint.ID,
				CreatedAt: checkpoint.CreatedAt,
				State:     checkpoint.State,
				Summary:   fmt.Sprintf("Checkpoint at %s", checkpoint.CreatedAt.Format(time.RFC3339)),
			}
			found = true
			break
		}
	}

	if !found {
		versions = append(versions, CheckpointVersion{
			Version:   checkpoint.Version,
			ID:        checkpoint.ID,
			CreatedAt: checkpoint.CreatedAt,
			State:     checkpoint.State,
			Summary:   fmt.Sprintf("Checkpoint at %s", checkpoint.CreatedAt.Format(time.RFC3339)),
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	data, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal versions: %w", err)
	}

	if err := os.WriteFile(versionsFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write versions file: %w", err)
	}

	return nil
}

func (s *FileCheckpointStore) listVersionsUnlocked(threadID string) ([]CheckpointVersion, error) {
	versionsFile := filepath.Join(s.threadDir(threadID), "versions.json")

	data, err := os.ReadFile(versionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []CheckpointVersion{}, nil
		}
		return nil, fmt.Errorf("failed to read versions file: %w", err)
	}

	var versions []CheckpointVersion
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal versions: %w", err)
	}

	return versions, nil
}
