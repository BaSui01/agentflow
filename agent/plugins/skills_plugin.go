package plugins

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
)

// SkillsPlugin discovers relevant skills and prepends instructions to the input.
// Phase: BeforeExecute, Priority: 10.
type SkillsPlugin struct {
	discoverer agent.SkillDiscoverer
	query      string // optional override; empty = use input content
}

// NewSkillsPlugin creates a skills discovery plugin.
func NewSkillsPlugin(discoverer agent.SkillDiscoverer, query string) *SkillsPlugin {
	return &SkillsPlugin{
		discoverer: discoverer,
		query:      query,
	}
}

func (p *SkillsPlugin) Name() string              { return "skills" }
func (p *SkillsPlugin) Priority() int              { return 10 }
func (p *SkillsPlugin) Phase() agent.PluginPhase   { return agent.PhaseBeforeExecute }
func (p *SkillsPlugin) Init(_ context.Context) error     { return nil }
func (p *SkillsPlugin) Shutdown(_ context.Context) error { return nil }

// BeforeExecute discovers skills and prepends instructions to the input content.
func (p *SkillsPlugin) BeforeExecute(ctx context.Context, pc *agent.PipelineContext) error {
	query := p.query
	if query == "" {
		query = pc.Input.Content
	}

	found, err := p.discoverer.DiscoverSkills(ctx, query)
	if err != nil {
		// Non-fatal: continue without skills
		return nil
	}

	var instructions []string
	for _, skill := range found {
		if skill == nil {
			continue
		}
		instructions = append(instructions, skill.GetInstructions())
	}

	if len(instructions) > 0 {
		pc.Input.Content = prependSkillInstructions(pc.Input.Content, instructions)
		pc.Metadata["skill_instructions"] = instructions
	}

	return nil
}

// prependSkillInstructions prepends deduplicated skill instructions to the prompt.
func prependSkillInstructions(prompt string, instructions []string) string {
	if len(instructions) == 0 {
		return prompt
	}
	unique := make(map[string]struct{}, len(instructions))
	cleaned := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		if _, exists := unique[instruction]; exists {
			continue
		}
		unique[instruction] = struct{}{}
		cleaned = append(cleaned, instruction)
	}
	if len(cleaned) == 0 {
		return prompt
	}
	var sb strings.Builder
	sb.WriteString("技能执行指令:\n")
	for idx, instruction := range cleaned {
		sb.WriteString(fmt.Sprintf("%d. %s\n", idx+1, instruction))
	}
	sb.WriteString("\n用户请求:\n")
	sb.WriteString(prompt)
	return sb.String()
}
