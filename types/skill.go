package types

// DiscoveredSkill 表示发现的技能概要信息。
// 由 agent/skills 层使用，在 agent 层接口中引用。
type DiscoveredSkill struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Category     string   `json:"category,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// GetInstructions 返回技能指令，供 agent 层注入/增强 prompt。
func (s *DiscoveredSkill) GetInstructions() string {
	if s == nil {
		return ""
	}
	return s.Instructions
}
