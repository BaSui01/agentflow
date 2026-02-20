package guardrails

import (
	"context"
	"regexp"
	"strings"
)

// InjectionPattern 注入模式类型
type InjectionPattern struct {
	Pattern     *regexp.Regexp
	Description string
	Severity    string
	Language    string // "en", "zh", "universal"
}

// InjectionDetectorConfig 注入检测器配置
type InjectionDetectorConfig struct {
	// CaseSensitive 是否区分大小写
	CaseSensitive bool
	// UseDelimiters 是否使用分隔符隔离
	UseDelimiters bool
	// CustomPatterns 自定义注入模式
	CustomPatterns []string
	// Priority 验证器优先级
	Priority int
	// EnabledLanguages 启用的语言检测，为空则启用所有
	EnabledLanguages []string
}

// DefaultInjectionDetectorConfig 返回默认配置
func DefaultInjectionDetectorConfig() *InjectionDetectorConfig {
	return &InjectionDetectorConfig{
		CaseSensitive:    false,
		UseDelimiters:    true,
		CustomPatterns:   nil,
		Priority:         50, // 高优先级，在 PII 检测之前
		EnabledLanguages: nil,
	}
}

// InjectionDetector 提示注入检测器
// 实现 Validator 接口，用于检测和防止提示注入攻击
type InjectionDetector struct {
	patterns      []*InjectionPattern
	caseSensitive bool
	useDelimiters bool
	priority      int
}

// NewInjectionDetector 创建注入检测器
func NewInjectionDetector(config *InjectionDetectorConfig) *InjectionDetector {
	if config == nil {
		config = DefaultInjectionDetectorConfig()
	}

	detector := &InjectionDetector{
		patterns:      make([]*InjectionPattern, 0),
		caseSensitive: config.CaseSensitive,
		useDelimiters: config.UseDelimiters,
		priority:      config.Priority,
	}

	// 加载默认注入模式
	defaultPatterns := getDefaultInjectionPatterns(config.CaseSensitive)

	// 过滤启用的语言
	enabledLangs := config.EnabledLanguages
	if len(enabledLangs) == 0 {
		enabledLangs = []string{"en", "zh", "universal"}
	}

	langSet := make(map[string]bool)
	for _, lang := range enabledLangs {
		langSet[lang] = true
	}

	for _, pattern := range defaultPatterns {
		if langSet[pattern.Language] {
			detector.patterns = append(detector.patterns, pattern)
		}
	}

	// 添加自定义模式
	for _, customPattern := range config.CustomPatterns {
		flags := ""
		if !config.CaseSensitive {
			flags = "(?i)"
		}
		if re, err := regexp.Compile(flags + customPattern); err == nil {
			detector.patterns = append(detector.patterns, &InjectionPattern{
				Pattern:     re,
				Description: "Custom injection pattern",
				Severity:    SeverityHigh,
				Language:    "custom",
			})
		}
	}

	return detector
}

