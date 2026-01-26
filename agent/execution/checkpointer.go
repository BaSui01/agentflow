// package execution provides pluggable persistence backends for agent state.
package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("checkpoint not found")
	ErrInvalidThread = errors.New("invalid thread id")
	ErrStorageFull   = errors.New("storage capacity exceeded")
)

// Checkpoint represents a saved state at a point in time.
type Checkpoint struct {
	ID        string         `json:"id"`
	ThreadID  string         `json:"thread_id"`
	ParentID  string         `json:"parent_id,omitempty"`
	State     map[string]any `json:"state"`
	Metadata  Metadata       `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

// Metadata contains checkpoint metadata.
type Metadata struct {
	Step   int               `json:"step"`
	NodeID string            `json:"node_id,omitempty"`
	Tags   []string          `json:"tags,omitempty"`
	Custom map[string]string `json:"custom,omitempty"`
}

// Checkpointer defines the interface for checkpoint storage backends.
type Checkpointer interface {
	// Put saves a checkpoint.
	Put(ctx context.Context, cp *Checkpoint) error

	// Get retrieves a checkpoint by ID.
	Get(ctx context.Context, threadID, checkpointID string) (*Checkpoint, error)

	// GetLatest retrieves the most recent checkpoint for a thread.
	GetLatest(ctx context.Context, threadID string) (*Checkpoint, error)

	// List returns checkpoints for a thread, ordered by creation time.
	List(ctx context.Context, threadID string, opts ListOptions) ([]*Checkpoint, error)

	// Delete removes a checkpoint.
	Delete(ctx context.Context, threadID, checkpointID string) error

	// DeleteThread removes all checkpoints for a thread.
	DeleteThread(ctx context.Context, threadID string) error
}

// ListOptions configures checkpoint listing.
type ListOptions struct {
	Limit  int       `json:"limit"`
	Before time.Time `json:"before,omitempty"`
	After  time.Time `json:"after,omitempty"`
}

// MemoryCheckpointer implements in-memory checkpoint storage.
type MemoryCheckpointer struct {
	threads map[string][]*Checkpoint
	mu      sync.RWMutex
	maxSize int
}

// NewMemoryCheckpointer creates a new in-memory checkpointer.
func NewMemoryCheckpointer(maxSize int) *MemoryCheckpointer {
	return &MemoryCheckpointer{
		threads: make(map[string][]*Checkpoint),
		maxSize: maxSize,
	}
}

func (m *MemoryCheckpointer) Put(ctx context.Context, cp *Checkpoint) error {
	if cp.ThreadID == "" {
		return ErrInvalidThread
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cp.CreatedAt = time.Now()
	m.threads[cp.ThreadID] = append(m.threads[cp.ThreadID], cp)

	// Enforce max size per thread
	if m.maxSize > 0 && len(m.threads[cp.ThreadID]) > m.maxSize {
		m.threads[cp.ThreadID] = m.threads[cp.ThreadID][1:]
	}

	return nil
}

func (m *MemoryCheckpointer) Get(ctx context.Context, threadID, checkpointID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps, ok := m.threads[threadID]
	if !ok {
		return nil, ErrNotFound
	}

	for _, cp := range cps {
		if cp.ID == checkpointID {
			return cp, nil
		}
	}

	return nil, ErrNotFound
}

func (m *MemoryCheckpointer) GetLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps, ok := m.threads[threadID]
	if !ok || len(cps) == 0 {
		return nil, ErrNotFound
	}

	return cps[len(cps)-1], nil
}

func (m *MemoryCheckpointer) List(ctx context.Context, threadID string, opts ListOptions) ([]*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps, ok := m.threads[threadID]
	if !ok {
		return []*Checkpoint{}, nil
	}

	result := make([]*Checkpoint, 0, len(cps))
	for _, cp := range cps {
		if !opts.Before.IsZero() && cp.CreatedAt.After(opts.Before) {
			continue
		}
		if !opts.After.IsZero() && cp.CreatedAt.Before(opts.After) {
			continue
		}
		result = append(result, cp)
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}

	return result, nil
}

func (m *MemoryCheckpointer) Delete(ctx context.Context, threadID, checkpointID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cps, ok := m.threads[threadID]
	if !ok {
		return ErrNotFound
	}

	for i, cp := range cps {
		if cp.ID == checkpointID {
			m.threads[threadID] = append(cps[:i], cps[i+1:]...)
			return nil
		}
	}

	return ErrNotFound
}

func (m *MemoryCheckpointer) DeleteThread(ctx context.Context, threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.threads, threadID)
	return nil
}

// Serde defines serialization/deserialization interface.
type Serde interface {
	Serialize(v any) ([]byte, error)
	Deserialize(data []byte, v any) error
}

// JSONSerde implements JSON serialization.
type JSONSerde struct{}

func (JSONSerde) Serialize(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONSerde) Deserialize(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// CheckpointerConfig configures checkpointer behavior.
type CheckpointerConfig struct {
	MaxCheckpointsPerThread int           `json:"max_checkpoints_per_thread"`
	TTL                     time.Duration `json:"ttl"`
	Serde                   Serde         `json:"-"`
}

// DefaultCheckpointerConfig returns sensible defaults.
func DefaultCheckpointerConfig() CheckpointerConfig {
	return CheckpointerConfig{
		MaxCheckpointsPerThread: 100,
		TTL:                     24 * time.Hour,
		Serde:                   JSONSerde{},
	}
}

// ThreadManager manages multiple threads with checkpointing.
type ThreadManager struct {
	checkpointer Checkpointer
	config       CheckpointerConfig
	mu           sync.RWMutex
}

// NewThreadManager creates a new thread manager.
func NewThreadManager(cp Checkpointer, config CheckpointerConfig) *ThreadManager {
	return &ThreadManager{
		checkpointer: cp,
		config:       config,
	}
}

// SaveState saves the current state for a thread.
func (tm *ThreadManager) SaveState(ctx context.Context, threadID string, state map[string]any, meta Metadata) (*Checkpoint, error) {
	cp := &Checkpoint{
		ID:       generateCheckpointID(),
		ThreadID: threadID,
		State:    state,
		Metadata: meta,
	}

	// Get parent ID from latest checkpoint
	if latest, err := tm.checkpointer.GetLatest(ctx, threadID); err == nil {
		cp.ParentID = latest.ID
	}

	if err := tm.checkpointer.Put(ctx, cp); err != nil {
		return nil, err
	}

	return cp, nil
}

// LoadState loads the latest state for a thread.
func (tm *ThreadManager) LoadState(ctx context.Context, threadID string) (map[string]any, error) {
	cp, err := tm.checkpointer.GetLatest(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return cp.State, nil
}

// Rollback rolls back to a specific checkpoint.
func (tm *ThreadManager) Rollback(ctx context.Context, threadID, checkpointID string) (*Checkpoint, error) {
	return tm.checkpointer.Get(ctx, threadID, checkpointID)
}

// GetHistory returns checkpoint history for a thread.
func (tm *ThreadManager) GetHistory(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	return tm.checkpointer.List(ctx, threadID, ListOptions{Limit: limit})
}

func generateCheckpointID() string {
	return time.Now().Format("20060102150405.000000")
}
