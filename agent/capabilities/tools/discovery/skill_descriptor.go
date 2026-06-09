package discovery

import "time"

// SkillProfile is the discovery-layer view needed to expose a skill as a capability.
type SkillProfile struct {
	ID           string
	Name         string
	Description  string
	Instructions string
	Version      string
	Category     string
	Tags         []string
	Author       string
}

// SkillCapabilityDescriptor describes a skill capability for discovery registration.
type SkillCapabilityDescriptor struct {
	Name        string
	Description string
	Category    string
	AgentID     string
	AgentName   string
	Tags        []string
	Metadata    map[string]string
}

// SkillDiscoveryResult is the discovery-layer view returned to agent prompt assembly.
type SkillDiscoveryResult struct {
	ID           string
	Name         string
	Description  string
	Instructions string
	Category     string
	Tags         []string
}

// SkillIndexEntry is the discovery-layer view used for skill index records.
type SkillIndexEntry struct {
	ID          string
	Name        string
	Description string
	Category    string
	Tags        []string
	Version     string
	Path        string
}

// MapSkillCategoryToCapabilityType maps a skill category string to a capability type string.
func MapSkillCategoryToCapabilityType(category string) string {
	switch category {
	case "coding", "automation":
		return "task"
	case "research", "data", "reasoning":
		return "query"
	case "communication":
		return "stream"
	default:
		return "task"
	}
}

// SkillDescriptorFromProfile converts a skill profile into a discovery descriptor.
func SkillDescriptorFromProfile(profile SkillProfile, agentID string, syncedAt time.Time) SkillCapabilityDescriptor {
	metadata := map[string]string{
		"source":    "skills",
		"skill_id":  profile.ID,
		"version":   profile.Version,
		"synced_at": syncedAt.Format(time.RFC3339),
	}
	if profile.Category != "" {
		metadata["category"] = profile.Category
	}
	if profile.Author != "" {
		metadata["author"] = profile.Author
	}

	return SkillCapabilityDescriptor{
		Name:        profile.ID,
		Description: profile.Description,
		Category:    MapSkillCategoryToCapabilityType(profile.Category),
		AgentID:     agentID,
		AgentName:   profile.Name,
		Tags:        append([]string(nil), profile.Tags...),
		Metadata:    metadata,
	}
}

// DiscoveredSkillFromProfile converts a skill profile into a discovery result DTO.
func DiscoveredSkillFromProfile(profile SkillProfile) SkillDiscoveryResult {
	return SkillDiscoveryResult{
		ID:           profile.ID,
		Name:         profile.Name,
		Description:  profile.Description,
		Instructions: profile.Instructions,
		Category:     profile.Category,
		Tags:         append([]string(nil), profile.Tags...),
	}
}

// SkillIndexEntryFromProfile converts a skill profile into an index record.
func SkillIndexEntryFromProfile(profile SkillProfile, path string) SkillIndexEntry {
	return SkillIndexEntry{
		ID:          profile.ID,
		Name:        profile.Name,
		Description: profile.Description,
		Category:    profile.Category,
		Tags:        append([]string(nil), profile.Tags...),
		Version:     profile.Version,
		Path:        path,
	}
}