// getDefaultInjectionPatterns 返回默认的注入检测模式
func getDefaultInjectionPatterns(caseSensitive bool) []*InjectionPattern {
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}

	patterns := []*InjectionPattern{
		// 英语模式 - 指令覆盖尝试
		{
			Pattern:     regexp.MustCompile(flags + `ignore\s+(all\s+)?(previous|prior|above|earlier)\s+(instructions?|prompts?|rules?|guidelines?)`),
			Description: "Attempt to ignore previous instructions",
			Severity:    SeverityCritical,
			Language:    "en",
		},
		{
			Pattern:     regexp.MustCompile(flags + `disregard\s+(all\s+)?(previous|prior|above|earlier|the\s+above)\s*(instructions?|prompts?|rules?|guidelines?)?`),
			Description: "Attempt to disregard instructions",
			Severity:    SeverityCritical,
			Language:    "en",
		},
		{
			Pattern:     regexp.MustCompile(flags + `forget\s+(everything|all|what)\s*(you\s+)?(know|learned|were\s+told)?`),
			Description: "Attempt to make model forget context",
			Severity:    SeverityCritical,
			Language:    "en",
		},
		{
			Pattern:     regexp.MustCompile(flags + `(new|different|updated|override)\s+instructions?`),
			Description: "Attempt to inject new instructions",
			Severity:    SeverityHigh,
			Language:    "en",
		},
		// 角色操纵尝试
		{
			Pattern:     regexp.MustCompile(flags + `you\s+are\s+now\s+(a|an|the)?`),
			Description: "Attempt to change model role",
			Severity:    SeverityHigh,
			Language:    "en",
		},
		{
			Pattern:     regexp.MustCompile(flags + `act\s+as\s+(if\s+you\s+are\s+)?(a|an|the)?`),
			Description: "Attempt to change model behavior",
			Severity:    SeverityMedium,
			Language:    "en",
		},
		{
			Pattern:     regexp.MustCompile(flags + `pretend\s+(to\s+be|you\s+are)\s+(a|an|the)?`),
			Description: "Attempt to make model pretend",
			Severity:    SeverityMedium,
			Language:    "en",
		},
		// 系统/作用标记
		{
			Pattern:     regexp.MustCompile(flags + `^\s*system\s*:\s*`),
			Description: "System role marker injection",
			Severity:    SeverityCritical,
			Language:    "universal",
		},
		{
			Pattern:     regexp.MustCompile(flags + `^\s*assistant\s*:\s*`),
			Description: "Assistant role marker injection",
			Severity:    SeverityHigh,
			Language:    "universal",
		},
		{
			Pattern:     regexp.MustCompile(flags + `^\s*user\s*:\s*`),
			Description: "User role marker injection",
			Severity:    SeverityMedium,
			Language:    "universal",
		},
		{
			Pattern:     regexp.MustCompile(flags + `<\s*system\s*>`),
			Description: "XML system tag injection",
			Severity:    SeverityCritical,
			Language:    "universal",
		},
		{
			Pattern:     regexp.MustCompile(flags + `\[\s*INST\s*\]`),
			Description: "Instruction tag injection",
			Severity:    SeverityHigh,
			Language:    "universal",
		},
		// 越狱未遂
		{
			Pattern:     regexp.MustCompile(flags + `(do\s+)?anything\s+now`),
			Description: "DAN jailbreak attempt",
			Severity:    SeverityCritical,
			Language:    "en",
		},
		{
			Pattern:     regexp.MustCompile(flags + `jailbreak`),
			Description: "Explicit jailbreak mention",
			Severity:    SeverityCritical,
			Language:    "universal",
		},
		// Chinese patterns - 指令覆盖尝试
		{
			Pattern:     regexp.MustCompile(`忽略(之前|上面|以上|先前|前面)(的)?(指令|指示|规则|提示|要求)`),
			Description: "尝试忽略之前的指令",
			Severity:    SeverityCritical,
			Language:    "zh",
		},
		{
			Pattern:     regexp.MustCompile(`忘(记|掉)(之前|上面|以上|所有|一切)(的)?(内容|指令|指示|规则)?`),
			Description: "尝试让模型忘记上下文",
			Severity:    SeverityCritical,
			Language:    "zh",
		},
		{
			Pattern:     regexp.MustCompile(`(新的|新|不同的|更新的|覆盖)(指令|指示|规则|要求)`),
			Description: "尝试注入新指令",
			Severity:    SeverityHigh,
			Language:    "zh",
		},
		{
			Pattern:     regexp.MustCompile(`不要(遵守|遵循|听从)(之前|上面|以上|任何)(的)?(指令|指示|规则)?`),
			Description: "尝试让模型不遵守指令",
			Severity:    SeverityCritical,
			Language:    "zh",
		},
		// 中国角色操纵
		{
			Pattern:     regexp.MustCompile(`你现在是(一个|一名)?`),
			Description: "尝试改变模型角色",
			Severity:    SeverityHigh,
			Language:    "zh",
		},
		{
			Pattern:     regexp.MustCompile(`(假装|假设|扮演)(你是)?`),
			Description: "尝试让模型扮演角色",
			Severity:    SeverityMedium,
			Language:    "zh",
		},
		{
			Pattern:     regexp.MustCompile(`从现在开始(你是|你要|你将)`),
			Description: "尝试改变模型行为",
			Severity:    SeverityHigh,
			Language:    "zh",
		},
		// 破坏者逃跑未遂
		{
			Pattern:     regexp.MustCompile(flags + `---+\s*(system|instructions?|rules?)\s*---+`),
			Description: "Delimiter-based injection attempt",
			Severity:    SeverityHigh,
			Language:    "universal",
		},
		{
			Pattern:     regexp.MustCompile(flags + `===+\s*(system|instructions?|rules?)\s*===+`),
			Description: "Delimiter-based injection attempt",
			Severity:    SeverityHigh,
			Language:    "universal",
		},
	}

	return patterns
}

// Name 返回验证器名称
func (d *InjectionDetector) Name() string {
	return "injection_detector"
}

// Priority 返回优先级
func (d *InjectionDetector) Priority() int {
	return d.priority
}

// InjectionMatch 注入匹配结果
type InjectionMatch struct {
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Position    int    `json:"position"`
	Length      int    `json:"length"`
	MatchedText string `json:"matched_text"`
}

