package rag

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 节点代表了知识图中的节点.
type Node struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties,omitempty"`
	Embedding  []float32      `json:"embedding,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// 边缘代表了节点之间的关系.
type Edge struct {
	ID         string         `json:"id"`
	Source     string         `json:"source"`
	Target     string         `json:"target"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Weight     float64        `json:"weight"`
}

// 三相代表一个主题-前相-对象三相.
type Triple struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// KnowledgeGraph提供记忆知识图操作.
type KnowledgeGraph struct {
	nodes    map[string]*Node
	edges    map[string]*Edge
	outEdges map[string][]string // nodeID -> edgeIDs
	inEdges  map[string][]string // nodeID -> edgeIDs
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewKnowledgeGraph创建了新的知识图.
func NewKnowledgeGraph(logger *zap.Logger) *KnowledgeGraph {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &KnowledgeGraph{
		nodes:    make(map[string]*Node),
		edges:    make(map[string]*Edge),
		outEdges: make(map[string][]string),
		inEdges:  make(map[string][]string),
		logger:   logger.With(zap.String("component", "knowledge_graph")),
	}
}

// 添加节点在图表中添加了节点。
func (g *KnowledgeGraph) AddNode(node *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if node.ID == "" {
		node.ID = fmt.Sprintf("node_%d", time.Now().UnixNano())
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now()
	}
	g.nodes[node.ID] = node
}

// 添加Edge在图中添加了边缘.
func (g *KnowledgeGraph) AddEdge(edge *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if edge.ID == "" {
		edge.ID = fmt.Sprintf("edge_%d", time.Now().UnixNano())
	}
	g.edges[edge.ID] = edge
	g.outEdges[edge.Source] = append(g.outEdges[edge.Source], edge.ID)
	g.inEdges[edge.Target] = append(g.inEdges[edge.Target], edge.ID)
}

// GetNode通过ID检索到一个节点.
func (g *KnowledgeGraph) GetNode(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}

// Get nearbors return 邻居的节点。
func (g *KnowledgeGraph) GetNeighbors(nodeID string, depth int) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	var results []*Node
	g.traverseNeighbors(nodeID, depth, visited, &results)
	return results
}

func (g *KnowledgeGraph) traverseNeighbors(nodeID string, depth int, visited map[string]bool, results *[]*Node) {
	if depth <= 0 || visited[nodeID] {
		return
	}
	visited[nodeID] = true

	// 外出边缘
	for _, edgeID := range g.outEdges[nodeID] {
		edge := g.edges[edgeID]
		if node, ok := g.nodes[edge.Target]; ok && !visited[edge.Target] {
			*results = append(*results, node)
			g.traverseNeighbors(edge.Target, depth-1, visited, results)
		}
	}

	// 即将来临的边缘
	for _, edgeID := range g.inEdges[nodeID] {
		edge := g.edges[edgeID]
		if node, ok := g.nodes[edge.Source]; ok && !visited[edge.Source] {
			*results = append(*results, node)
			g.traverseNeighbors(edge.Source, depth-1, visited, results)
		}
	}
}

// 查询ByType返回特定类型的节点。
func (g *KnowledgeGraph) QueryByType(nodeType string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var results []*Node
	for _, n := range g.nodes {
		if n.Type == nodeType {
			results = append(results, n)
		}
	}
	return results
}

// GraphRAG结合了知识图和向量检索.
type GraphRAG struct {
	graph       *KnowledgeGraph
	vectorStore GraphVectorStore
	embedder    GraphEmbedder
	config      GraphRAGConfig
	logger      *zap.Logger
}

// GraphRAG中矢量操作的 GraphVectorStore 接口.
type GraphVectorStore interface {
	Store(ctx context.Context, id string, embedding []float32, metadata map[string]any) error
	Search(ctx context.Context, embedding []float32, limit int) ([]GraphVectorResult, error)
}

// GraphVectorResult 代表着向量搜索结果.
type GraphVectorResult struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// GraphEmbedder 生成嵌入式.
type GraphEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// GraphRAGConfig 配置了 GraphRAG.
type GraphRAGConfig struct {
	GraphWeight   float64 `json:"graph_weight"`    // Weight for graph results
	VectorWeight  float64 `json:"vector_weight"`   // Weight for vector results
	MaxGraphDepth int     `json:"max_graph_depth"` // Max traversal depth
	MaxResults    int     `json:"max_results"`
	MinScore      float64 `json:"min_score"`
}

// 默认 GraphRAGConfig 返回默认配置 。
func DefaultGraphRAGConfig() GraphRAGConfig {
	return GraphRAGConfig{
		GraphWeight:   0.4,
		VectorWeight:  0.6,
		MaxGraphDepth: 2,
		MaxResults:    10,
		MinScore:      0.5,
	}
}

