// Package workflow provides enhanced checkpoint capabilities for DAG workflows.
// Implements LangGraph-style persistent checkpointing with time-travel debugging.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EnhancedCheckpoint represents a workflow checkpoint with full state.
type EnhancedCheckpoint struct {
	ID             string         `json:"id"`
	WorkflowID     string         `json:"workflow_id"`
	ThreadID       string         `json:"thread_id"`
	Version        int            `json:"version"`
	NodeID         string         `json:"node_id"`
	NodeResults    map[string]any `json:"node_results"`
	Variables      map[string]any `json:"variables"`
	PendingNodes   []string       `json:"pending_nodes"`
	CompletedNodes []string       `json:"completed_nodes"`
	Input          any            `json:"input"`
	CreatedAt      time.Time      `json:"created_at"`
	ParentID       string         `json:"parent_id,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Snapshot       *GraphSnapshot `json:"snapshot,omitempty"`
}

// GraphSnapshot captures the complete graph state.
type GraphSnapshot struct {
	Nodes     map[string]NodeSnapshot `json:"nodes"`
	Edges     map[string][]string     `json:"edges"`
	EntryNode string                  `json:"entry_node"`
}

// NodeSnapshot captures a node's state.
type NodeSnapshot struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Input    any    `json:"input,omitempty"`
	Output   any    `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms,omitempty"`
}

// CheckpointStore defines storage interface for enhanced checkpoints.
type CheckpointStore interface {
	Save(ctx context.Context, checkpoint *EnhancedCheckpoint) error
	Load(ctx context.Context, checkpointID string) (*EnhancedCheckpoint, error)
	LoadLatest(ctx context.Context, threadID string) (*EnhancedCheckpoint, error)
	LoadVersion(ctx context.Context, threadID string, version int) (*EnhancedCheckpoint, error)
	ListVersions(ctx context.Context, threadID string) ([]*EnhancedCheckpoint, error)
	Delete(ctx context.Context, checkpointID string) error
}

// EnhancedCheckpointManager manages workflow checkpoints with time-travel.
type EnhancedCheckpointManager struct {
	store  CheckpointStore
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewEnhancedCheckpointManager creates a new checkpoint manager.
func NewEnhancedCheckpointManager(store CheckpointStore, logger *zap.Logger) *EnhancedCheckpointManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EnhancedCheckpointManager{
		store:  store,
		logger: logger.With(zap.String("component", "checkpoint_manager")),
	}
}

