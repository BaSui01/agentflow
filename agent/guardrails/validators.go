package guardrails

import (
	"context"
	"fmt"
	"strings"
)

// LengthAction 长度超限处理动作
type LengthAction string

const (
	// LengthActionTruncate 截断处理
	LengthActionTruncate LengthAction = "truncate"
	// LengthActionReject 拒绝处理
	LengthActionReject LengthAction = "reject"
)

// LengthValidatorConfig 长度验证器配置
type LengthValidatorConfig struct {
	// MaxLength 最大长度（字符数）
	MaxLength int
	// Action 超限处理动作
	Action LengthAction
	// Priority 验证器优先级
	Priority int
}

// DefaultLengthValidatorConfig 返回默认配置
func DefaultLengthValidatorConfig() *LengthValidatorConfig {
	return &LengthValidatorConfig{
		MaxLength: 10000,
		Action:    LengthActionReject,
		Priority:  10, // 高优先级，最先执行
	}
}

// LengthValidator 长度验证器
// 实现 Validator 接口，用于验证输入长度
// Requirements 1.3: 当用户输入超过配置的最大长度时，截断或拒绝该输入
type LengthValidator struct {
	maxLength int
	action    LengthAction
	priority  int
}

// NewLengthValidator 创建长度验证器
func NewLengthValidator(config *LengthValidatorConfig) *LengthValidator {
	if config == nil {
		config = DefaultLengthValidatorConfig()
	}

	return &LengthValidator{
		maxLength: config.MaxLength,
		action:    config.Action,
		priority:  config.Priority,
	}
}

// Name 返回验证器名称
func (v *LengthValidator) Name() string {
	return "length_validator"
}

// Priority 返回优先级
func (v *LengthValidator) Priority() int {
	return v.priority
}

// Validate 执行长度验证
// 实现 Validator 接口
func (v *LengthValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()

	contentLen := len([]rune(content)) // 使用 rune 计算字符数，支持中文
	if contentLen <= v.maxLength {
		return result, nil
	}

	// 记录原始长度到 metadata
	result.Metadata["original_length"] = contentLen
	result.Metadata["max_length"] = v.maxLength
	result.Metadata["exceeded_by"] = contentLen - v.maxLength

	switch v.action {
	case LengthActionReject:
		// 拒绝模式：添加错误
		result.AddError(ValidationError{
			Code:     ErrCodeMaxLengthExceeded,
			Message:  fmt.Sprintf("输入长度 %d 超过最大限制 %d", contentLen, v.maxLength),
			Severity: SeverityHigh,
		})
	case LengthActionTruncate:
		// 截断模式：添加警告并在 metadata 中提供截断后的内容
		result.AddWarning(fmt.Sprintf("输入已从 %d 字符截断至 %d 字符", contentLen, v.maxLength))
		runes := []rune(content)
		result.Metadata["truncated_content"] = string(runes[:v.maxLength])
		result.Metadata["truncated"] = true
	}

	return result, nil
}

// GetMaxLength 返回配置的最大长度
func (v *LengthValidator) GetMaxLength() int {
	return v.maxLength
}

// GetAction 返回配置的处理动作
func (v *LengthValidator) GetAction() LengthAction {
	return v.action
}

// Truncate 截断内容至最大长度
func (v *LengthValidator) Truncate(content string) string {
	runes := []rune(content)
	if len(runes) <= v.maxLength {
		return content
	}
	return string(runes[:v.maxLength])
}

// KeywordSeverity 关键词严重级别配置
type KeywordSeverity struct {
	Keyword  string
	Severity string
}

// KeywordAction 关键词处理动作
type KeywordAction string

const (
	// KeywordActionReject 拒绝处理
	KeywordActionReject KeywordAction = "reject"
	// KeywordActionWarn 警告处理
	KeywordActionWarn KeywordAction = "warn"
	// KeywordActionFilter 过滤处理（替换关键词）
	KeywordActionFilter KeywordAction = "filter"
)

// KeywordValidatorConfig 关键词验证器配置
type KeywordValidatorConfig struct {
	// BlockedKeywords 禁止的关键词列表
	BlockedKeywords []string
	// KeywordSeverities 关键词严重级别映射（可选，未配置的使用默认级别）
	KeywordSeverities map[string]string
	// DefaultSeverity 默认严重级别
	DefaultSeverity string
	// Action 处理动作
	Action KeywordAction
	// CaseSensitive 是否区分大小写
	CaseSensitive bool
	// Replacement 过滤模式下的替换文本
	Replacement string
	// Priority 验证器优先级
	Priority int
}

