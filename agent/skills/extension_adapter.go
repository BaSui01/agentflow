package skills

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"encoding/json"
	"fmt"
)

// SkillsExtensionAdapter adapts DefaultSkillManager to the types.SkillsExtension interface.
// This bridges the gap between the Skills system and the extension registry.
//
// types.SkillsExtension interface:
//   - LoadSkill(ctx context.Context, name string) error
//   - ExecuteSkill(ctx context.Context, name string, input any) (any, error)
//   - ListSkills() []string
type SkillsExtensionAdapter struct {
	manager  *DefaultSkillManager
	registry *Registry
}

// NewSkillsExtensionAdapter creates a new adapter.
// manager is used for skill loading/discovery, registry is used for skill execution.
// If registry is nil, ExecuteSkill will return an error.
func NewSkillsExtensionAdapter(manager *DefaultSkillManager, registry *Registry) *SkillsExtensionAdapter {
	return &SkillsExtensionAdapter{
		manager:  manager,
		registry: registry,
	}
}

// LoadSkill loads a skill by name. It searches the manager's index by name,
// then loads the matching skill.
func (a *SkillsExtensionAdapter) LoadSkill(ctx context.Context, name string) error {
	// Search by name in the manager's index
	metadata := a.manager.ListSkills()
	for _, meta := range metadata {
		if meta.Name == name || meta.ID == name {
			_, err := a.manager.LoadSkill(ctx, meta.ID)
			return err
		}
	}
	return fmt.Errorf("skill not found: %s", name)
}

// ExecuteSkill executes a loaded skill by name.
// It first checks the Registry (which supports handlers), then falls back to
// returning the skill's instructions as output.
func (a *SkillsExtensionAdapter) ExecuteSkill(ctx context.Context, name string, input any) (any, error) {
	// Try registry first — it has actual handlers
	if a.registry != nil {
		if instance, ok := a.registry.GetByName(name); ok && instance.Handler != nil && instance.Enabled {
			inputJSON, err := json.Marshal(input)
			if err != nil {
				return nil, fmt.Errorf("marshal skill input: %w", err)
			}
			result, err := a.registry.Invoke(ctx, instance.Definition.ID, inputJSON)
			if err != nil {
				return nil, err
			}
			var output any
			if err := json.Unmarshal(result, &output); err != nil {
				// Return raw string if not valid JSON
				return string(result), nil
			}
			return output, nil
		}
	}

	// Fallback: find skill in manager and return its instructions
	skill, ok := a.manager.GetSkill(name)
	if !ok {
		// Try searching by name
		metadata := a.manager.ListSkills()
		for _, meta := range metadata {
			if meta.Name == name {
				var err error
				skill, err = a.manager.LoadSkill(ctx, meta.ID)
				if err != nil {
					return nil, fmt.Errorf("load skill %s: %w", name, err)
				}
				ok = true
				break
			}
		}
	}

	if !ok || skill == nil {
		return nil, fmt.Errorf("skill not found or not loaded: %s", name)
	}

	// Return skill instructions as the execution result (placeholder behavior, §14)
	return map[string]any{
		"skill_id":     skill.ID,
		"instructions": skill.GetInstructions(),
		"tools":        skill.Tools,
		"input":        input,
	}, nil
}

// ListSkills returns the names of all available skills.
func (a *SkillsExtensionAdapter) ListSkills() []string {
	metadata := a.manager.ListSkills()
	names := make([]string, len(metadata))
	for i, meta := range metadata {
		names[i] = meta.Name
	}
	return names
}


