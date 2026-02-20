package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DecayConfig配置了智能衰变机制.
type DecayConfig struct {
	RecencyWeight   float64       `json:"recency_weight"`   // Weight for recency score (0-1)
	RelevanceWeight float64       `json:"relevance_weight"` // Weight for relevance score (0-1)
	UtilityWeight   float64       `json:"utility_weight"`   // Weight for utility score (0-1)
	DecayThreshold  float64       `json:"decay_threshold"`  // Score below which memories are pruned
	DecayInterval   time.Duration `json:"decay_interval"`   // How often to run decay
	MaxMemories     int           `json:"max_memories"`     // Maximum memories to retain
	HalfLife        time.Duration `json:"half_life"`        // Time for recency score to halve
}

// 默认DecayConfig 返回合理的默认值 。
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		RecencyWeight:   0.3,
		RelevanceWeight: 0.4,
		UtilityWeight:   0.3,
		DecayThreshold:  0.2,
		DecayInterval:   time.Hour,
		MaxMemories:     1000,
		HalfLife:        24 * time.Hour,
	}
}

// 内存项目代表着一个带有衰变元数据的内存项目.
type MemoryItem struct {
	ID           string                 `json:"id"`
	Content      string                 `json:"content"`
	Vector       []float64              `json:"vector,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	LastAccessed time.Time              `json:"last_accessed"`
	AccessCount  int                    `json:"access_count"`
	Relevance    float64                `json:"relevance"` // User-defined relevance (0-1)
	Utility      float64                `json:"utility"`   // Computed utility based on usage
	Tags         []string               `json:"tags,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// 复合分数计算出复合衰减分数.
func (m *MemoryItem) CompositeScore(config DecayConfig) float64 {
	recency := m.RecencyScore(config.HalfLife)
	return config.RecencyWeight*recency +
		config.RelevanceWeight*m.Relevance +
		config.UtilityWeight*m.Utility
}

// RecencyScore使用指数衰减计算Recency分数.
func (m *MemoryItem) RecencyScore(halfLife time.Duration) float64 {
	age := time.Since(m.LastAccessed)
	lambda := math.Ln2 / halfLife.Seconds()
	return math.Exp(-lambda * age.Seconds())
}

// IntelligentDecay以智能衰变管理内存.
type IntelligentDecay struct {
	config   DecayConfig
	memories map[string]*MemoryItem
	mu       sync.RWMutex
	logger   *zap.Logger

	running   bool
	stopCh    chan struct{}
	closeOnce sync.Once
}

// 新智能Decay创建了新的智能衰变管理器.
func NewIntelligentDecay(config DecayConfig, logger *zap.Logger) *IntelligentDecay {
	if logger == nil {
		logger = zap.NewNop()
	}
	// 使权重正常化
	total := config.RecencyWeight + config.RelevanceWeight + config.UtilityWeight
	if total > 0 {
		config.RecencyWeight /= total
		config.RelevanceWeight /= total
		config.UtilityWeight /= total
	}
	return &IntelligentDecay{
		config:   config,
		memories: make(map[string]*MemoryItem),
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// 添加一个内存项 。
func (d *IntelligentDecay) Add(item *MemoryItem) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	if item.LastAccessed.IsZero() {
		item.LastAccessed = time.Now()
	}
	if item.Relevance == 0 {
		item.Relevance = 0.5 // Default relevance
	}

	d.memories[item.ID] = item
	d.logger.Debug("memory added", zap.String("id", item.ID))
}

// 获取一个内存项目并更新访问元数据 。
func (d *IntelligentDecay) Get(id string) *MemoryItem {
	d.mu.Lock()
	defer d.mu.Unlock()

	item, ok := d.memories[id]
	if !ok {
		return nil
	}

	// 更新访问元数据
	item.LastAccessed = time.Now()
	item.AccessCount++
	item.Utility = d.calculateUtility(item)

	return item
}

// 搜索通过对查询向量的关联找到记忆 。
func (d *IntelligentDecay) Search(queryVector []float64, topK int) []*MemoryItem {
	d.mu.RLock()
	defer d.mu.RUnlock()

	type scored struct {
		item  *MemoryItem
		score float64
	}

	var results []scored
	for _, item := range d.memories {
		if len(item.Vector) == 0 {
			continue
		}
		similarity := cosineSimilarity(queryVector, item.Vector)
		composite := item.CompositeScore(d.config)
		// 将相似性与综合分数合并
		finalScore := 0.6*similarity + 0.4*composite
		results = append(results, scored{item: item, score: finalScore})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}

	items := make([]*MemoryItem, topK)
	for i := 0; i < topK; i++ {
		items[i] = results[i].item
	}
	return items
}

// 衰变运行了一次衰变过程.
func (d *IntelligentDecay) Decay(ctx context.Context) DecayResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := DecayResult{
		Timestamp:   time.Now(),
		TotalBefore: len(d.memories),
	}

	// 计算分数并识别要分数的项目
	type scored struct {
		id    string
		score float64
	}

	var items []scored
	for id, item := range d.memories {
		score := item.CompositeScore(d.config)
		items = append(items, scored{id: id, score: score})
	}

	// 按分数排序( 最先刷分)
	sort.Slice(items, func(i, j int) bool {
		return items[i].score < items[j].score
	})

	// 低于阈值或超过最大值的倾角项目
	pruneCount := 0
	for _, item := range items {
		shouldPrune := item.score < d.config.DecayThreshold ||
			(len(d.memories)-pruneCount > d.config.MaxMemories)

		if shouldPrune {
			delete(d.memories, item.id)
			pruneCount++
			result.PrunedIDs = append(result.PrunedIDs, item.id)
		}
	}

	result.TotalAfter = len(d.memories)
	result.PrunedCount = pruneCount

	d.logger.Info("decay completed",
		zap.Int("pruned", pruneCount),
		zap.Int("remaining", result.TotalAfter))

	return result
}

// 衰变Result包含衰变操作的结果.
type DecayResult struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalBefore int       `json:"total_before"`
	TotalAfter  int       `json:"total_after"`
	PrunedCount int       `json:"pruned_count"`
	PrunedIDs   []string  `json:"pruned_ids,omitempty"`
}

