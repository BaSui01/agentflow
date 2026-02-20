// 包内存为AI代理提供了分层内存系统.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// 内存Type定义了内存的类型.
// 折旧:使用类型。 用于新代码的内存类型 。
type MemoryType = types.MemoryCategory

// 内存类型常数 - 映射到统一类型. 内存类型
// 折旧:使用类型。 记忆Episodic,类型. 记忆语义等.
const (
	MemoryTypeEpisodic   = types.MemoryEpisodic   // Event-based memories
	MemoryTypeSemantic   = types.MemorySemantic   // Factual knowledge
	MemoryTypeWorking    = types.MemoryWorking    // Short-term context
	MemoryTypeProcedural = types.MemoryProcedural // How-to knowledge
)

// 内存 Entry 代表单个内存条目.
type MemoryEntry struct {
	ID          string         `json:"id"`
	Type        MemoryType     `json:"type"`
	Content     string         `json:"content"`
	Embedding   []float32      `json:"embedding,omitempty"`
	Importance  float64        `json:"importance"`
	AccessCount int            `json:"access_count"`
	CreatedAt   time.Time      `json:"created_at"`
	LastAccess  time.Time      `json:"last_access"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Relations   []string       `json:"relations,omitempty"`
}

// EpisodicMemory存储基于事件的经验.
type EpisodicMemory struct {
	episodes []*Episode
	maxSize  int
	logger   *zap.Logger
	mu       sync.RWMutex
}

// 第一部代表一集/活动.
type Episode struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	Context      string         `json:"context"`
	Action       string         `json:"action"`
	Result       string         `json:"result"`
	Emotion      string         `json:"emotion,omitempty"`
	Importance   float64        `json:"importance"`
	Participants []string       `json:"participants,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// NewEpisodic Memory创建了一款新的偶联记忆商店.
func NewEpisodicMemory(maxSize int, logger *zap.Logger) *EpisodicMemory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EpisodicMemory{
		episodes: make([]*Episode, 0),
		maxSize:  maxSize,
		logger:   logger.With(zap.String("memory", "episodic")),
	}
}

// 存储新剧集。
func (m *EpisodicMemory) Store(ep *Episode) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ep.ID == "" {
		ep.ID = fmt.Sprintf("ep_%d", time.Now().UnixNano())
	}
	if ep.Timestamp.IsZero() {
		ep.Timestamp = time.Now()
	}

	m.episodes = append(m.episodes, ep)

	// 如果容量过大, 就会发生旧事件
	if len(m.episodes) > m.maxSize {
		m.episodes = m.episodes[1:]
	}
}

// 召回最近的一些事件。
func (m *EpisodicMemory) Recall(limit int) []*Episode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.episodes) {
		limit = len(m.episodes)
	}

	start := len(m.episodes) - limit
	return append([]*Episode{}, m.episodes[start:]...)
}

