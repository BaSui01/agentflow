package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// InMemoryKnowledgeGraph 基于内存的知识图谱实现。
// 适用于本地开发、测试和小规模部署场景。
type InMemoryKnowledgeGraph struct {
	mu        sync.RWMutex
	entities  map[string]*Entity
	relations map[string]*Relation
	// outRels 记录从某个实体出发的关系 ID 列表
	outRels map[string][]string
	// inRels 记录指向某个实体的关系 ID 列表
	inRels map[string][]string
	logger *zap.Logger
}

// NewInMemoryKnowledgeGraph 创建内存知识图谱。
func NewInMemoryKnowledgeGraph(logger *zap.Logger) *InMemoryKnowledgeGraph {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InMemoryKnowledgeGraph{
		entities:  make(map[string]*Entity),
		relations: make(map[string]*Relation),
		outRels:   make(map[string][]string),
		inRels:    make(map[string][]string),
		logger:    logger.With(zap.String("component", "knowledge_graph_inmemory")),
	}
}

// AddEntity 添加实体到知识图谱。
func (g *InMemoryKnowledgeGraph) AddEntity(ctx context.Context, entity *Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entity == nil {
		return fmt.Errorf("entity is nil")
	}
	if entity.ID == "" {
		return fmt.Errorf("entity id is required")
	}

	now := time.Now()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now

	g.mu.Lock()
	defer g.mu.Unlock()

	// 存储副本
	copied := *entity
	g.entities[entity.ID] = &copied

	g.logger.Debug("entity added",
		zap.String("id", entity.ID),
		zap.String("type", entity.Type),
		zap.String("name", entity.Name))

	return nil
}

// AddRelation 添加关系到知识图谱。
func (g *InMemoryKnowledgeGraph) AddRelation(ctx context.Context, relation *Relation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if relation == nil {
		return fmt.Errorf("relation is nil")
	}
	if relation.ID == "" {
		relation.ID = fmt.Sprintf("rel_%d", time.Now().UnixNano())
	}
	if relation.FromID == "" || relation.ToID == "" {
		return fmt.Errorf("relation from_id and to_id are required")
	}
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// 存储副本
	copied := *relation
	g.relations[relation.ID] = &copied
	g.outRels[relation.FromID] = append(g.outRels[relation.FromID], relation.ID)
	g.inRels[relation.ToID] = append(g.inRels[relation.ToID], relation.ID)

	g.logger.Debug("relation added",
		zap.String("id", relation.ID),
		zap.String("from", relation.FromID),
		zap.String("to", relation.ToID),
		zap.String("type", relation.Type))

	return nil
}

// QueryEntity 按 ID 查询实体。
func (g *InMemoryKnowledgeGraph) QueryEntity(ctx context.Context, id string) (*Entity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, fmt.Errorf("entity id is required")
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	entity, ok := g.entities[id]
	if !ok {
		return nil, fmt.Errorf("entity %q not found", id)
	}

	// 返回副本
	copied := *entity
	return &copied, nil
}

// QueryRelations 查询与指定实体相关的关系。
// 如果 relationType 非空，则只返回匹配类型的关系。
func (g *InMemoryKnowledgeGraph) QueryRelations(ctx context.Context, entityID string, relationType string) ([]Relation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if entityID == "" {
		return nil, fmt.Errorf("entity id is required")
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make([]Relation, 0)

	// 收集出边关系
	for _, relID := range g.outRels[entityID] {
		rel, ok := g.relations[relID]
		if !ok {
			continue
		}
		if relationType != "" && rel.Type != relationType {
			continue
		}
		results = append(results, *rel)
	}

	// 收集入边关系
	for _, relID := range g.inRels[entityID] {
		rel, ok := g.relations[relID]
		if !ok {
			continue
		}
		if relationType != "" && rel.Type != relationType {
			continue
		}
		results = append(results, *rel)
	}

	return results, nil
}

// FindPath 在知识图谱中查找从 fromID 到 toID 的路径。
// maxDepth 限制搜索深度，返回所有找到的路径（每条路径为实体 ID 序列）。
func (g *InMemoryKnowledgeGraph) FindPath(ctx context.Context, fromID, toID string, maxDepth int) ([][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if fromID == "" || toID == "" {
		return nil, fmt.Errorf("from_id and to_id are required")
	}
	if maxDepth <= 0 {
		return nil, nil
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	// 检查起点和终点是否存在
	if _, ok := g.entities[fromID]; !ok {
		return nil, fmt.Errorf("entity %q not found", fromID)
	}
	if _, ok := g.entities[toID]; !ok {
		return nil, fmt.Errorf("entity %q not found", toID)
	}

	var paths [][]string
	visited := make(map[string]bool)
	g.dfs(ctx, fromID, toID, maxDepth, visited, []string{fromID}, &paths)

	return paths, nil
}

// dfs 深度优先搜索路径（需在持有读锁时调用）。
func (g *InMemoryKnowledgeGraph) dfs(ctx context.Context, current, target string, depth int, visited map[string]bool, path []string, paths *[][]string) {
	if ctx.Err() != nil {
		return
	}
	if current == target && len(path) > 1 {
		// 找到一条路径，复制后保存
		found := make([]string, len(path))
		copy(found, path)
		*paths = append(*paths, found)
		return
	}
	if depth <= 0 {
		return
	}

	visited[current] = true
	defer func() { visited[current] = false }()

	// 遍历出边
	for _, relID := range g.outRels[current] {
		rel, ok := g.relations[relID]
		if !ok {
			continue
		}
		next := rel.ToID
		if visited[next] {
			continue
		}
		g.dfs(ctx, next, target, depth-1, visited, append(path, next), paths)
	}

	// 遍历入边（双向搜索）
	for _, relID := range g.inRels[current] {
		rel, ok := g.relations[relID]
		if !ok {
			continue
		}
		next := rel.FromID
		if visited[next] {
			continue
		}
		g.dfs(ctx, next, target, depth-1, visited, append(path, next), paths)
	}
}