// 启动自动衰变进程 。
func (d *IntelligentDecay) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = true
	d.stopCh = make(chan struct{})
	d.mu.Unlock()

	go d.runDecayLoop(ctx)
	return nil
}

// 停止自动衰变过程 。
func (d *IntelligentDecay) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		d.closeOnce.Do(func() { close(d.stopCh) })
		d.running = false
	}
}

func (d *IntelligentDecay) runDecayLoop(ctx context.Context) {
	ticker := time.NewTicker(d.config.DecayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.Decay(ctx)
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *IntelligentDecay) calculateUtility(item *MemoryItem) float64 {
	// 基于接入频率和惯性
	if item.AccessCount == 0 {
		return 0.1
	}

	// 日志比例访问次数( 减少返回)
	accessScore := math.Log1p(float64(item.AccessCount)) / 10.0
	if accessScore > 1.0 {
		accessScore = 1.0
	}

	// 与审慎相结合
	recency := item.RecencyScore(d.config.HalfLife)
	return 0.5*accessScore + 0.5*recency
}

// 更新关联更新一个内存的相关分数.
func (d *IntelligentDecay) UpdateRelevance(id string, relevance float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	item, ok := d.memories[id]
	if !ok {
		return ErrMemoryNotFound
	}

	if relevance < 0 {
		relevance = 0
	} else if relevance > 1 {
		relevance = 1
	}
	item.Relevance = relevance
	return nil
}

// GetStats 返回关于记忆库的统计数据.
func (d *IntelligentDecay) GetStats() DecayStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := DecayStats{
		TotalMemories: len(d.memories),
	}

	if len(d.memories) == 0 {
		return stats
	}

	var totalScore, totalRecency, totalRelevance, totalUtility float64
	for _, item := range d.memories {
		score := item.CompositeScore(d.config)
		totalScore += score
		totalRecency += item.RecencyScore(d.config.HalfLife)
		totalRelevance += item.Relevance
		totalUtility += item.Utility
	}

	n := float64(len(d.memories))
	stats.AverageScore = totalScore / n
	stats.AverageRecency = totalRecency / n
	stats.AverageRelevance = totalRelevance / n
	stats.AverageUtility = totalUtility / n

	return stats
}

// DecayStats包含了关于记忆库的统计数据.
type DecayStats struct {
	TotalMemories    int     `json:"total_memories"`
	AverageScore     float64 `json:"average_score"`
	AverageRecency   float64 `json:"average_recency"`
	AverageRelevance float64 `json:"average_relevance"`
	AverageUtility   float64 `json:"average_utility"`
}

// 未找到内存时返回 Err Memory Not Found 。
var ErrMemoryNotFound = fmt.Errorf("memory not found")

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
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
