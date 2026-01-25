package agent

import (
	"regexp"
	"strings"

	"github.com/BaSui01/agentflow/llm"
)

// PromptBundle 模块化提示词包（按版本管理）。
//
// 说明：当前版本主要承载 System 模块，其他模块作为扩展点保留。
type PromptBundle struct {
	Version     string            `json:"version"`
	System      SystemPrompt      `json:"system"`
	Tools       []llm.ToolSchema  `json:"tools,omitempty"`
	Examples    []Example         `json:"examples,omitempty"`
	Memory      MemoryConfig      `json:"memory,omitempty"`
	Plan        *PlanConfig       `json:"plan,omitempty"`
	Reflection  *ReflectionConfig `json:"reflection,omitempty"`
	Constraints []string          `json:"constraints,omitempty"`
}

type SystemPrompt struct {
	Role        string   `json:"role,omitempty"`
	Identity    string   `json:"identity,omitempty"`
	Policies    []string `json:"policies,omitempty"`
	OutputRules []string `json:"output_rules,omitempty"`
	Prohibits   []string `json:"prohibits,omitempty"`
}

type Example struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

type MemoryConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type PlanConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type ReflectionConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

func NewPromptBundleFromIdentity(version, identity string) PromptBundle {
	return PromptBundle{
		Version: strings.TrimSpace(version),
		System: SystemPrompt{
			Identity: strings.TrimSpace(identity),
		},
	}
}

func (b PromptBundle) IsZero() bool {
	return strings.TrimSpace(b.Version) == "" && b.System.IsZero() && len(b.Tools) == 0 && len(b.Examples) == 0 && !b.Memory.Enabled && b.Plan == nil && b.Reflection == nil && len(b.Constraints) == 0
}

func (b PromptBundle) EffectiveVersion(defaultVersion string) string {
	if v := strings.TrimSpace(b.Version); v != "" {
		return v
	}
	return strings.TrimSpace(defaultVersion)
}

func (b PromptBundle) RenderSystemPrompt() string {
	var parts []string
	if s := strings.TrimSpace(b.System.Render()); s != "" {
		parts = append(parts, s)
	}
	if len(b.Constraints) > 0 {
		var cs []string
		for _, c := range b.Constraints {
			c = strings.TrimSpace(c)
			if c != "" {
				cs = append(cs, "- "+c)
			}
		}
		if len(cs) > 0 {
			parts = append(parts, "额外约束：\n"+strings.Join(cs, "\n"))
		}
	}
	return strings.Join(parts, "\n\n")
}

func (s SystemPrompt) IsZero() bool {
	return strings.TrimSpace(s.Role) == "" && strings.TrimSpace(s.Identity) == "" && len(s.Policies) == 0 && len(s.OutputRules) == 0 && len(s.Prohibits) == 0
}

func (s SystemPrompt) Render() string {
	var parts []string
	if v := strings.TrimSpace(s.Role); v != "" {
		parts = append(parts, v)
	}
	if v := strings.TrimSpace(s.Identity); v != "" {
		parts = append(parts, v)
	}
	if len(s.Policies) > 0 {
		parts = append(parts, formatBulletSection("行为政策：", s.Policies))
	}
	if len(s.OutputRules) > 0 {
		parts = append(parts, formatBulletSection("输出规则：", s.OutputRules))
	}
	if len(s.Prohibits) > 0 {
		parts = append(parts, formatBulletSection("禁止行为：", s.Prohibits))
	}
	return strings.Join(parts, "\n\n")
}

func formatBulletSection(title string, items []string) string {
	var cleaned []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" {
			cleaned = append(cleaned, "- "+it)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	return strings.TrimSpace(title) + "\n" + strings.Join(cleaned, "\n")
}

