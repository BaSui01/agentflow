package rag

import (
	"context"
	"math"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// SimpleGraphEmbedder 基于词袋模型的简单嵌入生成器。
// 不依赖外部嵌入服务，通过词频统计生成固定维度的向量，
// 适用于本地开发、测试和不需要高质量嵌入的场景。
type SimpleGraphEmbedder struct {
	mu        sync.RWMutex
	dimension int
	vocab     map[string]int // 词汇表：word -> index
	nextIdx   int
	logger    *zap.Logger
}

// SimpleGraphEmbedderConfig 简单嵌入生成器配置。
type SimpleGraphEmbedderConfig struct {
	// Dimension 嵌入向量维度，默认 128。
	Dimension int
}

// NewSimpleGraphEmbedder 创建简单嵌入生成器。
func NewSimpleGraphEmbedder(config SimpleGraphEmbedderConfig, logger *zap.Logger) *SimpleGraphEmbedder {
	if logger == nil {
		logger = zap.NewNop()
	}
	dim := config.Dimension
	if dim <= 0 {
		dim = 128
	}
	return &SimpleGraphEmbedder{
		dimension: dim,
		vocab:     make(map[string]int),
		logger:    logger.With(zap.String("component", "graph_embedder_simple")),
	}
}

// Embed 为文本生成嵌入向量。
// 使用词袋 + 哈希映射的方式将文本映射到固定维度的向量空间，并进行 L2 归一化。
func (e *SimpleGraphEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 分词并转小写
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return make([]float32, e.dimension), nil
	}

	vec := make([]float32, e.dimension)

	// 统计词频并映射到向量维度
	for _, word := range words {
		idx := e.getOrAssignIndex(word)
		// 使用取模映射到固定维度
		pos := idx % e.dimension
		vec[pos] += 1.0
	}

	// L2 归一化
	normalize32(vec)

	e.logger.Debug("embedding generated",
		zap.Int("text_len", len(text)),
		zap.Int("word_count", len(words)),
		zap.Int("dimension", e.dimension))

	return vec, nil
}

// getOrAssignIndex 获取或分配词汇索引。
func (e *SimpleGraphEmbedder) getOrAssignIndex(word string) int {
	e.mu.RLock()
	idx, ok := e.vocab[word]
	e.mu.RUnlock()
	if ok {
		return idx
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 双重检查
	if idx, ok := e.vocab[word]; ok {
		return idx
	}
	idx = e.nextIdx
	e.vocab[word] = idx
	e.nextIdx++
	return idx
}

// normalize32 对 float32 向量进行 L2 归一化。
func normalize32(vec []float32) {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
}
