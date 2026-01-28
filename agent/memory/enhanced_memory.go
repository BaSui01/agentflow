package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EnhancedMemorySystem 增强的多层记忆系统
// 实现短期、工作、长期、情节和语义记忆
type EnhancedMemorySystem struct {
	// 短期记忆（Redis/内存）- 最近的交互
	shortTerm MemoryStore

	// 工作记忆（内存）- 当前任务的上下文
	working MemoryStore

	// 长期记忆（向量数据库）- 持久化的重要信息
	longTerm VectorStore

	// 情节记忆（时序数据库）- 时间序列事件
	episodic EpisodicStore

	// 语义记忆（知识图谱）- 结构化知识
	semantic KnowledgeGraph

	// 记忆整合器
	consolidator *MemoryConsolidator

	// 配置
	config EnhancedMemoryConfig

	logger *zap.Logger
}

// EnhancedMemoryConfig 增强记忆配置
type EnhancedMemoryConfig struct {
	// 短期记忆配置
	ShortTermTTL     time.Duration `json:"short_term_ttl"`      // 短期记忆 TTL
	ShortTermMaxSize int           `json:"short_term_max_size"` // 最大条目数

	// 工作记忆配置
	WorkingMemorySize int `json:"working_memory_size"` // 工作记忆容量

	// 长期记忆配置
	LongTermEnabled bool `json:"long_term_enabled"` // 是否启用长期记忆
	VectorDimension int  `json:"vector_dimension"`  // 向量维度

	// 情节记忆配置
	EpisodicEnabled   bool          `json:"episodic_enabled"`   // 是否启用情节记忆
	EpisodicRetention time.Duration `json:"episodic_retention"` // 情节记忆保留时间

	// 语义记忆配置
	SemanticEnabled bool `json:"semantic_enabled"` // 是否启用语义记忆

	// 记忆整合配置
	ConsolidationEnabled  bool          `json:"consolidation_enabled"`  // 是否启用记忆整合
	ConsolidationInterval time.Duration `json:"consolidation_interval"` // 整合间隔
}

// DefaultEnhancedMemoryConfig 默认配置
func DefaultEnhancedMemoryConfig() EnhancedMemoryConfig {
	return EnhancedMemoryConfig{
		ShortTermTTL:          24 * time.Hour,
		ShortTermMaxSize:      100,
		WorkingMemorySize:     20,
		LongTermEnabled:       true,
		VectorDimension:       1536,
		EpisodicEnabled:       true,
		EpisodicRetention:     30 * 24 * time.Hour, // 30 天
		SemanticEnabled:       true,
		ConsolidationEnabled:  true,
		ConsolidationInterval: 1 * time.Hour,
	}
}