// Validate 执行注入检测验证
// 实现 Validator 接口
func (d *InjectionDetector) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()

	// 检测所有注入模式
	matches := d.Detect(content)
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

	// 添加验证错误
	result.AddError(ValidationError{
		Code:     ErrCodeInjectionDetected,
		Message:  formatInjectionErrorMessage(matches),
		Severity: highestSeverity,
	})

	// 记录检测信息到 metadata
	result.Metadata["injection_detected"] = true
	result.Metadata["injection_matches"] = matches
	result.Metadata["injection_count"] = len(matches)

	return result, nil
}

// Detect 检测内容中的所有注入模式
func (d *InjectionDetector) Detect(content string) []InjectionMatch {
	var matches []InjectionMatch

	// 如果启用分隔符隔离，检查是否有分隔符逃逸
	if d.useDelimiters {
		delimiterMatches := d.detectDelimiterEscape(content)
		matches = append(matches, delimiterMatches...)
	}

	// 检测所有注入模式
	for _, pattern := range d.patterns {
		locs := pattern.Pattern.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			matchedText := content[loc[0]:loc[1]]
			matches = append(matches, InjectionMatch{
				Pattern:     pattern.Pattern.String(),
				Description: pattern.Description,
				Severity:    pattern.Severity,
				Position:    loc[0],
				Length:      loc[1] - loc[0],
				MatchedText: matchedText,
			})
		}
	}

	return matches
}

// detectDelimiterEscape 检测分隔符逃逸尝试
func (d *InjectionDetector) detectDelimiterEscape(content string) []InjectionMatch {
	var matches []InjectionMatch

	// 检测常见的分隔符逃逸模式
	escapePatterns := []struct {
		pattern     *regexp.Regexp
		description string
	}{
		{
			pattern:     regexp.MustCompile(`(?i)\]\s*\[\s*(system|inst)`),
			description: "Bracket delimiter escape",
		},
		{
			pattern:     regexp.MustCompile(`(?i)>\s*<\s*(system|inst)`),
			description: "Angle bracket delimiter escape",
		},
		{
			pattern:     regexp.MustCompile(`(?i)\}\s*\{\s*(system|inst)`),
			description: "Brace delimiter escape",
		},
		{
			pattern:     regexp.MustCompile(`(?i)"""\s*(system|instructions)`),
			description: "Triple quote delimiter escape",
		},
		{
			pattern:     regexp.MustCompile("(?i)```\\s*(system|instructions)"),
			description: "Code block delimiter escape",
		},
	}

	for _, ep := range escapePatterns {
		locs := ep.pattern.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			matches = append(matches, InjectionMatch{
				Pattern:     ep.pattern.String(),
				Description: ep.description,
				Severity:    SeverityHigh,
				Position:    loc[0],
				Length:      loc[1] - loc[0],
				MatchedText: content[loc[0]:loc[1]],
			})
		}
	}

	return matches
}

// IsolateWithDelimiters 使用分隔符隔离用户输入
// 返回被安全分隔符包围的内容
func (d *InjectionDetector) IsolateWithDelimiters(content string) string {
	delimiter := "<<<USER_INPUT>>>"
	return delimiter + "\n" + content + "\n" + delimiter
}

// IsolateWithRole 使用角色标记隔离用户输入
// 返回带有明确角色标记的内容
func (d *InjectionDetector) IsolateWithRole(content string) string {
	return "[USER_MESSAGE_START]\n" + content + "\n[USER_MESSAGE_END]"
}

// compareSeverity 比较两个严重级别
// 返回: >0 如果 a > b, <0 如果 a < b, 0 如果相等
func compareSeverity(a, b string) int {
	severityOrder := map[string]int{
		SeverityLow:      1,
		SeverityMedium:   2,
		SeverityHigh:     3,
		SeverityCritical: 4,
	}
	return severityOrder[a] - severityOrder[b]
}

// formatInjectionErrorMessage 格式化注入错误消息
func formatInjectionErrorMessage(matches []InjectionMatch) string {
	if len(matches) == 0 {
		return "检测到潜在的提示注入攻击"
	}

	// 收集唯一的描述
	descriptions := make([]string, 0)
	seen := make(map[string]bool)
	for _, match := range matches {
		if !seen[match.Description] {
			seen[match.Description] = true
			descriptions = append(descriptions, match.Description)
		}
	}

	if len(descriptions) == 1 {
		return "检测到提示注入攻击: " + descriptions[0]
	}

	return "检测到多个提示注入模式: " + strings.Join(descriptions, "; ")
}
