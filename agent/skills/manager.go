package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.uber.org/zap"
)

// SkillManager 技能管理器
type SkillManager interface {
	// 技能发现
	DiscoverSkills(ctx context.Context, task string) ([]*Skill, error)

	// 技能加载
	LoadSkill(ctx context.Context, skillID string) (*Skill, error)
	UnloadSkill(ctx context.Context, skillID string) error

	// 技能查询
	GetSkill(skillID string) (*Skill, bool)
	ListSkills() []*SkillMetadata
	SearchSkills(query string) []*SkillMetadata

	// 技能管理
	RegisterSkill(skill *Skill) error
	UnregisterSkill(skillID string) error

	// 技能仓库
	ScanDirectory(dir string) error
	RefreshIndex() error
}

// DefaultSkillManager 默认技能管理器实现
type DefaultSkillManager struct {
	// 已加载的技能
	skills map[string]*Skill
	mu     sync.RWMutex

	// 内存注册的技能（无落盘路径）
	inMemory map[string]*Skill

	// 技能索引（用于快速查找）
	index map[string]*SkillMetadata

	// 技能目录
	directories []string

	// 配置
	config SkillManagerConfig

	logger *zap.Logger
}

// SkillManagerConfig 技能管理器配置
type SkillManagerConfig struct {
	AutoLoad        bool    `json:"auto_load"`         // 自动加载技能
	MaxLoadedSkills int     `json:"max_loaded_skills"` // 最大加载技能数
	MinMatchScore   float64 `json:"min_match_score"`   // 最低匹配分数
	EnableCaching   bool    `json:"enable_caching"`    // 启用缓存
	CacheTTL        int     `json:"cache_ttl"`         // 缓存 TTL（秒）
}

// DefaultSkillManagerConfig 默认配置
func DefaultSkillManagerConfig() SkillManagerConfig {
	return SkillManagerConfig{
		AutoLoad:        true,
		MaxLoadedSkills: 10,
		MinMatchScore:   0.3,
		EnableCaching:   true,
		CacheTTL:        3600,
	}
}

// NewSkillManager 创建技能管理器
func NewSkillManager(config SkillManagerConfig, logger *zap.Logger) *DefaultSkillManager {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	return &DefaultSkillManager{
		skills:      make(map[string]*Skill),
		inMemory:    make(map[string]*Skill),
		index:       make(map[string]*SkillMetadata),
		directories: []string{},
		config:      config,
		logger:      logger.With(zap.String("component", "skill_manager")),
	}
}

// DiscoverSkills 发现适合任务的技能
func (m *DefaultSkillManager) DiscoverSkills(ctx context.Context, task string) ([]*Skill, error) {
	m.logger.Debug("discovering skills for task", zap.String("task", task))

	// 1. 搜索匹配的技能
	metadata := m.SearchSkills(task)

	if len(metadata) == 0 {
		m.logger.Info("no matching skills found", zap.String("task", task))
		return []*Skill{}, nil
	}

	// 2. 加载技能并评分
	type scoredSkill struct {
		skill *Skill
		score float64
	}

	scored := []scoredSkill{}

	for _, meta := range metadata {
		skill, err := m.LoadSkill(ctx, meta.ID)
		if err != nil {
			m.logger.Warn("failed to load skill",
				zap.String("skill_id", meta.ID),
				zap.Error(err),
			)
			continue
		}

		score := skill.MatchesTask(task)
		if score >= m.config.MinMatchScore {
			scored = append(scored, scoredSkill{skill: skill, score: score})
		}
	}

	// 3. 按分数排序
	sort.Slice(scored, func(i, j int) bool {
		// 优先级高的排前面
		if scored[i].skill.Priority != scored[j].skill.Priority {
			return scored[i].skill.Priority > scored[j].skill.Priority
		}
		// 分数高的排前面
		return scored[i].score > scored[j].score
	})

	// 4. 返回 Top-K 技能
	maxSkills := m.config.MaxLoadedSkills
	if maxSkills <= 0 || maxSkills > len(scored) {
		maxSkills = len(scored)
	}

	result := make([]*Skill, maxSkills)
	for i := 0; i < maxSkills; i++ {
		result[i] = scored[i].skill
	}

	m.logger.Info("discovered skills",
		zap.String("task", task),
		zap.Int("found", len(result)),
	)

	return result, nil
}

// LoadSkill 加载技能
func (m *DefaultSkillManager) LoadSkill(ctx context.Context, skillID string) (*Skill, error) {
	return m.loadSkill(ctx, skillID, map[string]struct{}{})
}