// MemoryStore 通用记忆存储接口
type MemoryStore interface {
	Save(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Load(ctx context.Context, key string) (interface{}, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, pattern string, limit int) ([]interface{}, error)
	Clear(ctx context.Context) error
}

// VectorStore 向量存储接口（用于语义搜索）
type VectorStore interface {
	// 存储向量
	Store(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error

	// 语义搜索
	Search(ctx context.Context, query []float64, topK int, filter map[string]interface{}) ([]VectorSearchResult, error)

	// 删除向量
	Delete(ctx context.Context, id string) error

	// 批量操作
	BatchStore(ctx context.Context, items []VectorItem) error
}

// VectorSearchResult 向量搜索结果
type VectorSearchResult struct {
	ID       string                 `json:"id"`
	Score    float64                `json:"score"` // 相似度分数
	Metadata map[string]interface{} `json:"metadata"`
}

// VectorItem 向量项
type VectorItem struct {
	ID       string
	Vector   []float64
	Metadata map[string]interface{}
}

// EpisodicStore 情节记忆存储接口
type EpisodicStore interface {
	// 记录事件
	RecordEvent(ctx context.Context, event *EpisodicEvent) error

	// 查询事件
	QueryEvents(ctx context.Context, query EpisodicQuery) ([]EpisodicEvent, error)

	// 获取时间线
	GetTimeline(ctx context.Context, agentID string, start, end time.Time) ([]EpisodicEvent, error)
}

// EpisodicEvent 情节事件
type EpisodicEvent struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agent_id"`
	Type      string                 `json:"type"`    // 事件类型
	Content   string                 `json:"content"` // 事件内容
	Context   map[string]interface{} `json:"context"` // 上下文
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration"` // 事件持续时间
}

// EpisodicQuery 情节查询
type EpisodicQuery struct {
	AgentID   string
	Type      string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// KnowledgeGraph 知识图谱接口
type KnowledgeGraph interface {
	// 添加实体
	AddEntity(ctx context.Context, entity *Entity) error

	// 添加关系
	AddRelation(ctx context.Context, relation *Relation) error

	// 查询实体
	QueryEntity(ctx context.Context, id string) (*Entity, error)

	// 查询关系
	QueryRelations(ctx context.Context, entityID string, relationType string) ([]Relation, error)

	// 路径查询
	FindPath(ctx context.Context, fromID, toID string, maxDepth int) ([][]string, error)
}

// Entity 实体
type Entity struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// Relation 关系
type Relation struct {
	ID         string                 `json:"id"`
	FromID     string                 `json:"from_id"`
	ToID       string                 `json:"to_id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Weight     float64                `json:"weight"`
	CreatedAt  time.Time              `json:"created_at"`
}

// MemoryConsolidator 记忆整合器
type MemoryConsolidator struct {
	system *EnhancedMemorySystem

	// 整合策略
	strategies []ConsolidationStrategy

	// 运行状态
	running bool
	stopCh  chan struct{}
	mu      sync.Mutex

	logger *zap.Logger
}

// ConsolidationStrategy 整合策略接口
type ConsolidationStrategy interface {
	// 判断是否应该整合
	ShouldConsolidate(ctx context.Context, memory interface{}) bool

	// 执行整合
	Consolidate(ctx context.Context, memories []interface{}) error
}

// NewEnhancedMemorySystem 创建增强记忆系统
func NewEnhancedMemorySystem(
	shortTerm MemoryStore,
	working MemoryStore,
	longTerm VectorStore,
	episodic EpisodicStore,
	semantic KnowledgeGraph,
	config EnhancedMemoryConfig,
	logger *zap.Logger,
) *EnhancedMemorySystem {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	system := &EnhancedMemorySystem{
		shortTerm: shortTerm,
		working:   working,
		longTerm:  longTerm,
		episodic:  episodic,
		semantic:  semantic,
		config:    config,
		logger:    logger.With(zap.String("component", "enhanced_memory")),
	}

	// 创建记忆整合器
	if config.ConsolidationEnabled {
		system.consolidator = NewMemoryConsolidator(system, logger)
	}

	return system
}

// NewDefaultEnhancedMemorySystem creates an EnhancedMemorySystem with in-memory default stores.
// It is intended for local development, tests, and quick starts.
func NewDefaultEnhancedMemorySystem(config EnhancedMemoryConfig, logger *zap.Logger) *EnhancedMemorySystem {
	if logger == nil {
		logger = zap.NewNop()
	}

	shortTerm := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
		MaxEntries: config.ShortTermMaxSize,
	}, logger)
	working := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
		MaxEntries: config.WorkingMemorySize,
	}, logger)

	var longTerm VectorStore
	if config.LongTermEnabled {
		longTerm = NewInMemoryVectorStore(InMemoryVectorStoreConfig{Dimension: config.VectorDimension}, logger)
	}

	system := NewEnhancedMemorySystem(shortTerm, working, longTerm, nil, nil, config, logger)
	if config.ConsolidationEnabled {
		_ = system.AddDefaultConsolidationStrategies()
	}
	return system
}

// SaveShortTerm 保存短期记忆
func (m *EnhancedMemorySystem) SaveShortTerm(ctx context.Context, agentID string, content string, metadata map[string]interface{}) error {
	if m.shortTerm == nil {
		return fmt.Errorf("short-term memory not configured")
	}

	key := fmt.Sprintf("short_term:%s:%d", agentID, time.Now().UnixNano())

	memory := map[string]interface{}{
		"key":       key,
		"agent_id":  agentID,
		"content":   content,
		"metadata":  metadata,
		"timestamp": time.Now(),
	}

	return m.shortTerm.Save(ctx, key, memory, m.config.ShortTermTTL)
}

// SaveShortTermWithVector saves a short-term memory entry and attaches a vector in metadata.
// Built-in consolidation strategies can promote such entries to long-term memory.
func (m *EnhancedMemorySystem) SaveShortTermWithVector(
	ctx context.Context,
	agentID string,
	content string,
	vector []float64,
	metadata map[string]interface{},
) error {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["vector"] = vector
	return m.SaveShortTerm(ctx, agentID, content, metadata)
}

// LoadShortTerm 加载短期记忆
func (m *EnhancedMemorySystem) LoadShortTerm(ctx context.Context, agentID string, limit int) ([]interface{}, error) {
	if m.shortTerm == nil {
		return nil, fmt.Errorf("short-term memory not configured")
	}

	pattern := fmt.Sprintf("short_term:%s:*", agentID)
	return m.shortTerm.List(ctx, pattern, limit)
}

// SaveWorking 保存工作记忆
func (m *EnhancedMemorySystem) SaveWorking(ctx context.Context, agentID string, content string, metadata map[string]interface{}) error {
	if m.working == nil {
		return fmt.Errorf("working memory not configured")
	}

	key := fmt.Sprintf("working:%s:%d", agentID, time.Now().UnixNano())

	memory := map[string]interface{}{
		"key":       key,
		"agent_id":  agentID,
		"content":   content,
		"metadata":  metadata,
		"timestamp": time.Now(),
	}

	return m.working.Save(ctx, key, memory, 0) // 工作记忆不过期
}

// LoadWorking 加载工作记忆
func (m *EnhancedMemorySystem) LoadWorking(ctx context.Context, agentID string) ([]interface{}, error) {
	if m.working == nil {
		return nil, fmt.Errorf("working memory not configured")
	}

	pattern := fmt.Sprintf("working:%s:*", agentID)
	return m.working.List(ctx, pattern, m.config.WorkingMemorySize)
}

// ClearWorking 清除工作记忆
func (m *EnhancedMemorySystem) ClearWorking(ctx context.Context, agentID string) error {
	if m.working == nil {
		return fmt.Errorf("working memory not configured")
	}

	return m.working.Clear(ctx)
}

// SaveLongTerm 保存长期记忆（向量化）
func (m *EnhancedMemorySystem) SaveLongTerm(ctx context.Context, agentID string, content string, vector []float64, metadata map[string]interface{}) error {
	if !m.config.LongTermEnabled || m.longTerm == nil {
		return fmt.Errorf("long-term memory not configured")
	}

	id := fmt.Sprintf("long_term:%s:%d", agentID, time.Now().UnixNano())

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["agent_id"] = agentID
	metadata["content"] = content
	metadata["timestamp"] = time.Now()

	return m.longTerm.Store(ctx, id, vector, metadata)
}

// SearchLongTerm 搜索长期记忆
func (m *EnhancedMemorySystem) SearchLongTerm(ctx context.Context, agentID string, queryVector []float64, topK int) ([]VectorSearchResult, error) {
	if !m.config.LongTermEnabled || m.longTerm == nil {
		return nil, fmt.Errorf("long-term memory not configured")
	}

	filter := map[string]interface{}{
		"agent_id": agentID,
	}

	return m.longTerm.Search(ctx, queryVector, topK, filter)
}

// RecordEpisode 记录情节
func (m *EnhancedMemorySystem) RecordEpisode(ctx context.Context, event *EpisodicEvent) error {
	if !m.config.EpisodicEnabled || m.episodic == nil {
		return fmt.Errorf("episodic memory not configured")
	}

	return m.episodic.RecordEvent(ctx, event)
}

// QueryEpisodes 查询情节
func (m *EnhancedMemorySystem) QueryEpisodes(ctx context.Context, query EpisodicQuery) ([]EpisodicEvent, error) {
	if !m.config.EpisodicEnabled || m.episodic == nil {
		return nil, fmt.Errorf("episodic memory not configured")
	}

	return m.episodic.QueryEvents(ctx, query)
}

// AddKnowledge 添加知识（实体和关系）
func (m *EnhancedMemorySystem) AddKnowledge(ctx context.Context, entity *Entity) error {
	if !m.config.SemanticEnabled || m.semantic == nil {
		return fmt.Errorf("semantic memory not configured")
	}

	return m.semantic.AddEntity(ctx, entity)
}

// AddKnowledgeRelation 添加知识关系
func (m *EnhancedMemorySystem) AddKnowledgeRelation(ctx context.Context, relation *Relation) error {
	if !m.config.SemanticEnabled || m.semantic == nil {
		return fmt.Errorf("semantic memory not configured")
	}

	return m.semantic.AddRelation(ctx, relation)
}

// QueryKnowledge 查询知识
func (m *EnhancedMemorySystem) QueryKnowledge(ctx context.Context, entityID string) (*Entity, error) {
	if !m.config.SemanticEnabled || m.semantic == nil {
		return nil, fmt.Errorf("semantic memory not configured")
	}

	return m.semantic.QueryEntity(ctx, entityID)
}

// StartConsolidation 启动记忆整合
func (m *EnhancedMemorySystem) StartConsolidation(ctx context.Context) error {
	if !m.config.ConsolidationEnabled || m.consolidator == nil {
		return fmt.Errorf("memory consolidation not configured")
	}

	return m.consolidator.Start(ctx)
}

// StopConsolidation 停止记忆整合
func (m *EnhancedMemorySystem) StopConsolidation() error {
	if m.consolidator == nil {
		return nil
	}

	return m.consolidator.Stop()
}

// ConsolidateOnce triggers one consolidation run (useful for manual runs and tests).
func (m *EnhancedMemorySystem) ConsolidateOnce(ctx context.Context) error {
	if !m.config.ConsolidationEnabled || m.consolidator == nil {
		return fmt.Errorf("memory consolidation not configured")
	}
	return m.consolidator.consolidate(ctx)
}

// AddConsolidationStrategy adds a consolidation strategy.
func (m *EnhancedMemorySystem) AddConsolidationStrategy(strategy ConsolidationStrategy) error {
	if !m.config.ConsolidationEnabled || m.consolidator == nil {
		return fmt.Errorf("memory consolidation not configured")
	}
	if strategy == nil {
		return fmt.Errorf("strategy is nil")
	}
	m.consolidator.AddStrategy(strategy)
	return nil
}

// NewMemoryConsolidator 创建记忆整合器
func NewMemoryConsolidator(system *EnhancedMemorySystem, logger *zap.Logger) *MemoryConsolidator {
	return &MemoryConsolidator{
		system:     system,
		strategies: []ConsolidationStrategy{},
		logger:     logger.With(zap.String("component", "memory_consolidator")),
	}
}

// Start 启动整合器
func (c *MemoryConsolidator) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consolidator already running")
	}
	c.stopCh = make(chan struct{})
	c.running = true
	c.mu.Unlock()

	go c.run(ctx)

	c.logger.Info("memory consolidator started")

	return nil
}

// Stop 停止整合器
func (c *MemoryConsolidator) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return fmt.Errorf("consolidator not running")
	}

	if c.stopCh != nil {
		close(c.stopCh)
	}
	c.running = false

	c.logger.Info("memory consolidator stopped")

	return nil
}

// run 运行整合循环
func (c *MemoryConsolidator) run(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	ticker := time.NewTicker(c.system.config.ConsolidationInterval)
	defer ticker.Stop()

	// 创建带超时的子上下文
	consolidationTimeout := 5 * time.Minute
	if c.system.config.ConsolidationInterval > 0 {
		consolidationTimeout = c.system.config.ConsolidationInterval / 2
	}

	for {
		select {
		case <-ticker.C:
			// 为每次整合创建带超时的上下文
			consolidateCtx, cancel := context.WithTimeout(ctx, consolidationTimeout)
			if err := c.consolidate(consolidateCtx); err != nil {
				c.logger.Error("consolidation failed", zap.Error(err))
			}
			cancel() // 确保释放资源
		case <-c.stopCh:
			c.logger.Debug("consolidator stopped via stopCh")
			return
		case <-ctx.Done():
			c.logger.Debug("consolidator stopped via context cancellation")
			return
		}
	}
}

// consolidate 执行记忆整合
func (c *MemoryConsolidator) consolidate(ctx context.Context) error {
	c.logger.Debug("starting memory consolidation")

	c.mu.Lock()
	strategies := append([]ConsolidationStrategy(nil), c.strategies...)
	c.mu.Unlock()

	if len(strategies) == 0 {
		c.logger.Debug("no consolidation strategies configured")
		return nil
	}

	var memories []interface{}

	if c.system.shortTerm != nil {
		limit := c.system.config.ShortTermMaxSize * 100
		if limit <= 0 {
			limit = 1000
		}
		items, err := c.system.shortTerm.List(ctx, "short_term:*", limit)
		if err != nil {
			return fmt.Errorf("list short-term memories: %w", err)
		}
		memories = append(memories, items...)
	}

	if c.system.working != nil {
		limit := c.system.config.WorkingMemorySize * 100
		if limit <= 0 {
			limit = 1000
		}
		items, err := c.system.working.List(ctx, "working:*", limit)
		if err != nil {
			return fmt.Errorf("list working memories: %w", err)
		}
		memories = append(memories, items...)
	}

	if len(memories) == 0 {
		c.logger.Debug("no memories available for consolidation")
		return nil
	}

	for _, strategy := range strategies {
		if err := ctx.Err(); err != nil {
			return err
		}

		var selected []interface{}
		for _, mem := range memories {
			if err := ctx.Err(); err != nil {
				return err
			}
			if strategy.ShouldConsolidate(ctx, mem) {
				selected = append(selected, mem)
			}
		}
		if len(selected) == 0 {
			continue
		}
		if err := strategy.Consolidate(ctx, selected); err != nil {
			return err
		}
	}

	c.logger.Debug("memory consolidation completed")

	return nil
}

// AddStrategy 添加整合策略
func (c *MemoryConsolidator) AddStrategy(strategy ConsolidationStrategy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.strategies = append(c.strategies, strategy)
}

func extractMemoryKey(memory interface{}) (string, bool) {
	m, ok := memory.(map[string]interface{})
	if !ok {
		return "", false
	}
	key, ok := m["key"].(string)
	return key, ok && key != ""
}
