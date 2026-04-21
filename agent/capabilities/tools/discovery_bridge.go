package skills

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// Skills → Discovery Bridge (§15 Workflow-Local Interfaces)
// =============================================================================
// The bridge uses local interfaces to avoid circular dependencies:
//   skills → discovery → a2a → agent → skills (cycle!)
// Instead, we define CapabilityRegistrar and CapabilityDescriptor locally.
// The concrete discovery.Registry satisfies CapabilityRegistrar via an adapter
// wired at the application layer.
// =============================================================================

// CapabilityDescriptor describes a capability for registration in a discovery system.
type CapabilityDescriptor struct {
	Name        string
	Description string
	Category    string // "task", "query", or "stream"
	AgentID     string
	AgentName   string
	Tags        []string
	Metadata    map[string]string
}

// CapabilityRegistrar is the interface for registering capabilities in a discovery system.
// Implement this interface to bridge skills with your discovery layer.
type CapabilityRegistrar interface {
	RegisterCapability(ctx context.Context, descriptor *CapabilityDescriptor) error
	UnregisterCapability(ctx context.Context, agentID string, capabilityName string) error
}

// SkillDiscoveryBridge bridges the Skills system to a Discovery system.
// It converts Skills into CapabilityDescriptors and registers them via a CapabilityRegistrar.
type SkillDiscoveryBridge struct {
	skillManager SkillManager
	registrar    CapabilityRegistrar
	agentID      string
	logger       *zap.Logger
}

// NewSkillDiscoveryBridge creates a new bridge between Skills and Discovery.
func NewSkillDiscoveryBridge(
	skillManager SkillManager,
	registrar CapabilityRegistrar,
	agentID string,
	logger *zap.Logger,
) *SkillDiscoveryBridge {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SkillDiscoveryBridge{
		skillManager: skillManager,
		registrar:    registrar,
		agentID:      agentID,
		logger:       logger.With(zap.String("component", "skill_discovery_bridge")),
	}
}

// RegisterSkillAsCapability converts a Skill to a CapabilityDescriptor and registers it.
func (b *SkillDiscoveryBridge) RegisterSkillAsCapability(ctx context.Context, skill *Skill) error {
	if skill == nil {
		return fmt.Errorf("skill is nil")
	}

	desc := skillToDescriptor(skill, b.agentID)

	if err := b.registrar.RegisterCapability(ctx, desc); err != nil {
		return fmt.Errorf("register skill %s as capability: %w", skill.ID, err)
	}

	b.logger.Info("skill registered as capability",
		zap.String("skill_id", skill.ID),
		zap.String("capability_name", desc.Name),
	)
	return nil
}

// SyncAll synchronizes all registered Skills to the Discovery system.
func (b *SkillDiscoveryBridge) SyncAll(ctx context.Context) error {
	skills := b.skillManager.ListSkills()
	if len(skills) == 0 {
		b.logger.Debug("no skills to sync")
		return nil
	}

	var syncErrors []error
	synced := 0

	for _, meta := range skills {
		skill, err := b.skillManager.LoadSkill(ctx, meta.ID)
		if err != nil {
			b.logger.Warn("failed to load skill for sync",
				zap.String("skill_id", meta.ID),
				zap.Error(err),
			)
			syncErrors = append(syncErrors, err)
			continue
		}

		if err := b.RegisterSkillAsCapability(ctx, skill); err != nil {
			b.logger.Warn("failed to register skill as capability",
				zap.String("skill_id", meta.ID),
				zap.Error(err),
			)
			syncErrors = append(syncErrors, err)
			continue
		}
		synced++
	}

	b.logger.Info("skill sync completed",
		zap.Int("synced", synced),
		zap.Int("errors", len(syncErrors)),
	)

	if len(syncErrors) > 0 {
		return fmt.Errorf("sync completed with %d errors (first: %w)", len(syncErrors), syncErrors[0])
	}
	return nil
}

// UnregisterSkill removes a skill's capability from the discovery system.
func (b *SkillDiscoveryBridge) UnregisterSkill(ctx context.Context, skillID string) error {
	return b.registrar.UnregisterCapability(ctx, b.agentID, skillID)
}

// skillToDescriptor converts a Skill to a CapabilityDescriptor.
func skillToDescriptor(skill *Skill, agentID string) *CapabilityDescriptor {
	return &CapabilityDescriptor{
		Name:        skill.ID,
		Description: skill.Description,
		Category:    mapSkillCategoryToCapType(skill.Category),
		AgentID:     agentID,
		AgentName:   skill.Name,
		Tags:        skill.Tags,
		Metadata:    buildSkillMetadata(skill),
	}
}

// mapSkillCategoryToCapType maps a skill category string to a capability type string.
func mapSkillCategoryToCapType(category string) string {
	switch SkillCategory(category) {
	case CategoryCoding, CategoryAutomation:
		return "task"
	case CategoryResearch, CategoryData, CategoryReasoning:
		return "query"
	case CategoryCommunication:
		return "stream"
	default:
		return "task"
	}
}

// buildSkillMetadata builds metadata map from skill fields.
func buildSkillMetadata(skill *Skill) map[string]string {
	meta := map[string]string{
		"source":    "skills",
		"skill_id":  skill.ID,
		"version":   skill.Version,
		"synced_at": time.Now().Format(time.RFC3339),
	}
	if skill.Category != "" {
		meta["category"] = skill.Category
	}
	if skill.Author != "" {
		meta["author"] = skill.Author
	}
	return meta
}
