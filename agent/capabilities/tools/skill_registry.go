package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type SkillCategory string

const (
	CategoryCoding        SkillCategory = "coding"
	CategoryAutomation    SkillCategory = "automation"
	CategoryResearch      SkillCategory = "research"
	CategoryData          SkillCategory = "data"
	CategoryReasoning     SkillCategory = "reasoning"
	CategoryCommunication SkillCategory = "communication"
)

type SkillDefinition struct {
	ID          string        `json:"id"`
	Name        string        `json:"name,omitempty"`
	Description string        `json:"description,omitempty"`
	Category    SkillCategory `json:"category,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
}

type SkillHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

type SkillStats struct {
	Invocations int64      `json:"invocations"`
	Successes   int64      `json:"successes"`
	Failures    int64      `json:"failures"`
	LastInvoked *time.Time `json:"last_invoked,omitempty"`
}

type SkillInstance struct {
	Definition *SkillDefinition `json:"definition"`
	Handler    SkillHandler     `json:"-"`
	Enabled    bool             `json:"enabled"`
	Stats      SkillStats       `json:"stats"`
}

type SkillRegistry struct {
	mu      sync.RWMutex
	skills  map[string]*SkillInstance
	counter int64
	logger  *zap.Logger
}

func NewRegistry(logger *zap.Logger) *SkillRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SkillRegistry{
		skills: make(map[string]*SkillInstance),
		logger: logger.With(zap.String("component", "skill_registry")),
	}
}

func (r *SkillRegistry) Register(def *SkillDefinition, handler SkillHandler) error {
	if def == nil {
		return fmt.Errorf("skill definition is nil")
	}
	if strings.TrimSpace(def.ID) == "" {
		r.counter++
		def.ID = fmt.Sprintf("skill_%d", r.counter)
	}
	inst := &SkillInstance{
		Definition: cloneSkillDefinition(def),
		Handler:    handler,
		Enabled:    true,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[def.ID] = inst
	return nil
}

func (r *SkillRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.skills[id]; !ok {
		return fmt.Errorf("skill not found: %s", id)
	}
	delete(r.skills, id)
	return nil
}

func (r *SkillRegistry) Get(id string) (*SkillInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.skills[id]
	if !ok {
		return nil, false
	}
	return cloneSkillInstance(inst), true
}

func (r *SkillRegistry) GetByName(name string) (*SkillInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.TrimSpace(strings.ToLower(name))
	for _, inst := range r.skills {
		if strings.TrimSpace(strings.ToLower(inst.Definition.Name)) == needle {
			return cloneSkillInstance(inst), true
		}
	}
	return nil, false
}

func (r *SkillRegistry) ListByCategory(category SkillCategory) []*SkillInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*SkillInstance, 0)
	for _, inst := range r.skills {
		if inst.Definition.Category == category {
			out = append(out, cloneSkillInstance(inst))
		}
	}
	return out
}

func (r *SkillRegistry) ListAll() []*SkillInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*SkillInstance, 0, len(r.skills))
	for _, inst := range r.skills {
		out = append(out, cloneSkillInstance(inst))
	}
	return out
}

func (r *SkillRegistry) Search(query string, tags []string) []*SkillInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.TrimSpace(strings.ToLower(query))
	out := make([]*SkillInstance, 0)
	for _, inst := range r.skills {
		if q != "" {
			name := strings.ToLower(inst.Definition.Name)
			desc := strings.ToLower(inst.Definition.Description)
			if !strings.Contains(name, q) && !strings.Contains(desc, q) {
				continue
			}
		}
		if len(tags) > 0 && !hasAnyTag(inst.Definition.Tags, tags) {
			continue
		}
		out = append(out, cloneSkillInstance(inst))
	}
	return out
}

func (r *SkillRegistry) Invoke(ctx context.Context, id string, input json.RawMessage) (json.RawMessage, error) {
	r.mu.Lock()
	inst, ok := r.skills[id]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("skill not found: %s", id)
	}
	if !inst.Enabled {
		r.mu.Unlock()
		return nil, fmt.Errorf("skill %s is disabled", id)
	}
	if inst.Handler == nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("skill %s has no handler", id)
	}
	now := time.Now()
	inst.Stats.Invocations++
	inst.Stats.LastInvoked = &now
	handler := inst.Handler
	r.mu.Unlock()

	result, err := handler(ctx, input)

	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.skills[id]; ok {
		if err != nil {
			current.Stats.Failures++
		} else {
			current.Stats.Successes++
		}
	}
	return result, err
}

func (r *SkillRegistry) Disable(id string) error {
	return r.setEnabled(id, false)
}

func (r *SkillRegistry) Enable(id string) error {
	return r.setEnabled(id, true)
}

func (r *SkillRegistry) setEnabled(id string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.skills[id]
	if !ok {
		return fmt.Errorf("skill not found: %s", id)
	}
	inst.Enabled = enabled
	return nil
}

func (r *SkillRegistry) Export() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]*SkillDefinition, 0, len(r.skills))
	for _, inst := range r.skills {
		defs = append(defs, cloneSkillDefinition(inst.Definition))
	}
	return json.Marshal(defs)
}

func (r *SkillRegistry) Import(data []byte) error {
	var defs []*SkillDefinition
	if err := json.Unmarshal(data, &defs); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, def := range defs {
		if def == nil || strings.TrimSpace(def.ID) == "" {
			continue
		}
		r.skills[def.ID] = &SkillInstance{
			Definition: cloneSkillDefinition(def),
			Enabled:    false,
		}
	}
	return nil
}

func cloneSkillDefinition(def *SkillDefinition) *SkillDefinition {
	if def == nil {
		return nil
	}
	out := *def
	out.Tags = append([]string(nil), def.Tags...)
	return &out
}

func cloneSkillInstance(inst *SkillInstance) *SkillInstance {
	if inst == nil {
		return nil
	}
	out := *inst
	out.Definition = cloneSkillDefinition(inst.Definition)
	return &out
}

func hasAnyTag(haystack []string, needles []string) bool {
	for _, existing := range haystack {
		existing = strings.ToLower(strings.TrimSpace(existing))
		for _, needle := range needles {
			if existing == strings.ToLower(strings.TrimSpace(needle)) {
				return true
			}
		}
	}
	return false
}