// 按上下文进行搜索 。
func (m *EpisodicMemory) Search(query string, limit int) []*Episode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*Episode
	for _, ep := range m.episodes {
		if contains(ep.Context, query) || contains(ep.Action, query) {
			results = append(results, ep)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 语义记忆存储事实知识.
type SemanticMemory struct {
	facts    map[string]*Fact
	embedder Embedder
	logger   *zap.Logger
	mu       sync.RWMutex
}

// 事实代表了事实知识。
type Fact struct {
	ID         string    `json:"id"`
	Subject    string    `json:"subject"`
	Predicate  string    `json:"predicate"`
	Object     string    `json:"object"`
	Confidence float64   `json:"confidence"`
	Source     string    `json:"source,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// 嵌入器生成文字嵌入.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewSemantic Memory创建了一个新的语义记忆商店.
func NewSemanticMemory(embedder Embedder, logger *zap.Logger) *SemanticMemory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SemanticMemory{
		facts:    make(map[string]*Fact),
		embedder: embedder,
		logger:   logger.With(zap.String("memory", "semantic")),
	}
}

// StoreFact存储了一个事实。
func (m *SemanticMemory) StoreFact(ctx context.Context, fact *Fact) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if fact.ID == "" {
		fact.ID = fmt.Sprintf("fact_%d", time.Now().UnixNano())
	}
	fact.CreatedAt = time.Now()
	fact.UpdatedAt = time.Now()

	// 生成嵌入
	if m.embedder != nil {
		text := fmt.Sprintf("%s %s %s", fact.Subject, fact.Predicate, fact.Object)
		emb, err := m.embedder.Embed(ctx, text)
		if err == nil {
			fact.Embedding = emb
		}
	}

	m.facts[fact.ID] = fact
	return nil
}

// 按主题查询事实。
func (m *SemanticMemory) Query(subject string) []*Fact {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*Fact
	for _, f := range m.facts {
		if f.Subject == subject {
			results = append(results, f)
		}
	}
	return results
}

// Get Fact通过身份证检索一个事实.
func (m *SemanticMemory) GetFact(id string) (*Fact, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.facts[id]
	return f, ok
}

// WorkMemory提供短期上下文存储.
type WorkingMemory struct {
	items    []WorkingItem
	capacity int
	ttl      time.Duration
	logger   *zap.Logger
	mu       sync.RWMutex
}

// 工作 项目是工作记忆中的一个项目。
type WorkingItem struct {
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// 新工作记忆创造出新的工作记忆.
func NewWorkingMemory(capacity int, ttl time.Duration, logger *zap.Logger) *WorkingMemory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WorkingMemory{
		items:    make([]WorkingItem, 0),
		capacity: capacity,
		ttl:      ttl,
		logger:   logger.With(zap.String("memory", "working")),
	}
}

// 设定工作内存中的值 。
func (m *WorkingMemory) Set(key string, value any, priority int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 以相同的密钥删除已存在的项目
	for i, item := range m.items {
		if item.Key == key {
			m.items = append(m.items[:i], m.items[i+1:]...)
			break
		}
	}

	item := WorkingItem{
		Key:       key,
		Value:     value,
		Priority:  priority,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(m.ttl),
	}

	m.items = append(m.items, item)

	// 超能力时优先项目
	if len(m.items) > m.capacity {
		m.evictLowestPriority()
	}
}

func (m *WorkingMemory) evictLowestPriority() {
	if len(m.items) == 0 {
		return
	}
	minIdx := 0
	for i, item := range m.items {
		if item.Priority < m.items[minIdx].Priority {
			minIdx = i
		}
	}
	m.items = append(m.items[:minIdx], m.items[minIdx+1:]...)
}

// 从工作记忆中获取一个值 。
func (m *WorkingMemory) Get(key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, item := range m.items {
		if item.Key == key && time.Now().Before(item.ExpiresAt) {
			return item.Value, true
		}
	}
	return nil, false
}

// 清除过期的项目 。
func (m *WorkingMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var valid []WorkingItem
	for _, item := range m.items {
		if now.Before(item.ExpiresAt) {
			valid = append(valid, item)
		}
	}
	m.items = valid
}

// Get All 返回所有未过期的项目 。
func (m *WorkingMemory) GetAll() []WorkingItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	var results []WorkingItem
	for _, item := range m.items {
		if now.Before(item.ExpiresAt) {
			results = append(results, item)
		}
	}
	return results
}

// 分层记忆结合了所有的内存类型.
type LayeredMemory struct {
	Episodic   *EpisodicMemory
	Semantic   *SemanticMemory
	Working    *WorkingMemory
	Procedural *ProceduralMemory
	logger     *zap.Logger
}

// 分层的MemoryConfig配置分层内存.
type LayeredMemoryConfig struct {
	EpisodicMaxSize  int
	WorkingCapacity  int
	WorkingTTL       time.Duration
	Embedder         Embedder
	ProceduralConfig ProceduralConfig
}

// NewLayered Memory创造了一个新的分层记忆系统.
func NewLayeredMemory(config LayeredMemoryConfig, logger *zap.Logger) *LayeredMemory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LayeredMemory{
		Episodic:   NewEpisodicMemory(config.EpisodicMaxSize, logger),
		Semantic:   NewSemanticMemory(config.Embedder, logger),
		Working:    NewWorkingMemory(config.WorkingCapacity, config.WorkingTTL, logger),
		Procedural: NewProceduralMemory(config.ProceduralConfig, logger),
		logger:     logger.With(zap.String("component", "layered_memory")),
	}
}

// 导出全部内存到 JSON 。
func (lm *LayeredMemory) Export() ([]byte, error) {
	data := map[string]any{
		"episodic": lm.Episodic.Recall(100),
		"working":  lm.Working.GetAll(),
	}
	return json.MarshalIndent(data, "", "  ")
}

// 程序Config配置程序内存.
type ProceduralConfig struct {
	MaxProcedures int `json:"max_procedures"`
}

// 程序记忆存储如何知识。
type ProceduralMemory struct {
	procedures map[string]*Procedure
	config     ProceduralConfig
	logger     *zap.Logger
	mu         sync.RWMutex
}

// 程序是一种学习过的程序。
type Procedure struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Triggers    []string `json:"triggers"`
	SuccessRate float64  `json:"success_rate"`
	Executions  int      `json:"executions"`
}

// 新程序记忆创造出一个新的程序记忆.
func NewProceduralMemory(config ProceduralConfig, logger *zap.Logger) *ProceduralMemory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ProceduralMemory{
		procedures: make(map[string]*Procedure),
		config:     config,
		logger:     logger.With(zap.String("memory", "procedural")),
	}
}

// 存储存储程序。
func (m *ProceduralMemory) Store(proc *Procedure) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if proc.ID == "" {
		proc.ID = fmt.Sprintf("proc_%d", time.Now().UnixNano())
	}
	m.procedures[proc.ID] = proc
}

// 获取一个程序 通过身份。
func (m *ProceduralMemory) Get(id string) (*Procedure, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.procedures[id]
	return p, ok
}

// FindByTrigger通过触发找到程序.
func (m *ProceduralMemory) FindByTrigger(trigger string) []*Procedure {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*Procedure
	for _, p := range m.procedures {
		for _, t := range p.Triggers {
			if t == trigger {
				results = append(results, p)
				break
			}
		}
	}
	return results
}