// NewGraphRAG创建了一个新的GraphRAG实例.
func NewGraphRAG(graph *KnowledgeGraph, vectorStore GraphVectorStore, embedder GraphEmbedder, config GraphRAGConfig, logger *zap.Logger) *GraphRAG {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GraphRAG{
		graph:       graph,
		vectorStore: vectorStore,
		embedder:    embedder,
		config:      config,
		logger:      logger.With(zap.String("component", "graph_rag")),
	}
}

// Graph Retrival Result 代表混合检索结果.
type GraphRetrievalResult struct {
	ID           string         `json:"id"`
	Content      string         `json:"content"`
	Score        float64        `json:"score"`
	GraphScore   float64        `json:"graph_score"`
	VectorScore  float64        `json:"vector_score"`
	Source       string         `json:"source"` // "graph", "vector", "hybrid"
	Metadata     map[string]any `json:"metadata,omitempty"`
	RelatedNodes []*Node        `json:"related_nodes,omitempty"`
}

// 检索进行混合检索。
func (r *GraphRAG) Retrieve(ctx context.Context, query string) ([]GraphRetrievalResult, error) {
	// 生成查询嵌入
	queryEmb, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// 矢量搜索
	vectorResults, err := r.vectorStore.Search(ctx, queryEmb, r.config.MaxResults*2)
	if err != nil {
		r.logger.Warn("vector search failed", zap.Error(err))
	}

	// 构建结果映射
	resultMap := make(map[string]*GraphRetrievalResult)

	// 进程向量结果
	for _, vr := range vectorResults {
		resultMap[vr.ID] = &GraphRetrievalResult{
			ID:          vr.ID,
			VectorScore: vr.Score,
			Source:      "vector",
			Metadata:    vr.Metadata,
		}
	}

	// 上向量结果的图正反转
	for _, vr := range vectorResults[:min(5, len(vectorResults))] {
		neighbors := r.graph.GetNeighbors(vr.ID, r.config.MaxGraphDepth)
		for _, neighbor := range neighbors {
			if existing, ok := resultMap[neighbor.ID]; ok {
				existing.GraphScore = 0.8 // Connected to query result
				existing.Source = "hybrid"
				existing.RelatedNodes = append(existing.RelatedNodes, neighbor)
			} else {
				resultMap[neighbor.ID] = &GraphRetrievalResult{
					ID:         neighbor.ID,
					Content:    neighbor.Label,
					GraphScore: 0.6,
					Source:     "graph",
					Metadata:   neighbor.Properties,
				}
			}
		}
	}

	// 计算最终分数和过滤器
	var results []GraphRetrievalResult
	for _, res := range resultMap {
		res.Score = res.VectorScore*r.config.VectorWeight + res.GraphScore*r.config.GraphWeight
		if res.Score >= r.config.MinScore {
			results = append(results, *res)
		}
	}

	// 按分数排序(优化:O(n logn n)而不是O(n2))
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制结果
	if len(results) > r.config.MaxResults {
		results = results[:r.config.MaxResults]
	}

	r.logger.Debug("hybrid retrieval completed",
		zap.Int("results", len(results)),
		zap.Int("vector_hits", len(vectorResults)),
	)

	return results, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 添加文件将文档添加到图表和向量存储中 。
func (r *GraphRAG) AddDocument(ctx context.Context, doc GraphDocument) error {
	// 生成嵌入
	emb, err := r.embedder.Embed(ctx, doc.Content)
	if err != nil {
		return fmt.Errorf("embed document %s: %w", doc.ID, err)
	}

	// 添加到向量存储
	if err := r.vectorStore.Store(ctx, doc.ID, emb, doc.Metadata); err != nil {
		return fmt.Errorf("store document %s: %w", doc.ID, err)
	}

	// 添加到图表
	node := &Node{
		ID:         doc.ID,
		Type:       "document",
		Label:      doc.Title,
		Properties: doc.Metadata,
		Embedding:  emb,
	}
	r.graph.AddNode(node)

	// 如果实体提供了, 则添加实体边缘
	for _, entity := range doc.Entities {
		entityNode := &Node{
			ID:    entity.ID,
			Type:  entity.Type,
			Label: entity.Name,
		}
		r.graph.AddNode(entityNode)
		r.graph.AddEdge(&Edge{
			Source: doc.ID,
			Target: entity.ID,
			Type:   "mentions",
		})
	}

	return nil
}

// GraphDocument是用于索引的文档。
type GraphDocument struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Entities []Entity       `json:"entities,omitempty"`
}

// 实体代表被提取的实体。
type Entity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}
