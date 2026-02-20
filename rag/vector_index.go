package rag

import (
	"container/heap"
	"fmt"
	"math"
	"math/rand"
	"sync"

	"go.uber.org/zap"
)

// VectorIndex 向量索引接口
type VectorIndex interface {
	// Build 构建索引
	Build(vectors [][]float64, ids []string) error
	
	// Search 搜索最近邻
	Search(query []float64, k int) ([]SearchResult, error)
	
	// Add 添加向量
	Add(vector []float64, id string) error
	
	// Delete 删除向量
	Delete(id string) error
	
	// Size 索引大小
	Size() int
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       string
	Distance float64
	Score    float64 // 1 - distance (for cosine)
}

// IndexType 索引类型
type IndexType string

const (
	IndexFlat IndexType = "flat" // 暴力搜索
	IndexHNSW IndexType = "hnsw" // HNSW 图索引
	IndexIVF  IndexType = "ivf"  // IVF 聚类索引
)

// ====== HNSW 索引实现 ======

// HNSWConfig HNSW 配置
type HNSWConfig struct {
	M              int     `json:"m"`                // 每层最大连接数（12-48）
	EfConstruction int     `json:"ef_construction"`  // 构建时搜索宽度（100-200）
	EfSearch       int     `json:"ef_search"`        // 搜索时宽度（50-200）
	MaxLevel       int     `json:"max_level"`        // 最大层数
	Ml             float64 `json:"ml"`               // 层数归一化因子
}

// DefaultHNSWConfig 默认 HNSW 配置（生产级）
func DefaultHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              16,   // 平衡性能和召回率
		EfConstruction: 200,  // 高质量构建
		EfSearch:       100,  // 快速搜索
		MaxLevel:       16,
		Ml:             1.0 / math.Log(2.0),
	}
}

// AdaptiveHNSWConfig 自适应 HNSW 配置（根据数据规模动态调整）
// 基于 2025 最佳实践：小数据集用小 M，大数据集用大 M
func AdaptiveHNSWConfig(dataSize int) HNSWConfig {
	config := DefaultHNSWConfig()
	
	// 根据数据规模动态调整 M
	// 数据规模与 M 的对应关系：<10K 时 M=12，10K-100K 时 M=16，100K-1M 时 M=24，>1M 时 M=32
	switch {
	case dataSize < 10000:
		config.M = 12
		config.EfConstruction = 100
		config.EfSearch = 50
	case dataSize < 100000:
		config.M = 16
		config.EfConstruction = 200
		config.EfSearch = 100
	case dataSize < 1000000:
		config.M = 24
		config.EfConstruction = 300
		config.EfSearch = 150
	default:
		config.M = 32
		config.EfConstruction = 400
		config.EfSearch = 200
	}
	
	return config
}

// HNSWIndex HNSW 索引（Hierarchical Navigable Small World）
type HNSWIndex struct {
	config  HNSWConfig
	vectors map[string][]float64
	graph   map[string]map[int][]string // id -> level -> neighbors
	entryPoint string
	maxLevel   int
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewHNSWIndex 创建 HNSW 索引
func NewHNSWIndex(config HNSWConfig, logger *zap.Logger) *HNSWIndex {
	return &HNSWIndex{
		config:  config,
		vectors: make(map[string][]float64),
		graph:   make(map[string]map[int][]string),
		logger:  logger,
	}
}

// Build 构建 HNSW 索引
func (idx *HNSWIndex) Build(vectors [][]float64, ids []string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	if len(vectors) != len(ids) {
		return fmt.Errorf("vectors and ids length mismatch")
	}
	
	idx.logger.Info("building HNSW index",
		zap.Int("vectors", len(vectors)),
		zap.Int("M", idx.config.M),
		zap.Int("ef_construction", idx.config.EfConstruction))
	
	for i, vec := range vectors {
		id := ids[i]
		idx.vectors[id] = vec
		
		// 确定插入层数
		level := idx.randomLevel()
		if level > idx.maxLevel {
			idx.maxLevel = level
		}
		
		// 初始化图结构
		idx.graph[id] = make(map[int][]string)
		for l := 0; l <= level; l++ {
			idx.graph[id][l] = []string{}
		}
		
		// 插入到图中
		if idx.entryPoint == "" {
			idx.entryPoint = id
		} else {
			idx.insert(id, vec, level)
		}
	}
	
	idx.logger.Info("HNSW index built",
		zap.Int("size", len(idx.vectors)),
		zap.Int("max_level", idx.maxLevel))
	
	return nil
}

// Search 搜索最近邻
func (idx *HNSWIndex) Search(query []float64, k int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	if len(idx.vectors) == 0 {
		return []SearchResult{}, nil
	}
	
	// 从顶层开始搜索
	ep := idx.entryPoint
	for level := idx.maxLevel; level > 0; level-- {
		ep = idx.searchLayer(query, ep, 1, level)[0]
	}
	
	// 在第 0 层搜索 k 个最近邻
	candidates := idx.searchLayer(query, ep, idx.config.EfSearch, 0)
	
	// 转换为结果
	results := make([]SearchResult, 0, k)
	for i := 0; i < len(candidates) && i < k; i++ {
		id := candidates[i]
		distance := idx.distance(query, idx.vectors[id])
		results = append(results, SearchResult{
			ID:       id,
			Distance: distance,
			Score:    1.0 - distance, // cosine similarity
		})
	}
	
	return results, nil
}

// Add 添加向量
func (idx *HNSWIndex) Add(vector []float64, id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	if _, exists := idx.vectors[id]; exists {
		return fmt.Errorf("vector %s already exists", id)
	}
	
	idx.vectors[id] = vector
	level := idx.randomLevel()
	
	if level > idx.maxLevel {
		idx.maxLevel = level
	}
	
	idx.graph[id] = make(map[int][]string)
	for l := 0; l <= level; l++ {
		idx.graph[id][l] = []string{}
	}
	
	if idx.entryPoint == "" {
		idx.entryPoint = id
	} else {
		idx.insert(id, vector, level)
	}
	
	return nil
}

// Delete 删除向量
func (idx *HNSWIndex) Delete(id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	if _, exists := idx.vectors[id]; !exists {
		return fmt.Errorf("vector %s not found", id)
	}
	
	// 删除向量
	delete(idx.vectors, id)
	
	// 删除图中的连接
	delete(idx.graph, id)
	
	// 从邻居中删除引用
	for _, neighbors := range idx.graph {
		for level, levelNeighbors := range neighbors {
			filtered := []string{}
			for _, nid := range levelNeighbors {
				if nid != id {
					filtered = append(filtered, nid)
				}
			}
			neighbors[level] = filtered
		}
	}
	
	// 如果删除的是入口点，选择新的入口点
	if idx.entryPoint == id {
		for newID := range idx.vectors {
			idx.entryPoint = newID
			break
		}
	}
	
	return nil
}

// Size 索引大小
func (idx *HNSWIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.vectors)
}

