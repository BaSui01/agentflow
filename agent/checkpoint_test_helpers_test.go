package agent

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ============================================================
// Test helpers for checkpoint tests
// ============================================================

// inMemoryCheckpointStore is a simple in-memory CheckpointStore for testing.
type inMemoryCheckpointStore struct {
	checkpoints map[string]*Checkpoint
	threads     map[string][]string // threadID -> []checkpointID (ordered by time)
}

func newInMemoryCheckpointStore() *inMemoryCheckpointStore {
	return &inMemoryCheckpointStore{
		checkpoints: make(map[string]*Checkpoint),
		threads:     make(map[string][]string),
	}
}

func (s *inMemoryCheckpointStore) Save(_ context.Context, cp *Checkpoint) error {
	if cp.Version == 0 {
		cp.Version = len(s.threads[cp.ThreadID]) + 1
	}
	s.checkpoints[cp.ID] = cp
	// Only append if not already tracked (avoid duplicates on re-save).
	found := false
	for _, id := range s.threads[cp.ThreadID] {
		if id == cp.ID {
			found = true
			break
		}
	}
	if !found {
		s.threads[cp.ThreadID] = append(s.threads[cp.ThreadID], cp.ID)
	}
	return nil
}

func (s *inMemoryCheckpointStore) Load(_ context.Context, id string) (*Checkpoint, error) {
	cp, ok := s.checkpoints[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return cp, nil
}

func (s *inMemoryCheckpointStore) LoadLatest(_ context.Context, threadID string) (*Checkpoint, error) {
	ids := s.threads[threadID]
	if len(ids) == 0 {
		return nil, fmt.Errorf("no checkpoints for thread: %s", threadID)
	}
	return s.checkpoints[ids[len(ids)-1]], nil
}

func (s *inMemoryCheckpointStore) List(_ context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	ids := s.threads[threadID]
	result := make([]*Checkpoint, 0, len(ids))
	for i := len(ids) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, s.checkpoints[ids[i]])
	}
	return result, nil
}

func (s *inMemoryCheckpointStore) Delete(_ context.Context, id string) error {
	delete(s.checkpoints, id)
	return nil
}

func (s *inMemoryCheckpointStore) DeleteThread(_ context.Context, threadID string) error {
	for _, id := range s.threads[threadID] {
		delete(s.checkpoints, id)
	}
	delete(s.threads, threadID)
	return nil
}

func (s *inMemoryCheckpointStore) LoadVersion(_ context.Context, threadID string, version int) (*Checkpoint, error) {
	for _, id := range s.threads[threadID] {
		if cp := s.checkpoints[id]; cp != nil && cp.Version == version {
			return cp, nil
		}
	}
	return nil, fmt.Errorf("version %d not found", version)
}

func (s *inMemoryCheckpointStore) ListVersions(_ context.Context, threadID string) ([]CheckpointVersion, error) {
	var versions []CheckpointVersion
	for _, id := range s.threads[threadID] {
		cp := s.checkpoints[id]
		if cp != nil {
			versions = append(versions, CheckpointVersion{
				Version:   cp.Version,
				ID:        cp.ID,
				CreatedAt: cp.CreatedAt,
				State:     cp.State,
			})
		}
	}
	return versions, nil
}

func (s *inMemoryCheckpointStore) Rollback(ctx context.Context, threadID string, version int) error {
	// Load the target version
	target, err := s.LoadVersion(ctx, threadID, version)
	if err != nil {
		return err
	}

	// Create a new checkpoint as the rollback point
	newCP := *target
	newCP.ID = generateCheckpointID()
	newCP.CreatedAt = time.Now()
	newCP.ParentID = target.ID
	newCP.Version = len(s.threads[threadID]) + 1
	if newCP.Metadata == nil {
		newCP.Metadata = make(map[string]any)
	}
	newCP.Metadata["rollback_from_version"] = version

	return s.Save(ctx, &newCP)
}

// errorCheckpointStore always returns errors from its methods.
type errorCheckpointStore struct {
	saveErr error
}

func (s *errorCheckpointStore) Save(_ context.Context, _ *Checkpoint) error { return s.saveErr }
func (s *errorCheckpointStore) Load(_ context.Context, _ string) (*Checkpoint, error) {
	return nil, fmt.Errorf("error")
}
func (s *errorCheckpointStore) LoadLatest(_ context.Context, _ string) (*Checkpoint, error) {
	return nil, fmt.Errorf("error")
}
func (s *errorCheckpointStore) List(_ context.Context, _ string, _ int) ([]*Checkpoint, error) {
	return nil, fmt.Errorf("error")
}
func (s *errorCheckpointStore) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("error")
}
func (s *errorCheckpointStore) DeleteThread(_ context.Context, _ string) error {
	return fmt.Errorf("error")
}
func (s *errorCheckpointStore) LoadVersion(_ context.Context, _ string, _ int) (*Checkpoint, error) {
	return nil, fmt.Errorf("error")
}
func (s *errorCheckpointStore) ListVersions(_ context.Context, _ string) ([]CheckpointVersion, error) {
	return nil, fmt.Errorf("error")
}
func (s *errorCheckpointStore) Rollback(_ context.Context, _ string, _ int) error {
	return fmt.Errorf("error")
}

// mockRedisClient implements RedisClient for testing.
type mockRedisClient struct {
	data  map[string][]byte
	zsets map[string][]zsetEntry
}

type zsetEntry struct {
	score  float64
	member string
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{
		data:  make(map[string][]byte),
		zsets: make(map[string][]zsetEntry),
	}
}

func (c *mockRedisClient) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.data[key] = value
	return nil
}

func (c *mockRedisClient) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := c.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return v, nil
}

func (c *mockRedisClient) Delete(_ context.Context, key string) error {
	delete(c.data, key)
	return nil
}

func (c *mockRedisClient) Keys(_ context.Context, pattern string) ([]string, error) {
	// Support prefix glob matching (e.g., "prefix:*")
	var keys []string
	prefix := ""
	if idx := len(pattern) - 1; idx >= 0 && pattern[idx] == '*' {
		prefix = pattern[:idx]
	}
	for k := range c.data {
		if prefix == "" || len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (c *mockRedisClient) ZAdd(_ context.Context, key string, score float64, member string) error {
	c.zsets[key] = append(c.zsets[key], zsetEntry{score: score, member: member})
	return nil
}

func (c *mockRedisClient) ZRevRange(_ context.Context, key string, start, stop int64) ([]string, error) {
	entries := c.zsets[key]
	if len(entries) == 0 {
		return nil, nil
	}
	// Sort by score descending
	sorted := make([]zsetEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})
	end := int(stop) + 1
	if end > len(sorted) {
		end = len(sorted)
	}
	startIdx := int(start)
	if startIdx >= len(sorted) {
		return nil, nil
	}
	var result []string
	for i := startIdx; i < end; i++ {
		result = append(result, sorted[i].member)
	}
	return result, nil
}

func (c *mockRedisClient) ZRemRangeByScore(_ context.Context, _, _, _ string) error {
	return nil
}

