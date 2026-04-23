package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// LongTermRetriever provides a higher-quality retrieval path for long-term
// memory search. When set, it replaces the raw VectorStore.Search
// with a RAG pipeline (e.g. BM25+Vector+Rerank fusion).
// Uses types.RetrievalRecord to avoid coupling to rag layer implementation.
type LongTermRetriever interface {
	Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]types.RetrievalRecord, error)
}

// EnhancedMemorySystem 增强的多层记忆系统
// 实现短期、工作、长期、情节、语义和观测记忆
type EnhancedMemorySystem struct {
	// 短期记忆（Redis/内存）- 最近的交互
	shortTerm MemoryStore

	// 工作记忆（内存）- 当前任务的上下文
	working MemoryStore

	// 长期记忆（向量数据库）- 持久化的重要信息
	// 使用 types.VectorStore 接口解耦对 rag 包的直接依赖
	longTerm types.VectorStore

	// 长期记忆高级检索器（可选）— 走 RAG 管线获得更高质量结果
	longTermRetriever LongTermRetriever

	// 情节记忆（时序数据库）- 时间序列事件
	episodic EpisodicStore

	// 语义记忆（知识图谱）- 结构化知识
	semantic KnowledgeGraph

	// 观测记忆 - 对话压缩与精炼
	observationStore ObservationStore
	observer         *Observer
	reflector        *Reflector

	// 记忆整合器
	consolidator     *MemoryConsolidator
	consolidatorOnce sync.Once // 确保 consolidator 只初始化一次

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

	// 观测记忆配置
	ObservationEnabled bool           `json:"observation_enabled"` // 是否启用观测记忆
	ObserverConfig     ObserverConfig `json:"observer_config"`     // Observer 配置

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
	Save(ctx context.Context, key string, value any, ttl time.Duration) error
	Load(ctx context.Context, key string) (any, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, pattern string, limit int) ([]any, error)
	Clear(ctx context.Context) error
}

func toStoreEntries(raw []any) []types.MemoryEntry {
	entries := make([]types.MemoryEntry, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := types.MemoryEntry{}
		if v, ok := m["key"].(string); ok {
			entry.Key = v
		}
		if v, ok := m["agent_id"].(string); ok {
			entry.AgentID = v
		}
		if v, ok := m["content"].(string); ok {
			entry.Content = v
		}
		if v, ok := m["metadata"].(map[string]any); ok {
			entry.Metadata = v
		}
		if v, ok := m["timestamp"].(time.Time); ok {
			entry.Timestamp = v
		}
		entries = append(entries, entry)
	}
	return entries
}

// VectorItem 向量项
type VectorItem struct {
	ID       string
	Vector   []float64
	Metadata map[string]any
}

// BatchVectorStore extends VectorStore with batch operations.
// This is memory-specific and not part of the shared types interface.
type BatchVectorStore interface {
	types.VectorStore
	BatchStore(ctx context.Context, items []VectorItem) error
}