// ====== HNSW 内部方法 ======

// insert 插入节点
func (idx *HNSWIndex) insert(id string, vector []float64, level int) {
	// 从顶层搜索到目标层
	ep := idx.entryPoint
	for lc := idx.maxLevel; lc > level; lc-- {
		ep = idx.searchLayer(vector, ep, 1, lc)[0]
	}
	
	// 在每一层插入
	for lc := level; lc >= 0; lc-- {
		candidates := idx.searchLayer(vector, ep, idx.config.EfConstruction, lc)
		
		// 选择 M 个最近邻
		m := idx.config.M
		if lc == 0 {
			m = idx.config.M * 2
		}
		
		neighbors := idx.selectNeighbors(id, candidates, m)
		
		// 添加双向连接
		idx.graph[id][lc] = neighbors
		for _, nid := range neighbors {
			idx.graph[nid][lc] = append(idx.graph[nid][lc], id)
			
			// 修剪邻居的连接
			if len(idx.graph[nid][lc]) > m {
				idx.graph[nid][lc] = idx.selectNeighbors(nid, idx.graph[nid][lc], m)
			}
		}
		
		if len(candidates) > 0 {
			ep = candidates[0]
		}
	}
}

// searchLayer 在指定层搜索
func (idx *HNSWIndex) searchLayer(query []float64, ep string, ef int, level int) []string {
	visited := make(map[string]bool)
	candidates := &minHeap{}
	w := &maxHeap{}
	
	// 初始化
	dist := idx.distance(query, idx.vectors[ep])
	heap.Push(candidates, &heapItem{id: ep, dist: dist})
	heap.Push(w, &heapItem{id: ep, dist: dist})
	visited[ep] = true
	
	for candidates.Len() > 0 {
		c := heap.Pop(candidates).(*heapItem)
		
		if c.dist > (*w)[0].dist {
			break
		}
		
		// 检查邻居
		for _, nid := range idx.graph[c.id][level] {
			if visited[nid] {
				continue
			}
			visited[nid] = true
			
			dist := idx.distance(query, idx.vectors[nid])
			
			if dist < (*w)[0].dist || w.Len() < ef {
				heap.Push(candidates, &heapItem{id: nid, dist: dist})
				heap.Push(w, &heapItem{id: nid, dist: dist})
				
				if w.Len() > ef {
					heap.Pop(w)
				}
			}
		}
	}
	
	// 返回结果
	result := make([]string, w.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(w).(*heapItem).id
	}
	
	return result
}

// selectNeighbors 选择邻居（启发式）
func (idx *HNSWIndex) selectNeighbors(id string, candidates []string, m int) []string {
	if len(candidates) <= m {
		return candidates
	}
	
	// 简化版：选择距离最近的 m 个
	type candidate struct {
		id   string
		dist float64
	}
	
	cands := make([]candidate, len(candidates))
	for i, cid := range candidates {
		cands[i] = candidate{
			id:   cid,
			dist: idx.distance(idx.vectors[id], idx.vectors[cid]),
		}
	}
	
	// 排序
	for i := 0; i < len(cands)-1; i++ {
		for j := i + 1; j < len(cands); j++ {
			if cands[i].dist > cands[j].dist {
				cands[i], cands[j] = cands[j], cands[i]
			}
		}
	}
	
	result := make([]string, m)
	for i := 0; i < m; i++ {
		result[i] = cands[i].id
	}
	
	return result
}

// randomLevel 随机生成层数
func (idx *HNSWIndex) randomLevel() int {
	level := 0
	for rand.Float64() < 0.5 && level < idx.config.MaxLevel {
		level++
	}
	return level
}

// distance 计算距离（余弦距离）
func (idx *HNSWIndex) distance(a, b []float64) float64 {
	if len(a) != len(b) {
		return 1.0
	}
	
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	
	if normA == 0 || normB == 0 {
		return 1.0
	}
	
	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	return 1.0 - similarity // 转换为距离
}

// ====== 堆实现 ======

type heapItem struct {
	id   string
	dist float64
}

type minHeap []*heapItem

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].dist < h[j].dist }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *minHeap) Push(x any) {
	*h = append(*h, x.(*heapItem))
}

func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type maxHeap []*heapItem

func (h maxHeap) Len() int           { return len(h) }
func (h maxHeap) Less(i, j int) bool { return h[i].dist > h[j].dist }
func (h maxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *maxHeap) Push(x any) {
	*h = append(*h, x.(*heapItem))
}

func (h *maxHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
