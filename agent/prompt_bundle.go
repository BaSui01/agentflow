package agent

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// PromptBundle 模块化提示词包（按版本管理）。
//
// 说明：当前版本主要承载 System 模块，其他模块作为扩展点保留。
type PromptBundle struct {
	Version     string             `json:"version"`
	System      SystemPrompt       `json:"system"`
	Tools       []types.ToolSchema `json:"tools,omitempty"`
	Examples    []Example          `json:"examples,omitempty"`
	Memory      MemoryConfig       `json:"memory,omitempty"`
	Plan        *PlanConfig        `json:"plan,omitempty"`
	Reflection  *ReflectionConfig  `json:"reflection,omitempty"`
	Constraints []string           `json:"constraints,omitempty"`
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
func (b PromptBundle) RenderExamplesAsMessages() []types.Message {
	if len(b.Examples) == 0 {
		return nil
	}
	messages := make([]types.Message, 0, len(b.Examples)*2)
	for _, ex := range b.Examples {
		if user := strings.TrimSpace(ex.User); user != "" {
			messages = append(messages, types.Message{
				Role:    llm.RoleUser,
				Content: user,
			})
		}
		if assistant := strings.TrimSpace(ex.Assistant); assistant != "" {
			messages = append(messages, types.Message{
				Role:    llm.RoleAssistant,
				Content: assistant,
			})
		}
	}
	return messages
}

// RenderExamplesAsMessagesWithVars 渲染 Examples 并替换变量
func (b PromptBundle) RenderExamplesAsMessagesWithVars(vars map[string]string) []types.Message {
	if len(b.Examples) == 0 {
		return nil
	}
	messages := make([]types.Message, 0, len(b.Examples)*2)
	for _, ex := range b.Examples {
		user := strings.TrimSpace(ex.User)
		assistant := strings.TrimSpace(ex.Assistant)
		if len(vars) > 0 {
			user = replaceTemplateVars(user, vars)
			assistant = replaceTemplateVars(assistant, vars)
		}
		if user != "" {
			messages = append(messages, types.Message{
				Role:    llm.RoleUser,
				Content: user,
			})
		}
		if assistant != "" {
			messages = append(messages, types.Message{
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

// =============================================================================
// Prompt Engineering (merged from prompt_engineering.go)
// =============================================================================
// PromptEnhancerConfig 提示词增强配置
type PromptEnhancerConfig struct {
	UseChainOfThought   bool `json:"use_chain_of_thought"`   // Use Chain of Thought (CoT)
	UseSelfConsistency  bool `json:"use_self_consistency"`   // Use self-consistency
	UseStructuredOutput bool `json:"use_structured_output"`  // Use structured output
	UseFewShot          bool `json:"use_few_shot"`           // Use few-shot learning
	MaxExamples         int  `json:"max_examples,omitempty"` // Maximum number of examples
	UseDelimiters       bool `json:"use_delimiters"`         // Use delimiters
}

// DefaultPromptEnhancerConfig 返回默认的提示词增强器配置
func DefaultPromptEnhancerConfig() *PromptEnhancerConfig {
	return &PromptEnhancerConfig{
		UseChainOfThought:   true,
		UseSelfConsistency:  false,
		UseStructuredOutput: true,
		UseFewShot:          true,
		MaxExamples:         3,
		UseDelimiters:       true,
	}
}

// PromptEnhancer 提示词增强器
type PromptEnhancer struct {
	config PromptEnhancerConfig
}

// NewPromptEnhancer 创建提示词增强器
func NewPromptEnhancer(config PromptEnhancerConfig) *PromptEnhancer {
	return &PromptEnhancer{config: config}
}

// EnhancePromptBundle 增强提示词包
func (e *PromptEnhancer) EnhancePromptBundle(bundle PromptBundle) PromptBundle {
	enhanced := bundle

	// 1. 添加思维链提示
	if e.config.UseChainOfThought {
		enhanced = e.addChainOfThought(enhanced)
	}

	// 2. 添加结构化输出规则
	if e.config.UseStructuredOutput {
		enhanced = e.addStructuredOutputRules(enhanced)
	}

	// 3. 限制示例数量
	if e.config.UseFewShot && e.config.MaxExamples > 0 && len(enhanced.Examples) > e.config.MaxExamples {
		enhanced.Examples = enhanced.Examples[:e.config.MaxExamples]
	}

	// 4. 添加分隔符
	if e.config.UseDelimiters {
		enhanced = e.addDelimiters(enhanced)
	}

	return enhanced
}

// addChainOfThought 添加思维链提示
func (e *PromptEnhancer) addChainOfThought(bundle PromptBundle) PromptBundle {
	// 检查是否已包含思维链提示
	identity := strings.ToLower(bundle.System.Identity)
	if strings.Contains(identity, "step by step") ||
		strings.Contains(identity, "一步步") ||
		strings.Contains(identity, "逐步思考") {
		return bundle
	}

	// 添加思维链规则
	cotRule := "在回答问题时，请一步步思考，展示你的推理过程"
	bundle.System.OutputRules = append(bundle.System.OutputRules, cotRule)

	return bundle
}

// addStructuredOutputRules 添加结构化输出规则
func (e *PromptEnhancer) addStructuredOutputRules(bundle PromptBundle) PromptBundle {
	// 检查是否已有输出规则
	hasStructureRule := false
	for _, rule := range bundle.System.OutputRules {
		if strings.Contains(strings.ToLower(rule), "格式") ||
			strings.Contains(strings.ToLower(rule), "format") ||
			strings.Contains(strings.ToLower(rule), "structure") {
			hasStructureRule = true
			break
		}
	}

	if !hasStructureRule {
		structureRule := "输出应该清晰、结构化，使用适当的格式（如列表、段落、代码块等）"
		bundle.System.OutputRules = append(bundle.System.OutputRules, structureRule)
	}

	return bundle
}

// addDelimiters 添加分隔符说明
func (e *PromptEnhancer) addDelimiters(bundle PromptBundle) PromptBundle {
	// 在 Identity 中添加分隔符说明
	if bundle.System.Identity != "" && !strings.Contains(bundle.System.Identity, "```") {
		delimiterNote := "\n\n注意：用户输入可能使用 ``` 或 ### 等分隔符来标记不同部分。"
		bundle.System.Identity += delimiterNote
	}

	return bundle
}

// EnhanceUserPrompt 增强用户提示词
func (e *PromptEnhancer) EnhanceUserPrompt(prompt string, outputFormat string) string {
	enhanced := prompt

	// 1. 添加分隔符
	if e.config.UseDelimiters && !strings.Contains(prompt, "```") {
		enhanced = fmt.Sprintf("```\n%s\n```", enhanced)
	}

	// 2. 添加思维链提示
	if e.config.UseChainOfThought {
		if !strings.Contains(strings.ToLower(enhanced), "step by step") &&
			!strings.Contains(enhanced, "一步步") {
			enhanced += "\n\n请一步步思考并解释你的推理过程。"
		}
	}

	// 3. 添加输出格式要求
	if e.config.UseStructuredOutput && outputFormat != "" {
		enhanced += fmt.Sprintf("\n\n请按照以下格式输出：\n%s", outputFormat)
	}

	return enhanced
}

// PromptOptimizer 提示词优化器（基于最佳实践）
type PromptOptimizer struct{}

// NewPromptOptimizer 创建提示词优化器
func NewPromptOptimizer() *PromptOptimizer {
	return &PromptOptimizer{}
}

// OptimizePrompt 优化提示词
// 基于 2025 年最佳实践：
// 1. 明确具体
// 2. 提供示例
// 3. 让模型思考
// 4. 使用分隔符
// 5. 拆分复杂任务
func (o *PromptOptimizer) OptimizePrompt(prompt string) string {
	optimized := prompt

	// 1. 检查是否足够具体
	if len(prompt) < 20 {
		// 提示词太短，可能不够具体
		optimized = o.makeMoreSpecific(optimized)
	}

	// 2. 检查是否有清晰的任务描述
	if !o.hasTaskDescription(optimized) {
		optimized = o.addTaskDescription(optimized)
	}

	// 3. 检查是否有约束条件
	if !o.hasConstraints(optimized) {
		optimized = o.addBasicConstraints(optimized)
	}

	return optimized
}

// makeMoreSpecific 使提示词更具体
func (o *PromptOptimizer) makeMoreSpecific(prompt string) string {
	return fmt.Sprintf("任务：%s\n\n请提供详细的回答，包括必要的解释和示例。", prompt)
}

// hasTaskDescription 检查是否有任务描述
func (o *PromptOptimizer) hasTaskDescription(prompt string) bool {
	keywords := []string{"请", "帮我", "任务", "需要", "要求", "please", "task", "need"}
	lower := strings.ToLower(prompt)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// addTaskDescription 添加任务描述
func (o *PromptOptimizer) addTaskDescription(prompt string) string {
	return fmt.Sprintf("请完成以下任务：\n\n%s", prompt)
}

// hasConstraints 检查是否有约束条件
func (o *PromptOptimizer) hasConstraints(prompt string) bool {
	keywords := []string{"不要", "避免", "必须", "应该", "限制", "don't", "avoid", "must", "should", "limit"}
	lower := strings.ToLower(prompt)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// addBasicConstraints 添加基本约束
func (o *PromptOptimizer) addBasicConstraints(prompt string) string {
	return fmt.Sprintf("%s\n\n要求：\n- 回答要准确、完整\n- 使用清晰的语言\n- 提供必要的解释", prompt)
}

// PromptTemplateLibrary 提示词模板库
type PromptTemplateLibrary struct {
	mu        sync.RWMutex
	templates map[string]PromptTemplate
}

// PromptTemplate 提示词模板
type PromptTemplate struct {
	Name        string
	Description string
	Template    string
	Variables   []string
	Examples    []Example
}

// NewPromptTemplateLibrary 创建提示词模板库
func NewPromptTemplateLibrary() *PromptTemplateLibrary {
	lib := &PromptTemplateLibrary{
		templates: make(map[string]PromptTemplate),
	}

	// 预定义常用模板
	lib.registerDefaultTemplates()

	return lib
}

// registerDefaultTemplates 注册默认模板
func (l *PromptTemplateLibrary) registerDefaultTemplates() {
	// 1. 分析任务模板
	l.templates["analysis"] = PromptTemplate{
		Name:        "analysis",
		Description: "分析任务模板",
		Template: `请分析以下{{.subject}}：

{{.content}}

请从以下角度进行分析：
1. 主要特点
2. 优势和劣势
3. 潜在影响
4. 改进建议`,
		Variables: []string{"subject", "content"},
	}

	// 2. 总结任务模板
	l.templates["summary"] = PromptTemplate{
		Name:        "summary",
		Description: "总结任务模板",
		Template: `请总结以下内容：

{{.content}}

要求：
- 提取关键信息
- 保持简洁明了
- 不超过{{.max_words}}字`,
		Variables: []string{"content", "max_words"},
	}

	// 3. 代码生成模板
	l.templates["code_generation"] = PromptTemplate{
		Name:        "code_generation",
		Description: "代码生成模板",
		Template: `请用{{.language}}编写代码来实现以下功能：

{{.requirement}}

要求：
- 代码要清晰、可读
- 添加必要的注释
- 遵循最佳实践
- 包含错误处理`,
		Variables: []string{"language", "requirement"},
	}

	// 4. 问题解答模板
	l.templates["qa"] = PromptTemplate{
		Name:        "qa",
		Description: "问题解答模板",
		Template: `问题：{{.question}}

请提供详细的回答，包括：
1. 直接答案
2. 解释说明
3. 相关示例（如适用）
4. 注意事项`,
		Variables: []string{"question"},
	}

	// 5. 创意生成模板
	l.templates["creative"] = PromptTemplate{
		Name:        "creative",
		Description: "创意生成模板",
		Template: `请为以下主题生成创意想法：

主题：{{.topic}}
目标：{{.goal}}

要求：
- 提供至少{{.count}}个不同的想法
- 每个想法要新颖、可行
- 简要说明实施方式`,
		Variables: []string{"topic", "goal", "count"},
	}
}

// GetTemplate 获取模板
func (l *PromptTemplateLibrary) GetTemplate(name string) (PromptTemplate, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	template, ok := l.templates[name]
	return template, ok
}

// RenderTemplate 渲染模板
func (l *PromptTemplateLibrary) RenderTemplate(name string, vars map[string]string) (string, error) {
	l.mu.RLock()
	template, ok := l.templates[name]
	l.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("template %s not found", name)
	}

	result := template.Template
	for _, varName := range template.Variables {
		placeholder := "{{." + varName + "}}"
		value, ok := vars[varName]
		if !ok {
			return "", fmt.Errorf("variable %s not provided", varName)
		}
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

// RegisterTemplate 注册自定义模板
func (l *PromptTemplateLibrary) RegisterTemplate(template PromptTemplate) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.templates[template.Name] = template
}

// ListTemplates 列出所有模板
func (l *PromptTemplateLibrary) ListTemplates() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	names := make([]string, 0, len(l.templates))
	for name := range l.templates {
		names = append(names, name)
	}
	return names
}
