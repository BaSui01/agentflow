package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SkillsExtensionAdapter adapts DefaultSkillManager and Registry for extension-registry integration.
// Exposes LoadSkill, ExecuteSkill, and ListSkills for skill loading, execution, and discovery.
type SkillsExtensionAdapter struct {
	manager  SkillManager
	registry *SkillRegistry
}

// NewSkillsExtensionAdapter creates a new adapter.
// manager is used for skill loading/discovery, registry is used for skill execution.
// If registry is nil, ExecuteSkill will return an error.
func NewSkillsExtensionAdapter(manager SkillManager, registry *SkillRegistry) *SkillsExtensionAdapter {
	return &SkillsExtensionAdapter{
		manager:  manager,
		registry: registry,
	}
}

// LoadSkill loads a skill by name. It searches the manager's index by name,
// then loads the matching skill.
func (a *SkillsExtensionAdapter) LoadSkill(ctx context.Context, name string) error {
	skillID, err := a.resolveSkillID(name)
	if err != nil {
		return err
	}
	_, err = a.manager.LoadSkill(ctx, skillID)
	return err
}

// ExecuteSkill executes a loaded skill by name.
// It first checks the Registry (which supports handlers), then falls back to
// returning the skill's instructions as output.
func (a *SkillsExtensionAdapter) ExecuteSkill(ctx context.Context, name string, input any) (any, error) {
	// Try registry first — it has actual handlers
	if a.registry != nil {
		skillID := strings.TrimSpace(name)
		if resolvedID, err := a.resolveSkillID(name); err == nil {
			skillID = resolvedID
		}
		if instance, ok := a.resolveRegistrySkill(name, skillID); ok && instance.Handler != nil && instance.Enabled {
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

	skillID, err := a.resolveSkillID(name)
	if err != nil {
		return nil, err
	}

	// Fallback: find skill in manager and return its instructions
	skill, ok := a.manager.GetSkill(skillID)
	if !ok || skill == nil {
		skill, err = a.manager.LoadSkill(ctx, skillID)
		if err != nil {
			return nil, fmt.Errorf("load skill %s: %w", name, err)
		}
		ok = true
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
	if a.manager == nil {
		return nil
	}
	metadata := a.manager.ListSkills()
	names := make([]string, len(metadata))
	for i, meta := range metadata {
		names[i] = meta.Name
	}
	return names
}

func (a *SkillsExtensionAdapter) resolveSkillID(name string) (string, error) {
	if a.manager == nil {
		return "", fmt.Errorf("skill manager is not configured")
	}

	needle := strings.TrimSpace(name)
	if needle == "" {
		return "", fmt.Errorf("skill not found: %s", name)
	}

	for _, meta := range a.manager.ListSkills() {
		if meta == nil {
			continue
		}
		if meta.ID == needle || meta.Name == needle {
			return meta.ID, nil
		}
	}

	return "", fmt.Errorf("skill not found: %s", name)
}

func (a *SkillsExtensionAdapter) resolveRegistrySkill(name, skillID string) (*SkillInstance, bool) {
	if a.registry == nil {
		return nil, false
	}
	if instance, ok := a.registry.Get(skillID); ok {
		return instance, true
	}
	if instance, ok := a.registry.GetByName(name); ok {
		return instance, true
	}
	return nil, false
}