// CreateCheckpoint creates a checkpoint from current execution state.
func (m *EnhancedCheckpointManager) CreateCheckpoint(ctx context.Context, executor *DAGExecutor, graph *DAGGraph, threadID string, input any) (*EnhancedCheckpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get latest version
	version := 1
	latest, err := m.store.LoadLatest(ctx, threadID)
	if err == nil && latest != nil {
		version = latest.Version + 1
	}

	checkpoint := &EnhancedCheckpoint{
		ID:          generateCheckpointID(),
		WorkflowID:  executor.GetExecutionID(),
		ThreadID:    threadID,
		Version:     version,
		NodeResults: make(map[string]any),
		Variables:   make(map[string]any),
		Input:       input,
		CreatedAt:   time.Now(),
		Metadata:    make(map[string]any),
	}

	// Capture node results
	for nodeID := range graph.Nodes() {
		if result, ok := executor.GetNodeResult(nodeID); ok {
			checkpoint.NodeResults[nodeID] = result
			checkpoint.CompletedNodes = append(checkpoint.CompletedNodes, nodeID)
		} else {
			checkpoint.PendingNodes = append(checkpoint.PendingNodes, nodeID)
		}
	}

	// Create graph snapshot
	checkpoint.Snapshot = m.createSnapshot(graph, executor)

	if latest != nil {
		checkpoint.ParentID = latest.ID
	}

	if err := m.store.Save(ctx, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to save checkpoint: %w", err)
	}

	m.logger.Info("checkpoint created",
		zap.String("id", checkpoint.ID),
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	return checkpoint, nil
}

func (m *EnhancedCheckpointManager) createSnapshot(graph *DAGGraph, executor *DAGExecutor) *GraphSnapshot {
	snapshot := &GraphSnapshot{
		Nodes:     make(map[string]NodeSnapshot),
		Edges:     graph.Edges(),
		EntryNode: graph.GetEntry(),
	}

	for nodeID, node := range graph.Nodes() {
		nodeSnap := NodeSnapshot{
			ID:   nodeID,
			Type: string(node.Type),
		}

		if result, ok := executor.GetNodeResult(nodeID); ok {
			nodeSnap.Status = "completed"
			nodeSnap.Output = result
		} else {
			nodeSnap.Status = "pending"
		}

		snapshot.Nodes[nodeID] = nodeSnap
	}

	return snapshot
}

// ResumeFromCheckpoint resumes workflow execution from a checkpoint.
func (m *EnhancedCheckpointManager) ResumeFromCheckpoint(ctx context.Context, checkpointID string, graph *DAGGraph) (*DAGExecutor, error) {
	checkpoint, err := m.store.Load(ctx, checkpointID)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	executor := NewDAGExecutor(nil, m.logger)

	// Restore node results
	for nodeID, result := range checkpoint.NodeResults {
		executor.mu.Lock()
		executor.nodeResults[nodeID] = result
		executor.visitedNodes[nodeID] = true
		executor.mu.Unlock()
	}

	m.logger.Info("resumed from checkpoint",
		zap.String("checkpoint_id", checkpointID),
		zap.Int("version", checkpoint.Version),
		zap.Int("completed_nodes", len(checkpoint.CompletedNodes)),
	)

	return executor, nil
}

// Rollback rolls back to a specific version.
func (m *EnhancedCheckpointManager) Rollback(ctx context.Context, threadID string, version int) (*EnhancedCheckpoint, error) {
	checkpoint, err := m.store.LoadVersion(ctx, threadID, version)
	if err != nil {
		return nil, fmt.Errorf("failed to load version %d: %w", version, err)
	}

	// Create new checkpoint as rollback point
	newCheckpoint := *checkpoint
	newCheckpoint.ID = generateCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = checkpoint.ID

	latest, _ := m.store.LoadLatest(ctx, threadID)
	if latest != nil {
		newCheckpoint.Version = latest.Version + 1
	}

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from"] = version

	if err := m.store.Save(ctx, &newCheckpoint); err != nil {
		return nil, err
	}

	m.logger.Info("rolled back to version",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	return &newCheckpoint, nil
}

// GetHistory returns checkpoint history for time-travel debugging.
func (m *EnhancedCheckpointManager) GetHistory(ctx context.Context, threadID string) ([]*EnhancedCheckpoint, error) {
	return m.store.ListVersions(ctx, threadID)
}

// Compare compares two checkpoint versions.
func (m *EnhancedCheckpointManager) Compare(ctx context.Context, threadID string, v1, v2 int) (*CheckpointDiff, error) {
	cp1, err := m.store.LoadVersion(ctx, threadID, v1)
	if err != nil {
		return nil, err
	}
	cp2, err := m.store.LoadVersion(ctx, threadID, v2)
	if err != nil {
		return nil, err
	}

	diff := &CheckpointDiff{
		Version1:       v1,
		Version2:       v2,
		AddedNodes:     []string{},
		RemovedNodes:   []string{},
		ChangedNodes:   []string{},
		TimeDifference: cp2.CreatedAt.Sub(cp1.CreatedAt),
	}

	// Find added/removed nodes
	for nodeID := range cp2.NodeResults {
		if _, ok := cp1.NodeResults[nodeID]; !ok {
			diff.AddedNodes = append(diff.AddedNodes, nodeID)
		}
	}
	for nodeID := range cp1.NodeResults {
		if _, ok := cp2.NodeResults[nodeID]; !ok {
			diff.RemovedNodes = append(diff.RemovedNodes, nodeID)
		}
	}

	// Find changed nodes
	for nodeID, result1 := range cp1.NodeResults {
		if result2, ok := cp2.NodeResults[nodeID]; ok {
			j1, _ := json.Marshal(result1)
			j2, _ := json.Marshal(result2)
			if string(j1) != string(j2) {
				diff.ChangedNodes = append(diff.ChangedNodes, nodeID)
			}
		}
	}

	return diff, nil
}

// CheckpointDiff represents differences between checkpoints.
type CheckpointDiff struct {
	Version1       int           `json:"version1"`
	Version2       int           `json:"version2"`
	AddedNodes     []string      `json:"added_nodes"`
	RemovedNodes   []string      `json:"removed_nodes"`
	ChangedNodes   []string      `json:"changed_nodes"`
	TimeDifference time.Duration `json:"time_difference"`
}

func generateCheckpointID() string {
	return fmt.Sprintf("wfckpt_%d", time.Now().UnixNano())
}

// InMemoryCheckpointStore provides in-memory storage.
type InMemoryCheckpointStore struct {
	checkpoints map[string]*EnhancedCheckpoint
	mu          sync.RWMutex
}

// NewInMemoryCheckpointStore creates a new in-memory store.
func NewInMemoryCheckpointStore() *InMemoryCheckpointStore {
	return &InMemoryCheckpointStore{
		checkpoints: make(map[string]*EnhancedCheckpoint),
	}
}

func (s *InMemoryCheckpointStore) Save(ctx context.Context, cp *EnhancedCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[cp.ID] = cp
	return nil
}

func (s *InMemoryCheckpointStore) Load(ctx context.Context, id string) (*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, ok := s.checkpoints[id]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found: %s", id)
	}
	return cp, nil
}

func (s *InMemoryCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *EnhancedCheckpoint
	for _, cp := range s.checkpoints {
		if cp.ThreadID == threadID {
			if latest == nil || cp.Version > latest.Version {
				latest = cp
			}
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no checkpoints for thread: %s", threadID)
	}
	return latest, nil
}

func (s *InMemoryCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cp := range s.checkpoints {
		if cp.ThreadID == threadID && cp.Version == version {
			return cp, nil
		}
	}
	return nil, fmt.Errorf("version %d not found", version)
}

func (s *InMemoryCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*EnhancedCheckpoint
	for _, cp := range s.checkpoints {
		if cp.ThreadID == threadID {
			results = append(results, cp)
		}
	}
	return results, nil
}

func (s *InMemoryCheckpointStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.checkpoints, id)
	return nil
}