// DefaultKeywordValidatorConfig 返回默认配置
func DefaultKeywordValidatorConfig() *KeywordValidatorConfig {
	return &KeywordValidatorConfig{
		BlockedKeywords:   []string{},
		KeywordSeverities: make(map[string]string),
		DefaultSeverity:   SeverityMedium,
		Action:            KeywordActionReject,
		CaseSensitive:     false,
		Replacement:       "[已过滤]",
		Priority:          20,
	}
}

// KeywordMatch 关键词匹配结果
type KeywordMatch struct {
	Keyword  string `json:"keyword"`
	Severity string `json:"severity"`
	Position int    `json:"position"`
	Length   int    `json:"length"`
}

// KeywordValidator 关键词验证器
// 实现 Validator 接口，用于检测禁止的关键词
// Requirements 1.4: 当用户输入包含禁止的关键词时，根据配置的严重级别进行处理
type KeywordValidator struct {
	blockedKeywords   []string
	keywordSeverities map[string]string
	defaultSeverity   string
	action            KeywordAction
	caseSensitive     bool
	replacement       string
	priority          int
}

// NewKeywordValidator 创建关键词验证器
func NewKeywordValidator(config *KeywordValidatorConfig) *KeywordValidator {
	if config == nil {
		config = DefaultKeywordValidatorConfig()
	}

	// 复制关键词列表
	keywords := make([]string, len(config.BlockedKeywords))
	copy(keywords, config.BlockedKeywords)

	// 复制严重级别映射
	severities := make(map[string]string)
	for k, v := range config.KeywordSeverities {
		severities[k] = v
	}

	return &KeywordValidator{
		blockedKeywords:   keywords,
		keywordSeverities: severities,
		defaultSeverity:   config.DefaultSeverity,
		action:            config.Action,
		caseSensitive:     config.CaseSensitive,
		replacement:       config.Replacement,
		priority:          config.Priority,
	}
}

// Name 返回验证器名称
func (v *KeywordValidator) Name() string {
	return "keyword_validator"
}

// Priority 返回优先级
func (v *KeywordValidator) Priority() int {
	return v.priority
}

// Validate 执行关键词验证
// 实现 Validator 接口
func (v *KeywordValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()

	// 检测所有关键词
	matches := v.Detect(content)
	if len(matches) == 0 {
		return result, nil
	}

	// 找出最高严重级别
	highestSeverity := SeverityLow
	for _, match := range matches {
		if compareSeverity(match.Severity, highestSeverity) > 0 {
			highestSeverity = match.Severity
		}
	}

	// 记录检测信息到 metadata
	result.Metadata["blocked_keywords_detected"] = true
	result.Metadata["keyword_matches"] = matches
	result.Metadata["keyword_count"] = len(matches)

	switch v.action {
	case KeywordActionReject:
		// 拒绝模式：添加错误
		result.AddError(ValidationError{
			Code:     ErrCodeBlockedKeyword,
			Message:  v.formatErrorMessage(matches),
			Severity: highestSeverity,
		})
	case KeywordActionWarn:
		// 警告模式：添加警告
		result.AddWarning(v.formatWarningMessage(matches))
	case KeywordActionFilter:
		// 过滤模式：添加警告并在 metadata 中提供过滤后的内容
		result.AddWarning(v.formatWarningMessage(matches))
		result.Metadata["filtered_content"] = v.Filter(content)
		result.Metadata["filtered"] = true
	}

	return result, nil
}

// Detect 检测内容中的所有禁止关键词
func (v *KeywordValidator) Detect(content string) []KeywordMatch {
	var matches []KeywordMatch

	searchContent := content
	if !v.caseSensitive {
		searchContent = strings.ToLower(content)
	}

	for _, keyword := range v.blockedKeywords {
		searchKeyword := keyword
		if !v.caseSensitive {
			searchKeyword = strings.ToLower(keyword)
		}

		// 查找所有匹配位置
		startPos := 0
		for {
			idx := strings.Index(searchContent[startPos:], searchKeyword)
			if idx == -1 {
				break
			}

			actualPos := startPos + idx
			matches = append(matches, KeywordMatch{
				Keyword:  keyword,
				Severity: v.getSeverity(keyword),
				Position: actualPos,
				Length:   len(keyword),
			})

			startPos = actualPos + len(searchKeyword)
		}
	}

	return matches
}

