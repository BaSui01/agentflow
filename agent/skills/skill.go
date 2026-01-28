package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
)

// Skill 代表一个可加载的 Agent 技能
// 基于 Anthropic Agent Skills 标准设计
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Category    string   `json:"category,omitempty"` // 技能分类
	Tags        []string `json:"tags,omitempty"`     // 标签

	// 核心内容
	Instructions string                 `json:"instructions"` // 技能指令
	Tools        []string               `json:"tools"`        // 需要的工具列表
	Resources    map[string]interface{} `json:"resources"`    // 资源（文件、数据等）
	Examples     []SkillExample         `json:"examples"`     // 使用示例

	// 加载策略
	LazyLoad     bool     `json:"lazy_load"`    // 是否延迟加载
	Priority     int      `json:"priority"`     // 优先级（用于冲突解决）
	Dependencies []string `json:"dependencies"` // 依赖的其他技能

	// 元数据
	Author    string    `json:"author,omitempty"`
	License   string    `json:"license,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 运行时状态
	Loaded   bool      `json:"-"` // 是否已加载
	LoadedAt time.Time `json:"-"` // 加载时间
}

// SkillExample 技能使用示例
type SkillExample struct {
	Input       string `json:"input"`
	Output      string `json:"output"`
	Explanation string `json:"explanation,omitempty"`
}

// SkillMetadata 技能元数据（用于发现和索引）
type SkillMetadata struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Version     string   `json:"version"`
	Path        string   `json:"path"` // 技能文件路径
}

// SkillManifest 技能清单文件（SKILL.json）
type SkillManifest struct {
	Skill
	Files []string `json:"files,omitempty"` // 关联的文件列表
}

// LoadSkillFromDirectory 从目录加载技能
func LoadSkillFromDirectory(dir string) (*Skill, error) {
	manifestPath := filepath.Join(dir, "SKILL.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill manifest: %w", err)
	}

	var manifest SkillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse skill manifest: %w", err)
	}

	skill := &manifest.Skill

	// 加载关联资源
	if err := loadSkillResources(dir, skill, manifest.Files); err != nil {
		return nil, fmt.Errorf("failed to load skill resources: %w", err)
	}

	skill.Loaded = true
	skill.LoadedAt = time.Now()

	return skill, nil
}

// loadSkillResources 加载技能资源文件
func loadSkillResources(dir string, skill *Skill, files []string) error {
	if skill.Resources == nil {
		skill.Resources = make(map[string]interface{})
	}

	for _, file := range files {
		filePath := filepath.Join(dir, file)

		// 检查文件是否存在
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue
		}

		// 读取文件内容
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read resource file %s: %w", file, err)
		}

		// 根据文件类型处理
		ext := filepath.Ext(file)
		switch ext {
		case ".json":
			var data interface{}
			if err := json.Unmarshal(content, &data); err != nil {
				skill.Resources[file] = string(content) // 解析失败，存储为字符串
			} else {
				skill.Resources[file] = data
			}
		case ".txt", ".md":
			skill.Resources[file] = string(content)
		default:
			skill.Resources[file] = content // 二进制数据
		}
	}

	return nil
}

// SaveSkillToDirectory 保存技能到目录
func SaveSkillToDirectory(skill *Skill, dir string) error {
	// 创建目录
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// 准备清单
	manifest := SkillManifest{
		Skill: *skill,
		Files: []string{},
	}

	// 保存资源文件
	for filename, content := range skill.Resources {
		filePath := filepath.Join(dir, filename)
		manifest.Files = append(manifest.Files, filename)

		var data []byte
		var err error

		switch v := content.(type) {
		case string:
			data = []byte(v)
		case []byte:
			data = v
		default:
			data, err = json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal resource %s: %w", filename, err)
			}
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write resource file %s: %w", filename, err)
		}
	}

	// 保存清单文件
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal skill manifest: %w", err)
	}

	manifestPath := filepath.Join(dir, "SKILL.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("failed to write skill manifest: %w", err)
	}

	return nil
}

// ToToolSchema 将技能转换为工具 Schema（如果技能可以作为工具使用）
func (s *Skill) ToToolSchema() llm.ToolSchema {
	// 构建参数 schema
	parametersMap := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type":        "string",
				"description": "Input for the skill",
			},
		},
		"required": []string{"input"},
	}

	// 转换为 json.RawMessage
	parametersJSON, _ := json.Marshal(parametersMap)

	return llm.ToolSchema{
		Name:        s.ID,
		Description: s.Description,
		Parameters:  parametersJSON,
	}
}

// RenderInstructions 渲染技能指令（支持变量替换）
func (s *Skill) RenderInstructions(vars map[string]string) string {
	instructions := s.Instructions

	if vars == nil {
		return instructions
	}

	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		instructions = strings.ReplaceAll(instructions, placeholder, value)
	}

	return instructions
}

// GetInstructions returns the skill instructions for prompt injection/augmentation.
func (s *Skill) GetInstructions() string {
	if s == nil {
		return ""
	}
	return s.Instructions
}

// GetResourceAsString 获取资源作为字符串
func (s *Skill) GetResourceAsString(name string) (string, error) {
	resource, ok := s.Resources[name]
	if !ok {
		return "", fmt.Errorf("resource %s not found", name)
	}

	switch v := resource.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("failed to marshal resource: %w", err)
		}
		return string(data), nil
	}
}

// GetResourceAsJSON 获取资源作为 JSON
func (s *Skill) GetResourceAsJSON(name string, target interface{}) error {
	resource, ok := s.Resources[name]
	if !ok {
		return fmt.Errorf("resource %s not found", name)
	}

	var data []byte
	var err error

	switch v := resource.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal resource: %w", err)
		}
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	return nil
}

// MatchesTask 检查技能是否匹配任务
func (s *Skill) MatchesTask(task string) float64 {
	task = strings.ToLower(task)
	score := 0.0

	// 检查名称匹配
	if strings.Contains(task, strings.ToLower(s.Name)) {
		score += 0.3
	}

	// 检查描述匹配
	descWords := strings.Fields(strings.ToLower(s.Description))
	taskWords := strings.Fields(task)

	matchCount := 0
	for _, tw := range taskWords {
		for _, dw := range descWords {
			if tw == dw || strings.Contains(dw, tw) || strings.Contains(tw, dw) {
				matchCount++
				break
			}
		}
	}

	if len(taskWords) > 0 {
		score += 0.4 * float64(matchCount) / float64(len(taskWords))
	}

	// 检查标签匹配
	for _, tag := range s.Tags {
		if strings.Contains(task, strings.ToLower(tag)) {
			score += 0.1
		}
	}

	// 检查分类匹配
	if s.Category != "" && strings.Contains(task, strings.ToLower(s.Category)) {
		score += 0.2
	}

	return score
}

// Clone 克隆技能（用于隔离修改）
func (s *Skill) Clone() *Skill {
	clone := *s

	// 深拷贝切片和 map
	clone.Tags = append([]string{}, s.Tags...)
	clone.Tools = append([]string{}, s.Tools...)
	clone.Dependencies = append([]string{}, s.Dependencies...)
	clone.Examples = append([]SkillExample{}, s.Examples...)

	clone.Resources = make(map[string]interface{})
	for k, v := range s.Resources {
		clone.Resources[k] = v
	}

	return &clone
}

// Validate 验证技能配置
func (s *Skill) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("skill ID is required")
	}
	if s.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if s.Instructions == "" {
		return fmt.Errorf("skill instructions are required")
	}
	if s.Version == "" {
		s.Version = "1.0.0"
	}
	return nil
}

// SkillBuilder 技能构建器
type SkillBuilder struct {
	skill Skill
}

// NewSkillBuilder 创建技能构建器
func NewSkillBuilder(id, name string) *SkillBuilder {
	return &SkillBuilder{
		skill: Skill{
			ID:        id,
			Name:      name,
			Version:   "1.0.0",
			Resources: make(map[string]interface{}),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
}

// WithDescription 设置描述
func (b *SkillBuilder) WithDescription(desc string) *SkillBuilder {
	b.skill.Description = desc
	return b
}

// WithInstructions 设置指令
func (b *SkillBuilder) WithInstructions(instructions string) *SkillBuilder {
	b.skill.Instructions = instructions
	return b
}

// WithCategory 设置分类
func (b *SkillBuilder) WithCategory(category string) *SkillBuilder {
	b.skill.Category = category
	return b
}

// WithTags 设置标签
func (b *SkillBuilder) WithTags(tags ...string) *SkillBuilder {
	b.skill.Tags = append(b.skill.Tags, tags...)
	return b
}

// WithTools 设置工具
func (b *SkillBuilder) WithTools(tools ...string) *SkillBuilder {
	b.skill.Tools = append(b.skill.Tools, tools...)
	return b
}

// WithResource 添加资源
func (b *SkillBuilder) WithResource(name string, content interface{}) *SkillBuilder {
	b.skill.Resources[name] = content
	return b
}

// WithExample 添加示例
func (b *SkillBuilder) WithExample(input, output, explanation string) *SkillBuilder {
	b.skill.Examples = append(b.skill.Examples, SkillExample{
		Input:       input,
		Output:      output,
		Explanation: explanation,
	})
	return b
}

// WithLazyLoad 设置延迟加载
func (b *SkillBuilder) WithLazyLoad(lazy bool) *SkillBuilder {
	b.skill.LazyLoad = lazy
	return b
}

// WithPriority 设置优先级
func (b *SkillBuilder) WithPriority(priority int) *SkillBuilder {
	b.skill.Priority = priority
	return b
}

// WithDependencies 设置依赖
func (b *SkillBuilder) WithDependencies(deps ...string) *SkillBuilder {
	b.skill.Dependencies = append(b.skill.Dependencies, deps...)
	return b
}

// Build 构建技能
func (b *SkillBuilder) Build() (*Skill, error) {
	if err := b.skill.Validate(); err != nil {
		return nil, err
	}
	return &b.skill, nil
}