func (m *DefaultSkillManager) loadSkill(ctx context.Context, skillID string, loading map[string]struct{}) (*Skill, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 检查是否已加载
	m.mu.RLock()
	if skill, ok := m.skills[skillID]; ok {
		m.mu.RUnlock()
		m.logger.Debug("skill already loaded", zap.String("skill_id", skillID))
		return skill, nil
	}
	meta, metaOK := m.index[skillID]
	inMemory := m.inMemory[skillID]
	m.mu.RUnlock()

	// 从索引获取元数据
	if !metaOK {
		return nil, fmt.Errorf("skill %s not found in index", skillID)
	}

	if _, exists := loading[skillID]; exists {
		return nil, fmt.Errorf("cyclic skill dependency detected: %s", skillID)
	}
	loading[skillID] = struct{}{}
	defer delete(loading, skillID)

	// 加载技能
	m.logger.Info("loading skill",
		zap.String("skill_id", skillID),
		zap.String("path", meta.Path),
	)

	var (
		skill *Skill
		err   error
	)

	if inMemory != nil {
		skill = inMemory.Clone()
		skill.Loaded = true
		skill.LoadedAt = time.Now()
	} else {
		if strings.TrimSpace(meta.Path) == "" {
			return nil, fmt.Errorf("skill %s has no storage path", skillID)
		}

		skill, err = LoadSkillFromDirectory(meta.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to load skill: %w", err)
		}
	}

	// 加载依赖
	if len(skill.Dependencies) > 0 {
		m.logger.Debug("loading skill dependencies",
			zap.String("skill_id", skillID),
			zap.Strings("dependencies", skill.Dependencies),
		)

		for _, depID := range skill.Dependencies {
			if _, err := m.loadSkill(ctx, depID, loading); err != nil {
				m.logger.Warn("failed to load dependency",
					zap.String("skill_id", skillID),
					zap.String("dependency", depID),
					zap.Error(err),
				)
			}
		}
	}

	// 存储到已加载技能
	m.mu.Lock()
	m.skills[skillID] = skill
	m.mu.Unlock()

	m.logger.Info("skill loaded successfully", zap.String("skill_id", skillID))

	return skill, nil
}

// UnloadSkill 卸载技能
func (m *DefaultSkillManager) UnloadSkill(ctx context.Context, skillID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.skills[skillID]; !ok {
		return fmt.Errorf("skill %s not loaded", skillID)
	}

	delete(m.skills, skillID)

	m.logger.Info("skill unloaded", zap.String("skill_id", skillID))

	return nil
}

// GetSkill 获取已加载的技能
func (m *DefaultSkillManager) GetSkill(skillID string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	skill, ok := m.skills[skillID]
	return skill, ok
}

// ListSkills 列出所有技能
func (m *DefaultSkillManager) ListSkills() []*SkillMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SkillMetadata, 0, len(m.index))
	for _, meta := range m.index {
		result = append(result, meta)
	}

	// 按名称排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// SearchSkills 搜索技能
func (m *DefaultSkillManager) SearchSkills(query string) []*SkillMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query = strings.ToLower(query)
	tokens := tokenizeQuery(query)

	type scoredMetadata struct {
		meta  *SkillMetadata
		score float64
	}
 
	scored := make([]scoredMetadata, 0, len(m.index))

	for _, meta := range m.index {
		score := scoreMetadataMatch(meta, query, tokens)
		if score > 0 {
			scored = append(scored, scoredMetadata{meta: meta, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].meta.Name < scored[j].meta.Name
	})

	result := make([]*SkillMetadata, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.meta)
	}

	return result
}

