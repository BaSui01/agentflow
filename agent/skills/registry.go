package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 技能类定义了技能类别.
type SkillCategory string

const (
	CategoryReasoning     SkillCategory = "reasoning"
	CategoryCoding        SkillCategory = "coding"
	CategoryResearch      SkillCategory = "research"
	CategoryCommunication SkillCategory = "communication"
	CategoryData          SkillCategory = "data"
	CategoryAutomation    SkillCategory = "automation"
)

// SkillDefinition定义了一种标准化的技能.
type SkillDefinition struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Category     SkillCategory   `json:"category"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Requirements []string        `json:"requirements,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Author       string          `json:"author,omitempty"`
	License      string          `json:"license,omitempty"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// 技能Instance是一个注册技能案例。
type SkillInstance struct {
	Definition *SkillDefinition `json:"definition"`
	Handler    SkillHandler     `json:"-"`
	Enabled    bool             `json:"enabled"`
	Stats      SkillStats       `json:"stats"`
}

// SkillStats跟踪技能使用统计.
type SkillStats struct {
	Invocations int64         `json:"invocations"`
	Successes   int64         `json:"successes"`
	Failures    int64         `json:"failures"`
	AvgLatency  time.Duration `json:"avg_latency"`
	LastInvoked *time.Time    `json:"last_invoked,omitempty"`
}

// SkillHandler执行一种技能.
type SkillHandler func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)

// 书记官处管理技能登记和发现。
type Registry struct {
	skills     map[string]*SkillInstance
	byCategory map[SkillCategory][]*SkillInstance
	logger     *zap.Logger
	mu         sync.RWMutex
}

// NewRegistry创建了新的技能注册.
func NewRegistry(logger *zap.Logger) *Registry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Registry{
		skills:     make(map[string]*SkillInstance),
		byCategory: make(map[SkillCategory][]*SkillInstance),
		logger:     logger.With(zap.String("component", "skill_registry")),
	}
}

// 注册注册技能。
func (r *Registry) Register(def *SkillDefinition, handler SkillHandler) error {
	if def.ID == "" {
		def.ID = fmt.Sprintf("skill_%d", time.Now().UnixNano())
	}
	if def.CreatedAt.IsZero() {
		def.CreatedAt = time.Now()
	}
	def.UpdatedAt = time.Now()

	instance := &SkillInstance{
		Definition: def,
		Handler:    handler,
		Enabled:    true,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.skills[def.ID] = instance
	r.byCategory[def.Category] = append(r.byCategory[def.Category], instance)

	r.logger.Info("skill registered",
		zap.String("id", def.ID),
		zap.String("name", def.Name),
		zap.String("category", string(def.Category)),
	)

	return nil
}

// 未注册者删除一个技能。
func (r *Registry) Unregister(skillID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, ok := r.skills[skillID]
	if !ok {
		return fmt.Errorf("skill not found: %s", skillID)
	}

	delete(r.skills, skillID)

	// 从类别索引中删除
	cat := instance.Definition.Category
	skills := r.byCategory[cat]
	for i, s := range skills {
		if s.Definition.ID == skillID {
			r.byCategory[cat] = append(skills[:i], skills[i+1:]...)
			break
		}
	}

	r.logger.Info("skill unregistered", zap.String("id", skillID))
	return nil
}

// 以身份获取技能。
func (r *Registry) Get(skillID string) (*SkillInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	skill, ok := r.skills[skillID]
	return skill, ok
}

// GetByName 按名称检索技能 。
func (r *Registry) GetByName(name string) (*SkillInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, skill := range r.skills {
		if skill.Definition.Name == name {
			return skill, true
		}
	}
	return nil, false
}

// ListByCategory 在一个类别中返回技能.
func (r *Registry) ListByCategory(category SkillCategory) []*SkillInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]*SkillInstance{}, r.byCategory[category]...)
}

// ListAll 返回所有注册技能 。
func (r *Registry) ListAll() []*SkillInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	skills := make([]*SkillInstance, 0, len(r.skills))
	for _, s := range r.skills {
		skills = append(skills, s)
	}
	return skills
}

// 通过标签或关键词搜索搜索技能.
func (r *Registry) Search(query string, tags []string) []*SkillInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*SkillInstance
	for _, skill := range r.skills {
		// 按名称或描述匹配
		if query != "" {
			if contains(skill.Definition.Name, query) ||
				contains(skill.Definition.Description, query) {
				results = append(results, skill)
				continue
			}
		}
		// 按标签匹配
		if len(tags) > 0 {
			for _, tag := range tags {
				for _, skillTag := range skill.Definition.Tags {
					if skillTag == tag {
						results = append(results, skill)
						break
					}
				}
			}
		}
	}
	return results
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Invoke 引用一个技能。
func (r *Registry) Invoke(ctx context.Context, skillID string, input json.RawMessage) (json.RawMessage, error) {
	r.mu.RLock()
	skill, ok := r.skills[skillID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("skill not found: %s", skillID)
	}

	if !skill.Enabled {
		return nil, fmt.Errorf("skill disabled: %s", skillID)
	}

	start := time.Now()
	result, err := skill.Handler(ctx, input)
	latency := time.Since(start)

	// 更新数据
	r.mu.Lock()
	skill.Stats.Invocations++
	if err != nil {
		skill.Stats.Failures++
	} else {
		skill.Stats.Successes++
	}
	now := time.Now()
	skill.Stats.LastInvoked = &now
	// 更新平均延迟
	n := skill.Stats.Invocations
	skill.Stats.AvgLatency = time.Duration((int64(skill.Stats.AvgLatency)*(n-1) + int64(latency)) / n)
	r.mu.Unlock()

	return result, err
}

// 启用一个技能 。
func (r *Registry) Enable(skillID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	skill, ok := r.skills[skillID]
	if !ok {
		return fmt.Errorf("skill not found: %s", skillID)
	}
	skill.Enabled = true
	return nil
}

// 禁用一个技能 。
func (r *Registry) Disable(skillID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	skill, ok := r.skills[skillID]
	if !ok {
		return fmt.Errorf("skill not found: %s", skillID)
	}
	skill.Enabled = false
	return nil
}

// 出口所有技能定义。
func (r *Registry) Export() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]*SkillDefinition, 0, len(r.skills))
	for _, s := range r.skills {
		defs = append(defs, s.Definition)
	}
	return json.MarshalIndent(defs, "", "  ")
}

// 进口技能定义(手提人必须单独登记).
func (r *Registry) Import(data []byte) error {
	var defs []*SkillDefinition
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, def := range defs {
		if _, exists := r.skills[def.ID]; !exists {
			r.skills[def.ID] = &SkillInstance{
				Definition: def,
				Enabled:    false, // No handler yet
			}
			r.byCategory[def.Category] = append(r.byCategory[def.Category], r.skills[def.ID])
		}
	}

	r.logger.Info("skills imported", zap.Int("count", len(defs)))
	return nil
}
