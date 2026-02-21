package memory

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/rag"
	"go.uber.org/zap"
)

type InMemoryVectorStoreConfig struct {
	// 尺寸在 > 0时验证存储/搜索向量。
	Dimension int

	// 现在用于测试。 默认时间 。 现在。
	Now func() time.Time
}

type vectorEntry struct {
	vector    []float64
	metadata  map[string]any
	createdAt time.Time
}

// InMemoryVectorStore是增强MemorySystem的基本矢量执行.
// 它支持通过平等进行元数据过滤和同位素相似性搜索.
type InMemoryVectorStore struct {
	mu        sync.RWMutex
	items     map[string]vectorEntry
	dimension int
	now       func() time.Time
	logger    *zap.Logger
}

func NewInMemoryVectorStore(config InMemoryVectorStoreConfig, logger *zap.Logger) *InMemoryVectorStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &InMemoryVectorStore{
		items:     make(map[string]vectorEntry),
		dimension: config.Dimension,
		now:       now,
		logger:    logger.With(zap.String("component", "vector_store_inmemory")),
	}
}

func (s *InMemoryVectorStore) Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if vector == nil {
		return fmt.Errorf("vector is required")
	}
	if s.dimension > 0 && len(vector) != s.dimension {
		return fmt.Errorf("vector dimension mismatch: got %d want %d", len(vector), s.dimension)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[id] = vectorEntry{
		vector:    append([]float64(nil), vector...),
		metadata:  cloneMap(metadata),
		createdAt: s.now(),
	}
	return nil
}

func (s *InMemoryVectorStore) Search(ctx context.Context, query []float64, topK int, filter map[string]any) ([]rag.LowLevelSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query == nil {
		return nil, fmt.Errorf("query vector is required")
	}
	if s.dimension > 0 && len(query) != s.dimension {
		return nil, fmt.Errorf("query vector dimension mismatch: got %d want %d", len(query), s.dimension)
	}
	if topK <= 0 {
		return []rag.LowLevelSearchResult{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]rag.LowLevelSearchResult, 0, len(s.items))
	for id, ent := range s.items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !matchesFilter(ent.metadata, filter) {
			continue
		}
		score := cosineSimilarityFloat64(query, ent.vector)
		results = append(results, rag.LowLevelSearchResult{
			ID:       id,
			Score:    score,
			Metadata: cloneMap(ent.metadata),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > len(results) {
		topK = len(results)
	}
	return results[:topK], nil
}

func (s *InMemoryVectorStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, id)
	return nil
}

func (s *InMemoryVectorStore) BatchStore(ctx context.Context, items []VectorItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.Store(ctx, it.ID, it.Vector, it.Metadata); err != nil {
			return err
		}
	}
	return nil
}

func matchesFilter(metadata map[string]any, filter map[string]any) bool {
	if len(filter) == 0 {
		return true
	}
	if metadata == nil {
		return false
	}
	for k, v := range filter {
		mv, ok := metadata[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(mv, v) {
			return false
		}
	}
	return true
}

func cosineSimilarityFloat64(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
