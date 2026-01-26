// Package memory provides layered memory systems for AI agents.
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

// MemoryType defines the type of memory.
// Deprecated: Use types.MemoryCategory instead for new code.
type MemoryType = types.MemoryCategory

// Memory type constants - mapped to unified types.MemoryCategory
// Deprecated: Use types.MemoryEpisodic, types.MemorySemantic, etc.
const (
	MemoryTypeEpisodic   = types.MemoryEpisodic   // Event-based memories
	MemoryTypeSemantic   = types.MemorySemantic   // Factual knowledge
	MemoryTypeWorking    = types.MemoryWorking    // Short-term context
	MemoryTypeProcedural = types.MemoryProcedural // How-to knowledge
)

// MemoryEntry represents a single memory entry.
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

// EpisodicMemory stores event-based experiences.
type EpisodicMemory struct {
	episodes []*Episode
	maxSize  int
	logger   *zap.Logger
	mu       sync.RWMutex
}

// Episode represents a single episode/event.
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

// NewEpisodicMemory creates a new episodic memory store.
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

// Store stores a new episode.
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

	// Evict old episodes if over capacity
	if len(m.episodes) > m.maxSize {
		m.episodes = m.episodes[1:]
	}
}

// Recall retrieves recent episodes.
func (m *EpisodicMemory) Recall(limit int) []*Episode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.episodes) {
		limit = len(m.episodes)
	}

	start := len(m.episodes) - limit
	return append([]*Episode{}, m.episodes[start:]...)
}

// Search searches episodes by context.
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

// SemanticMemory stores factual knowledge.
type SemanticMemory struct {
	facts    map[string]*Fact
	embedder Embedder
	logger   *zap.Logger
	mu       sync.RWMutex
}

// Fact represents a piece of factual knowledge.
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

// Embedder generates embeddings for text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewSemanticMemory creates a new semantic memory store.
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

// StoreFact stores a fact.
func (m *SemanticMemory) StoreFact(ctx context.Context, fact *Fact) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if fact.ID == "" {
		fact.ID = fmt.Sprintf("fact_%d", time.Now().UnixNano())
	}
	fact.CreatedAt = time.Now()
	fact.UpdatedAt = time.Now()

	// Generate embedding
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

// Query queries facts by subject.
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

// GetFact retrieves a fact by ID.
func (m *SemanticMemory) GetFact(id string) (*Fact, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.facts[id]
	return f, ok
}

// WorkingMemory provides short-term context storage.
type WorkingMemory struct {
	items    []WorkingItem
	capacity int
	ttl      time.Duration
	logger   *zap.Logger
	mu       sync.RWMutex
}

// WorkingItem represents an item in working memory.
type WorkingItem struct {
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewWorkingMemory creates a new working memory.
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

// Set sets a value in working memory.
func (m *WorkingMemory) Set(key string, value any, priority int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove existing item with same key
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

	// Evict low priority items if over capacity
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

// Get retrieves a value from working memory.
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

// Clear clears expired items.
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

// GetAll returns all non-expired items.
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

// LayeredMemory combines all memory types.
type LayeredMemory struct {
	Episodic   *EpisodicMemory
	Semantic   *SemanticMemory
	Working    *WorkingMemory
	Procedural *ProceduralMemory
	logger     *zap.Logger
}

// LayeredMemoryConfig configures layered memory.
type LayeredMemoryConfig struct {
	EpisodicMaxSize  int
	WorkingCapacity  int
	WorkingTTL       time.Duration
	Embedder         Embedder
	ProceduralConfig ProceduralConfig
}

// NewLayeredMemory creates a new layered memory system.
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

// Export exports all memory to JSON.
func (lm *LayeredMemory) Export() ([]byte, error) {
	data := map[string]any{
		"episodic": lm.Episodic.Recall(100),
		"working":  lm.Working.GetAll(),
	}
	return json.MarshalIndent(data, "", "  ")
}

// ProceduralConfig configures procedural memory.
type ProceduralConfig struct {
	MaxProcedures int `json:"max_procedures"`
}

// ProceduralMemory stores how-to knowledge.
type ProceduralMemory struct {
	procedures map[string]*Procedure
	config     ProceduralConfig
	logger     *zap.Logger
	mu         sync.RWMutex
}

// Procedure represents a learned procedure.
type Procedure struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Triggers    []string `json:"triggers"`
	SuccessRate float64  `json:"success_rate"`
	Executions  int      `json:"executions"`
}

// NewProceduralMemory creates a new procedural memory.
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

// Store stores a procedure.
func (m *ProceduralMemory) Store(proc *Procedure) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if proc.ID == "" {
		proc.ID = fmt.Sprintf("proc_%d", time.Now().UnixNano())
	}
	m.procedures[proc.ID] = proc
}

// Get retrieves a procedure by ID.
func (m *ProceduralMemory) Get(id string) (*Procedure, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.procedures[id]
	return p, ok
}

// FindByTrigger finds procedures by trigger.
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
