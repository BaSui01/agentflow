package discovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMapSkillCategoryToCapabilityType(t *testing.T) {
	tests := map[string]string{
		"coding":        "task",
		"automation":    "task",
		"research":      "query",
		"data":          "query",
		"reasoning":     "query",
		"communication": "stream",
		"unknown":       "task",
		"":              "task",
	}

	for category, expected := range tests {
		t.Run(category, func(t *testing.T) {
			assert.Equal(t, expected, MapSkillCategoryToCapabilityType(category))
		})
	}
}

func TestSkillDescriptorFromProfile(t *testing.T) {
	now := time.Date(2026, 5, 13, 1, 2, 3, 0, time.UTC)
	desc := SkillDescriptorFromProfile(SkillProfile{
		ID:          "review",
		Name:        "Code Review",
		Description: "reviews code",
		Version:     "1.2.3",
		Category:    "coding",
		Tags:        []string{"go"},
		Author:      "team",
	}, "agent-1", now)

	assert.Equal(t, "review", desc.Name)
	assert.Equal(t, "reviews code", desc.Description)
	assert.Equal(t, "task", desc.Category)
	assert.Equal(t, "agent-1", desc.AgentID)
	assert.Equal(t, "Code Review", desc.AgentName)
	assert.Equal(t, []string{"go"}, desc.Tags)
	assert.Equal(t, map[string]string{
		"source":    "skills",
		"skill_id":  "review",
		"version":   "1.2.3",
		"synced_at": now.Format(time.RFC3339),
		"category":  "coding",
		"author":    "team",
	}, desc.Metadata)
}

func TestDiscoveredSkillFromProfileCopiesPromptFieldsAndTags(t *testing.T) {
	discovered := DiscoveredSkillFromProfile(SkillProfile{
		ID:           "review",
		Name:         "Code Review",
		Description:  "review code",
		Instructions: "inspect code",
		Category:     "coding",
		Tags:         []string{"go"},
	})

	assert.Equal(t, SkillDiscoveryResult{
		ID:           "review",
		Name:         "Code Review",
		Description:  "review code",
		Instructions: "inspect code",
		Category:     "coding",
		Tags:         []string{"go"},
	}, discovered)

	discovered.Tags[0] = "mutated"
	original := DiscoveredSkillFromProfile(SkillProfile{Tags: []string{"go"}})
	assert.Equal(t, []string{"go"}, original.Tags)
}

func TestSkillIndexEntryFromProfileCopiesMetadataAndTags(t *testing.T) {
	entry := SkillIndexEntryFromProfile(SkillProfile{
		ID:          "review",
		Name:        "Code Review",
		Description: "review code",
		Category:    "coding",
		Tags:        []string{"go"},
		Version:     "1.2.3",
	}, "skills/review")

	assert.Equal(t, SkillIndexEntry{
		ID:          "review",
		Name:        "Code Review",
		Description: "review code",
		Category:    "coding",
		Tags:        []string{"go"},
		Version:     "1.2.3",
		Path:        "skills/review",
	}, entry)

	entry.Tags[0] = "mutated"
	fresh := SkillIndexEntryFromProfile(SkillProfile{Tags: []string{"go"}}, "")
	assert.Equal(t, []string{"go"}, fresh.Tags)
}
