package agent

import (
	"fmt"
	"strings"
)

// PromptEngineeringConfig 提示词工程配置
type PromptEngineeringConfig struct {
	UseChainOfThought   bool `json:"use_chain_of_thought"`   // 使用思维链 (CoT)
	UseSelfConsistency  bool `json:"use_self_consistency"`   // 使用自我一致性
	UseStructuredOutput bool `json:"use_structured_output"`  // 使用结构化输出
	UseFewShot          bool `json:"use_few_shot"`           // 使用 Few-shot
	MaxExamples         int  `json:"max_examples,omitempty"` // 最多示例数
	UseDelimiters       bool `json:"use_delimiters"`         // 使用分隔符
}

// DefaultPromptEngineeringConfig 默认配置
func DefaultPromptEngineeringConfig() PromptEngineeringConfig {
	return PromptEngineeringConfig{
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
	config PromptEngineeringConfig
}

// NewPromptEnhancer 创建提示词增强器
func NewPromptEnhancer(config PromptEngineeringConfig) *PromptEnhancer {
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
	template, ok := l.templates[name]
	return template, ok
}

// RenderTemplate 渲染模板
func (l *PromptTemplateLibrary) RenderTemplate(name string, vars map[string]string) (string, error) {
	template, ok := l.templates[name]
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
	l.templates[template.Name] = template
}

// ListTemplates 列出所有模板
func (l *PromptTemplateLibrary) ListTemplates() []string {
	names := make([]string, 0, len(l.templates))
	for name := range l.templates {
		names = append(names, name)
	}
	return names
}