// templateVarRegexp 匹配模板变量 {{variable}} 或 {{ variable }}
var templateVarRegexp = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.-]*)\s*\}\}`)

// RenderSystemPromptWithVars 渲染系统提示词并替换模板变量
func (b PromptBundle) RenderSystemPromptWithVars(vars map[string]string) string {
	prompt := b.RenderSystemPrompt()
	if len(vars) == 0 {
		return prompt
	}
	return replaceTemplateVars(prompt, vars)
}

// RenderWithVars 渲染完整提示词包并替换变量（包括 Examples 中的变量）
func (b PromptBundle) RenderWithVars(vars map[string]string) PromptBundle {
	if len(vars) == 0 {
		return b
	}

	result := PromptBundle{
		Version: b.Version,
		System: SystemPrompt{
			Role:        replaceTemplateVars(b.System.Role, vars),
			Identity:    replaceTemplateVars(b.System.Identity, vars),
			Policies:    replaceTemplateVarsSlice(b.System.Policies, vars),
			OutputRules: replaceTemplateVarsSlice(b.System.OutputRules, vars),
			Prohibits:   replaceTemplateVarsSlice(b.System.Prohibits, vars),
		},
		Tools:       b.Tools,
		Memory:      b.Memory,
		Plan:        b.Plan,
		Reflection:  b.Reflection,
		Constraints: replaceTemplateVarsSlice(b.Constraints, vars),
	}

	// 替换 Examples 中的变量
	if len(b.Examples) > 0 {
		result.Examples = make([]Example, len(b.Examples))
		for i, ex := range b.Examples {
			result.Examples[i] = Example{
				User:      replaceTemplateVars(ex.User, vars),
				Assistant: replaceTemplateVars(ex.Assistant, vars),
			}
		}
	}

	return result
}

// ExtractVariables 从 PromptBundle 中提取所有模板变量名
func (b PromptBundle) ExtractVariables() []string {
	var allText strings.Builder
	allText.WriteString(b.System.Role)
	allText.WriteString(b.System.Identity)
	for _, p := range b.System.Policies {
		allText.WriteString(p)
	}
	for _, r := range b.System.OutputRules {
		allText.WriteString(r)
	}
	for _, p := range b.System.Prohibits {
		allText.WriteString(p)
	}
	for _, c := range b.Constraints {
		allText.WriteString(c)
	}
	for _, ex := range b.Examples {
		allText.WriteString(ex.User)
		allText.WriteString(ex.Assistant)
	}

	return extractTemplateVars(allText.String())
}

// replaceTemplateVars 替换字符串中的模板变量
func replaceTemplateVars(text string, vars map[string]string) string {
	if text == "" || len(vars) == 0 {
		return text
	}
	return templateVarRegexp.ReplaceAllStringFunc(text, func(match string) string {
		// 提取变量名
		submatch := templateVarRegexp.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		varName := submatch[1]
		if val, ok := vars[varName]; ok {
			return val
		}
		return match // 未找到变量值，保留原样
	})
}

// replaceTemplateVarsSlice 替换切片中每个字符串的模板变量
func replaceTemplateVarsSlice(items []string, vars map[string]string) []string {
	if len(items) == 0 {
		return items
	}
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = replaceTemplateVars(item, vars)
	}
	return result
}

// extractTemplateVars 从文本中提取所有模板变量名（去重排序）
func extractTemplateVars(text string) []string {
	matches := templateVarRegexp.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	var vars []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		v := strings.TrimSpace(m[1])
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		vars = append(vars, v)
	}
	return vars
}

// RenderExamplesAsMessages 将 Examples 渲染为 LLM Message 格式
func (b PromptBundle) RenderExamplesAsMessages() []llm.Message {
	if len(b.Examples) == 0 {
		return nil
	}
	messages := make([]llm.Message, 0, len(b.Examples)*2)
	for _, ex := range b.Examples {
		if user := strings.TrimSpace(ex.User); user != "" {
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: user,
			})
		}
		if assistant := strings.TrimSpace(ex.Assistant); assistant != "" {
			messages = append(messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: assistant,
			})
		}
	}
	return messages
}

// RenderExamplesAsMessagesWithVars 渲染 Examples 并替换变量
func (b PromptBundle) RenderExamplesAsMessagesWithVars(vars map[string]string) []llm.Message {
	if len(b.Examples) == 0 {
		return nil
	}
	messages := make([]llm.Message, 0, len(b.Examples)*2)
	for _, ex := range b.Examples {
		user := strings.TrimSpace(ex.User)
		assistant := strings.TrimSpace(ex.Assistant)
		if len(vars) > 0 {
			user = replaceTemplateVars(user, vars)
			assistant = replaceTemplateVars(assistant, vars)
		}
		if user != "" {
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: user,
			})
		}
		if assistant != "" {
			messages = append(messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: assistant,
			})
		}
	}
	return messages
}

// HasExamples 检查是否有 Few-shot Examples
func (b PromptBundle) HasExamples() bool {
	return len(b.Examples) > 0
}

// AppendExamples 追加 Examples
func (b *PromptBundle) AppendExamples(examples ...Example) {
	b.Examples = append(b.Examples, examples...)
}