// Filter 过滤内容中的禁止关键词
func (v *KeywordValidator) Filter(content string) string {
	result := content

	for _, keyword := range v.blockedKeywords {
		if v.caseSensitive {
			result = strings.ReplaceAll(result, keyword, v.replacement)
		} else {
			// 不区分大小写的替换
			result = replaceAllCaseInsensitive(result, keyword, v.replacement)
		}
	}

	return result
}

// getSeverity 获取关键词的严重级别
func (v *KeywordValidator) getSeverity(keyword string) string {
	if severity, ok := v.keywordSeverities[keyword]; ok {
		return severity
	}
	// 不区分大小写查找
	if !v.caseSensitive {
		lowerKeyword := strings.ToLower(keyword)
		for k, severity := range v.keywordSeverities {
			if strings.ToLower(k) == lowerKeyword {
				return severity
			}
		}
	}
	return v.defaultSeverity
}

// formatErrorMessage 格式化关键词错误消息
func (v *KeywordValidator) formatErrorMessage(matches []KeywordMatch) string {
	if len(matches) == 0 {
		return "检测到禁止的关键词"
	}

	// 收集唯一的关键词
	keywords := make([]string, 0)
	seen := make(map[string]bool)
	for _, match := range matches {
		lowerKeyword := strings.ToLower(match.Keyword)
		if !seen[lowerKeyword] {
			seen[lowerKeyword] = true
			keywords = append(keywords, match.Keyword)
		}
	}

	if len(keywords) == 1 {
		return fmt.Sprintf("检测到禁止的关键词: %s", keywords[0])
	}

	return fmt.Sprintf("检测到 %d 个禁止的关键词: %s", len(keywords), strings.Join(keywords, ", "))
}

// formatWarningMessage 格式化关键词警告消息
func (v *KeywordValidator) formatWarningMessage(matches []KeywordMatch) string {
	if len(matches) == 0 {
		return "检测到禁止的关键词"
	}

	// 收集唯一的关键词
	keywords := make([]string, 0)
	seen := make(map[string]bool)
	for _, match := range matches {
		lowerKeyword := strings.ToLower(match.Keyword)
		if !seen[lowerKeyword] {
			seen[lowerKeyword] = true
			keywords = append(keywords, match.Keyword)
		}
	}

	return fmt.Sprintf("内容包含 %d 个禁止的关键词", len(keywords))
}

// AddKeyword 添加禁止关键词
func (v *KeywordValidator) AddKeyword(keyword string, severity string) {
	v.blockedKeywords = append(v.blockedKeywords, keyword)
	if severity != "" {
		v.keywordSeverities[keyword] = severity
	}
}

// RemoveKeyword 移除禁止关键词
func (v *KeywordValidator) RemoveKeyword(keyword string) {
	for i, k := range v.blockedKeywords {
		if k == keyword || (!v.caseSensitive && strings.EqualFold(k, keyword)) {
			v.blockedKeywords = append(v.blockedKeywords[:i], v.blockedKeywords[i+1:]...)
			delete(v.keywordSeverities, keyword)
			return
		}
	}
}

// GetBlockedKeywords 返回禁止关键词列表
func (v *KeywordValidator) GetBlockedKeywords() []string {
	result := make([]string, len(v.blockedKeywords))
	copy(result, v.blockedKeywords)
	return result
}

// GetAction 返回配置的处理动作
func (v *KeywordValidator) GetAction() KeywordAction {
	return v.action
}

// replaceAllCaseInsensitive 不区分大小写替换所有匹配
func replaceAllCaseInsensitive(content, old, new string) string {
	if old == "" {
		return content
	}

	var result strings.Builder
	lowerContent := strings.ToLower(content)
	lowerOld := strings.ToLower(old)

	start := 0
	for {
		idx := strings.Index(lowerContent[start:], lowerOld)
		if idx == -1 {
			result.WriteString(content[start:])
			break
		}

		result.WriteString(content[start : start+idx])
		result.WriteString(new)
		start = start + idx + len(old)
	}

	return result.String()
}
