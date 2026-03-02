package types

// SkillCategory is the shared contract for skill classification.
type SkillCategory string

const (
	SkillCategoryReasoning     SkillCategory = "reasoning"
	SkillCategoryCoding        SkillCategory = "coding"
	SkillCategoryResearch      SkillCategory = "research"
	SkillCategoryCommunication SkillCategory = "communication"
	SkillCategoryData          SkillCategory = "data"
	SkillCategoryAutomation    SkillCategory = "automation"
)