// RegisterSkill 注册技能
func (m *DefaultSkillManager) RegisterSkill(skill *Skill) error {
	if err := skill.Validate(); err != nil {
		return fmt.Errorf("invalid skill: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.inMemory[skill.ID] = skill.Clone()

	// 添加到索引
	m.index[skill.ID] = &SkillMetadata{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Category:    skill.Category,
		Tags:        skill.Tags,
		Version:     skill.Version,
		Path:        "", // 内存中的技能没有路径
	}

	// 如果配置了自动加载，直接加载
	if m.config.AutoLoad {
		loaded := skill.Clone()
		loaded.Loaded = true
		loaded.LoadedAt = time.Now()
		m.skills[skill.ID] = loaded
	}

	m.logger.Info("skill registered", zap.String("skill_id", skill.ID))

	return nil
}

// UnregisterSkill 注销技能
func (m *DefaultSkillManager) UnregisterSkill(skillID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.index, skillID)
	delete(m.skills, skillID)
	delete(m.inMemory, skillID)

	m.logger.Info("skill unregistered", zap.String("skill_id", skillID))

	return nil
}

// ScanDirectory 扫描目录查找技能
func (m *DefaultSkillManager) ScanDirectory(dir string) error {
	m.logger.Info("scanning directory for skills", zap.String("dir", dir))
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" {
		return fmt.Errorf("directory is empty")
	}

	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", dir)
	}

	// 添加到目录列表
	m.mu.Lock()
	exists := false
	for _, scannedDir := range m.directories {
		if scannedDir == dir {
			exists = true
			break
		}
	}
	if !exists {
		m.directories = append(m.directories, dir)
	}
	m.mu.Unlock()

	// 遍历目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		manifestPath := filepath.Join(skillDir, "SKILL.json")

		// 检查是否有 SKILL.json
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		// 加载技能元数据
		skill, err := LoadSkillFromDirectory(skillDir)
		if err != nil {
			m.logger.Warn("failed to load skill",
				zap.String("dir", skillDir),
				zap.Error(err),
			)
			continue
		}

		// 添加到索引
		m.mu.Lock()
		m.index[skill.ID] = &SkillMetadata{
			ID:          skill.ID,
			Name:        skill.Name,
			Description: skill.Description,
			Category:    skill.Category,
			Tags:        skill.Tags,
			Version:     skill.Version,
			Path:        skillDir,
		}
		m.mu.Unlock()

		count++
	}

	m.logger.Info("directory scan completed",
		zap.String("dir", dir),
		zap.Int("skills_found", count),
	)

	return nil
}

// RefreshIndex 刷新技能索引
func (m *DefaultSkillManager) RefreshIndex() error {
	m.logger.Info("refreshing skill index")

	m.mu.RLock()
	directories := append([]string(nil), m.directories...)
	inMemory := make(map[string]*Skill, len(m.inMemory))
	for id, skill := range m.inMemory {
		inMemory[id] = skill.Clone()
	}
	m.mu.RUnlock()

	// 清空索引
	m.mu.Lock()
	m.index = make(map[string]*SkillMetadata)
	m.mu.Unlock()

	// 先恢复内存技能的索引
	for _, skill := range inMemory {
		if err := m.RegisterSkill(skill); err != nil {
			m.logger.Warn("failed to restore in-memory skill",
				zap.String("skill_id", skill.ID),
				zap.Error(err),
			)
		}
	}

	// 重新扫描所有目录
	for _, dir := range directories {
		if err := m.ScanDirectory(dir); err != nil {
			m.logger.Warn("failed to scan directory",
				zap.String("dir", dir),
				zap.Error(err),
			)
		}
	}

	m.logger.Info("skill index refreshed",
		zap.Int("total_skills", len(m.index)),
	)

	return nil
}

// GetLoadedSkillsCount 获取已加载技能数量
func (m *DefaultSkillManager) GetLoadedSkillsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.skills)
}

// GetIndexedSkillsCount 获取索引中的技能数量
func (m *DefaultSkillManager) GetIndexedSkillsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.index)
}

// ClearCache 清除缓存（卸载所有技能）
func (m *DefaultSkillManager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.skills = make(map[string]*Skill)

	m.logger.Info("skill cache cleared")
}

func tokenizeQuery(query string) []string {
	if query == "" {
		return nil
	}
	tokens := strings.FieldsFunc(query, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	})
	if len(tokens) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if _, exists := unique[token]; exists {
			continue
		}
		unique[token] = struct{}{}
		result = append(result, token)
	}

	return result
}

func scoreMetadataMatch(meta *SkillMetadata, query string, tokens []string) float64 {
	if meta == nil {
		return 0
	}
	if query == "" {
		return 1
	}

	name := strings.ToLower(meta.Name)
	description := strings.ToLower(meta.Description)
	category := strings.ToLower(meta.Category)

	score := 0.0
	if strings.Contains(name, query) {
		score += 0.45
	}
	if strings.Contains(description, query) {
		score += 0.25
	}
	if strings.Contains(category, query) {
		score += 0.15
	}

	tagMatched := false
	for _, tag := range meta.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			tagMatched = true
			break
		}
	}
	if tagMatched {
		score += 0.15
	}

	if len(tokens) > 0 {
		matched := 0
		for _, token := range tokens {
			if strings.Contains(name, token) || strings.Contains(description, token) || strings.Contains(category, token) {
				matched++
				continue
			}
			for _, tag := range meta.Tags {
				if strings.Contains(strings.ToLower(tag), token) {
					matched++
					break
				}
			}
		}
		score += 0.4 * float64(matched) / float64(len(tokens))
	}

	if score > 1 {
		return 1
	}
	return score
}