// EpisodicStore 情节记忆存储接口
type EpisodicStore interface {
	// 记录事件
	RecordEvent(ctx context.Context, event *types.EpisodicEvent) error

	// 查询事件
	QueryEvents(ctx context.Context, query EpisodicQuery) ([]types.EpisodicEvent, error)

	// 获取时间线
	GetTimeline(ctx context.Context, agentID string, start, end time.Time) ([]types.EpisodicEvent, error)
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
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Relation 关系
type Relation struct {
	ID         string         `json:"id"`
	FromID     string         `json:"from_id"`
	ToID       string         `json:"to_id"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Weight     float64        `json:"weight"`
	CreatedAt  time.Time      `json:"created_at"`
}

// MemoryConsolidator 记忆整合器
type MemoryConsolidator struct {
	system *EnhancedMemorySystem

	// 整合策略
	strategies []ConsolidationStrategy

	// 运行状态
	running   bool
	runEpoch  uint64
	stopCh    chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex

	logger *zap.Logger
}

// ConsolidationStrategy 整合策略接口
type ConsolidationStrategy interface {
	// 判断是否应该整合
	ShouldConsolidate(ctx context.Context, memory any) bool

	// 执行整合
	Consolidate(ctx context.Context, memories []any) error
}

// NewEnhancedMemorySystem 创建增强记忆系统
func NewEnhancedMemorySystem(
	shortTerm MemoryStore,
	working MemoryStore,
	longTerm types.VectorStore,
	episodic EpisodicStore,
	semantic KnowledgeGraph,
	observationStore ObservationStore,
	config EnhancedMemoryConfig,
	logger *zap.Logger,
) *EnhancedMemorySystem {
	if logger == nil {
		logger = zap.NewNop()
	}

	system := &EnhancedMemorySystem{
		shortTerm:        shortTerm,
		working:          working,
		longTerm:         longTerm,
		episodic:         episodic,
		semantic:         semantic,
		observationStore: observationStore,
		config:           config,
		logger:           logger.With(zap.String("component", "enhanced_memory")),
	}

	if config.ConsolidationEnabled {
		system.initConsolidator(logger)
	}

	return system
}

// NewDefault Enhanced MemorySystem 创建了带有memory默认商店的增强记忆系统.
// 它旨在地方发展、测试和快速启动。
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

	var longTerm types.VectorStore
	if config.LongTermEnabled {
		longTerm = NewInMemoryVectorStore(InMemoryVectorStoreConfig{Dimension: config.VectorDimension}, logger)
	}

	system := NewEnhancedMemorySystem(shortTerm, working, longTerm, nil, nil, nil, config, logger)
	if config.ConsolidationEnabled {
		_ = system.AddDefaultConsolidationStrategies()
	}
	return system
}

// SaveShortTerm 保存短期记忆
func (m *EnhancedMemorySystem) SaveShortTerm(ctx context.Context, agentID string, content string, metadata map[string]any) error {
	if m.shortTerm == nil {
		return fmt.Errorf("short-term memory store not configured")
	}
	key := fmt.Sprintf("short_term:%s:%d", agentID, time.Now().UnixNano())

	memory := map[string]any{
		"key":       key,
		"agent_id":  agentID,
		"content":   content,
		"metadata":  metadata,
		"timestamp": time.Now(),
	}

	return m.shortTerm.Save(ctx, key, memory, m.config.ShortTermTTL)
}

// 保存ShortTermWith Vector 保存一个短期内存条目并附加元数据中的向量.
// 内建的合并战略可以促进此类条目的长期记忆。
func (m *EnhancedMemorySystem) SaveShortTermWithVector(
	ctx context.Context,
	agentID string,
	content string,
	vector []float64,
	metadata map[string]any,
) error {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["vector"] = vector
	return m.SaveShortTerm(ctx, agentID, content, metadata)
}

// LoadShortTerm 加载短期记忆
func (m *EnhancedMemorySystem) LoadShortTerm(ctx context.Context, agentID string, limit int) ([]types.MemoryEntry, error) {
	if m.shortTerm == nil {
		return nil, fmt.Errorf("short-term memory store not configured")
	}
	pattern := fmt.Sprintf("short_term:%s:*", agentID)
	raw, err := m.shortTerm.List(ctx, pattern, limit)
	if err != nil {
		return nil, err
	}
	return toStoreEntries(raw), nil
}

// SaveWorking 保存工作记忆
func (m *EnhancedMemorySystem) SaveWorking(ctx context.Context, agentID string, content string, metadata map[string]any) error {
	if m.working == nil {
		return fmt.Errorf("working memory store not configured")
	}
	key := fmt.Sprintf("working:%s:%d", agentID, time.Now().UnixNano())

	memory := map[string]any{
		"key":       key,
		"agent_id":  agentID,
		"content":   content,
		"metadata":  metadata,
		"timestamp": time.Now(),
	}

	return m.working.Save(ctx, key, memory, 0) // 工作记忆不过期
}

// LoadWorking 加载工作记忆
func (m *EnhancedMemorySystem) LoadWorking(ctx context.Context, agentID string) ([]types.MemoryEntry, error) {
	if m.working == nil {
		return nil, fmt.Errorf("working memory store not configured")
	}
	pattern := fmt.Sprintf("working:%s:*", agentID)
	raw, err := m.working.List(ctx, pattern, m.config.WorkingMemorySize)
	if err != nil {
		return nil, err
	}
	return toStoreEntries(raw), nil
}

// ClearWorking 清除工作记忆
func (m *EnhancedMemorySystem) ClearWorking(ctx context.Context, agentID string) error {
	if m.working == nil {
		return fmt.Errorf("working memory store not configured")
	}
	return m.working.Clear(ctx)
}

// SaveLongTerm 保存长期记忆（向量化）
func (m *EnhancedMemorySystem) SaveLongTerm(ctx context.Context, agentID string, content string, vector []float64, metadata map[string]any) error {
	if !m.config.LongTermEnabled {
		return fmt.Errorf("long-term memory not enabled")
	}

	id := fmt.Sprintf("long_term:%s:%d", agentID, time.Now().UnixNano())

	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["agent_id"] = agentID
	metadata["content"] = content
	metadata["timestamp"] = time.Now()

	return m.longTerm.Store(ctx, id, vector, metadata)
}

// SetLongTermRetriever injects a higher-quality retriever (e.g. HybridRetriever
// from the RAG pipeline) for long-term memory search.
func (m *EnhancedMemorySystem) SetLongTermRetriever(r LongTermRetriever) {
	m.longTermRetriever = r
}

// SearchLongTerm 搜索长期记忆
// When a LongTermRetriever is set, it is used instead of raw vector search.
func (m *EnhancedMemorySystem) SearchLongTerm(ctx context.Context, agentID string, queryVector []float64, topK int) ([]types.VectorSearchResult, error) {
	if !m.config.LongTermEnabled {
		return nil, fmt.Errorf("long-term memory not enabled")
	}

	if m.longTermRetriever != nil {
		return m.searchViaRetriever(ctx, agentID, queryVector, topK)
	}

	filter := map[string]any{
		"agent_id": agentID,
	}
	return m.longTerm.Search(ctx, queryVector, topK, filter)
}

func (m *EnhancedMemorySystem) searchViaRetriever(ctx context.Context, agentID string, queryVector []float64, topK int) ([]types.VectorSearchResult, error) {
	query := fmt.Sprintf("agent:%s long-term memory", agentID)
	results, err := m.longTermRetriever.Retrieve(ctx, query, queryVector)
	if err != nil {
		m.logger.Warn("retriever search failed, falling back to vector search",
			zap.Error(err), zap.String("agent_id", agentID))
		return m.longTerm.Search(ctx, queryVector, topK, map[string]any{"agent_id": agentID})
	}

	out := make([]types.VectorSearchResult, 0, len(results))
	for _, r := range results {
		if len(out) >= topK {
			break
		}
		out = append(out, types.VectorSearchResult{
			ID:    r.DocID,
			Score: r.Score,
			Metadata: map[string]any{
				"source":  r.Source,
				"content": r.Content,
			},
		})
	}
	return out, nil
}

// RecordEpisode 记录情节
func (m *EnhancedMemorySystem) RecordEpisode(ctx context.Context, event *types.EpisodicEvent) error {
	if !m.config.EpisodicEnabled {
		return fmt.Errorf("episodic memory not enabled")
	}
	if m.episodic == nil {
		return fmt.Errorf("episodic memory store not configured")
	}

	return m.episodic.RecordEvent(ctx, event)
}

// QueryEpisodes 查询情节
func (m *EnhancedMemorySystem) QueryEpisodes(ctx context.Context, query EpisodicQuery) ([]types.EpisodicEvent, error) {
	if !m.config.EpisodicEnabled {
		return nil, fmt.Errorf("episodic memory not enabled")
	}
	if m.episodic == nil {
		return nil, fmt.Errorf("episodic memory store not configured")
	}

	return m.episodic.QueryEvents(ctx, query)
}

// AddKnowledge 添加知识（实体和关系）
func (m *EnhancedMemorySystem) AddKnowledge(ctx context.Context, entity *Entity) error {
	if !m.config.SemanticEnabled {
		return fmt.Errorf("semantic memory not configured")
	}

	return m.semantic.AddEntity(ctx, entity)
}

// AddKnowledgeRelation 添加知识关系
func (m *EnhancedMemorySystem) AddKnowledgeRelation(ctx context.Context, relation *Relation) error {
	if !m.config.SemanticEnabled {
		return fmt.Errorf("semantic memory not configured")
	}

	return m.semantic.AddRelation(ctx, relation)
}

// QueryKnowledge 查询知识
func (m *EnhancedMemorySystem) QueryKnowledge(ctx context.Context, entityID string) (*Entity, error) {
	if !m.config.SemanticEnabled {
		return nil, fmt.Errorf("semantic memory not configured")
	}

	return m.semantic.QueryEntity(ctx, entityID)
}

// EnableObservationPipeline 启用观测管线（Observer + Reflector），
// 需要 LLM CompletionFunc 来驱动压缩与精炼。
func (m *EnhancedMemorySystem) EnableObservationPipeline(completeFn CompletionFunc) {
	cfg := m.config.ObserverConfig
	if cfg.MaxMessagesPerBatch == 0 {
		cfg = DefaultObserverConfig()
	}
	m.observer = NewObserver(cfg, completeFn, m.logger)
	m.reflector = NewReflector(completeFn, m.logger)
}

// ProcessObservation 对一批对话消息执行观测：Observer 压缩 -> Reflector 精炼 -> Store 持久化。
func (m *EnhancedMemorySystem) ProcessObservation(ctx context.Context, agentID string, messages []types.Message) (*Observation, error) {
	if !m.config.ObservationEnabled || m.observationStore == nil {
		return nil, fmt.Errorf("observation memory not configured")
	}
	if m.observer == nil {
		return nil, fmt.Errorf("observation pipeline not enabled, call EnableObservationPipeline first")
	}

	draft, err := m.observer.Observe(ctx, agentID, messages)
	if err != nil {
		return nil, fmt.Errorf("observer: %w", err)
	}
	if draft == nil {
		return nil, nil
	}

	if m.reflector != nil {
		existing, _ := m.observationStore.LoadRecent(ctx, agentID, 10)
		draft, err = m.reflector.Reflect(ctx, existing, draft)
		if err != nil {
			return nil, fmt.Errorf("reflector: %w", err)
		}
	}

	if err := m.observationStore.Save(ctx, *draft); err != nil {
		return nil, fmt.Errorf("save observation: %w", err)
	}

	m.logger.Debug("observation processed",
		zap.String("agent_id", agentID),
		zap.String("obs_id", draft.ID),
	)
	return draft, nil
}

// GetRecentObservations 从 Store 加载最近的观测记录。
func (m *EnhancedMemorySystem) GetRecentObservations(ctx context.Context, agentID string, limit int) ([]Observation, error) {
	if !m.config.ObservationEnabled || m.observationStore == nil {
		return nil, fmt.Errorf("observation memory not configured")
	}
	return m.observationStore.LoadRecent(ctx, agentID, limit)
}

// BuildObservationContext 构建观测上下文摘要，可直接拼接到 system prompt。
func (m *EnhancedMemorySystem) BuildObservationContext(ctx context.Context, agentID string, limit int) (string, error) {
	if !m.config.ObservationEnabled || m.observationStore == nil {
		return "", nil
	}

	observations, err := m.observationStore.LoadRecent(ctx, agentID, limit)
	if err != nil {
		return "", fmt.Errorf("load observations: %w", err)
	}
	if len(observations) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Observation Memory\n\n")
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		fmt.Fprintf(&sb, "[%s] %s\n\n", obs.Date, obs.Content)
	}
	return sb.String(), nil
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

// 合并后启动一次合并运行(对手工运行和测试有用)。
func (m *EnhancedMemorySystem) ConsolidateOnce(ctx context.Context) error {
	if !m.config.ConsolidationEnabled || m.consolidator == nil {
		return fmt.Errorf("memory consolidation not configured")
	}
	return m.consolidator.consolidate(ctx)
}

// 添加整合战略增加了整合战略.
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

// initConsolidator 使用 sync.Once 确保 consolidator 只初始化一次，
// 防止并发调用导致重复创建。
func (m *EnhancedMemorySystem) initConsolidator(logger *zap.Logger) {
	m.consolidatorOnce.Do(func() {
		m.consolidator = NewMemoryConsolidator(m, logger)
	})
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
	c.closeOnce = sync.Once{}
	c.runEpoch++
	epoch := c.runEpoch
	stopCh := c.stopCh
	c.running = true
	c.mu.Unlock()

	go c.run(ctx, epoch, stopCh)

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
		c.closeOnce.Do(func() { close(c.stopCh) })
	}
	c.running = false

	c.logger.Info("memory consolidator stopped")

	return nil
}

// run 运行整合循环
func (c *MemoryConsolidator) run(ctx context.Context, epoch uint64, stopCh chan struct{}) {
	defer func() {
		c.mu.Lock()
		// 仅允许当前代次 goroutine 更新运行状态，避免旧 goroutine 覆盖新 Start 状态。
		if c.runEpoch == epoch {
			c.running = false
			if c.stopCh == stopCh {
				c.stopCh = nil
			}
		}
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
		case <-stopCh:
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

	var memories []any

	{
		if c.system.shortTerm == nil {
			return fmt.Errorf("short-term memory store not configured")
		}
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

	{
		if c.system.working == nil {
			if c.system.config.WorkingMemorySize <= 0 {
				goto processStrategies
			}
			return fmt.Errorf("working memory store not configured")
		}
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

processStrategies:
	if len(memories) == 0 {
		c.logger.Debug("no memories available for consolidation")
		return nil
	}

	for _, strategy := range strategies {
		if err := ctx.Err(); err != nil {
			return err
		}

		var selected []any
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

func extractMemoryKey(memory any) (string, bool) {
	m, ok := memory.(map[string]any)
	if !ok {
		return "", false
	}
	key, ok := m["key"].(string)
	return key, ok && key != ""
}
