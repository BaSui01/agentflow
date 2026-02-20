// 包护栏为代理商提供输入/输出验证和内容过滤.
package guardrails

import (
	"context"
	"regexp"
	"strings"
)

// PIIType PII 类型
type PIIType string

const (
	// PIITypePhone 手机号
	PIITypePhone PIIType = "phone"
	// PIITypeEmail 邮箱
	PIITypeEmail PIIType = "email"
	// PIITypeIDCard 身份证号
	PIITypeIDCard PIIType = "id_card"
	// PIITypeBankCard 银行卡号
	PIITypeBankCard PIIType = "bank_card"
	// PIITypeAddress 地址
	PIITypeAddress PIIType = "address"
)

// PIIAction PII 处理动作
type PIIAction string

const (
	// PIIActionMask 脱敏处理
	PIIActionMask PIIAction = "mask"
	// PIIActionReject 拒绝处理
	PIIActionReject PIIAction = "reject"
	// PIIActionWarn 警告处理
	PIIActionWarn PIIAction = "warn"
)

// PIIMatch PII 匹配结果
type PIIMatch struct {
	Type     PIIType `json:"type"`
	Value    string  `json:"value"`
	Masked   string  `json:"masked"`
	Position int     `json:"position"`
	Length   int     `json:"length"`
}

// PIIDetectorConfig PII 检测器配置
type PIIDetectorConfig struct {
	// Action 处理动作
	Action PIIAction
	// EnabledTypes 启用的 PII 类型，为空则启用所有类型
	EnabledTypes []PIIType
	// CustomPatterns 自定义正则模式
	CustomPatterns map[PIIType]*regexp.Regexp
	// Priority 验证器优先级
	Priority int
}

// DefaultPIIDetectorConfig 返回默认配置
func DefaultPIIDetectorConfig() *PIIDetectorConfig {
	return &PIIDetectorConfig{
		Action:         PIIActionMask,
		EnabledTypes:   nil, // 启用所有类型
		CustomPatterns: nil,
		Priority:       100,
	}
}

// PIIDetector PII 检测器
// 实现 Validator 接口，用于检测和处理个人身份信息
type PIIDetector struct {
	patterns map[PIIType]*regexp.Regexp
	action   PIIAction
	priority int
}

// NewPIIDetector 创建 PII 检测器
func NewPIIDetector(config *PIIDetectorConfig) *PIIDetector {
	if config == nil {
		config = DefaultPIIDetectorConfig()
	}

	detector := &PIIDetector{
		patterns: make(map[PIIType]*regexp.Regexp),
		action:   config.Action,
		priority: config.Priority,
	}

	// 初始化默认正则模式
	defaultPatterns := getDefaultPatterns()

	// 确定要启用的类型
	enabledTypes := config.EnabledTypes
	if len(enabledTypes) == 0 {
		// 启用所有默认类型
		enabledTypes = []PIIType{
			PIITypePhone,
			PIITypeEmail,
			PIITypeIDCard,
			PIITypeBankCard,
		}
	}

	// 加载启用类型的模式
	for _, piiType := range enabledTypes {
		if customPattern, ok := config.CustomPatterns[piiType]; ok {
			detector.patterns[piiType] = customPattern
		} else if defaultPattern, ok := defaultPatterns[piiType]; ok {
			detector.patterns[piiType] = defaultPattern
		}
	}

	return detector
}

// getDefaultPatterns 返回默认的 PII 正则模式
func getDefaultPatterns() map[PIIType]*regexp.Regexp {
	return map[PIIType]*regexp.Regexp{
		// 中国大陆手机号: 1开头，第二位3-9，共11位
		PIITypePhone: regexp.MustCompile(`1[3-9]\d{9}`),
		// 邮箱地址
		PIITypeEmail: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		// 中国大陆身份证号: 18位，最后一位可能是X
		PIITypeIDCard: regexp.MustCompile(`[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`),
		// 银行卡号: 16-19位数字
		PIITypeBankCard: regexp.MustCompile(`\d{16,19}`),
	}
}

// Name 返回验证器名称
func (d *PIIDetector) Name() string {
	return "pii_detector"
}

// Priority 返回优先级
func (d *PIIDetector) Priority() int {
	return d.priority
}

// Validate 执行 PII 检测验证
// 实现 Validator 接口
func (d *PIIDetector) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()

	// 检测所有 PII
	matches := d.Detect(content)
	if len(matches) == 0 {
		return result, nil
	}

	// 记录检测到的 PII 类型
	detectedTypes := make(map[PIIType]int)
	for _, match := range matches {
		detectedTypes[match.Type]++
	}

	// 根据配置的动作处理
	switch d.action {
	case PIIActionReject:
		// 拒绝模式：添加错误
		for piiType, count := range detectedTypes {
			result.AddError(ValidationError{
				Code:     ErrCodePIIDetected,
				Message:  formatPIIErrorMessage(piiType, count),
				Severity: SeverityHigh,
				Field:    string(piiType),
			})
		}
	case PIIActionWarn:
		// 警告模式：添加警告
		for piiType, count := range detectedTypes {
			result.AddWarning(formatPIIWarningMessage(piiType, count))
		}
	case PIIActionMask:
		// 脱敏模式：添加警告并在 metadata 中提供脱敏后的内容
		for piiType, count := range detectedTypes {
			result.AddWarning(formatPIIWarningMessage(piiType, count))
		}
		result.Metadata["masked_content"] = d.Mask(content)
		result.Metadata["pii_matches"] = matches
	}

	// 记录检测到的 PII 信息到 metadata
	result.Metadata["pii_detected"] = true
	result.Metadata["pii_types"] = detectedTypes

	return result, nil
}

// Detect 检测内容中的所有 PII
func (d *PIIDetector) Detect(content string) []PIIMatch {
	var matches []PIIMatch

	for piiType, pattern := range d.patterns {
		locs := pattern.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			value := content[loc[0]:loc[1]]
			matches = append(matches, PIIMatch{
				Type:     piiType,
				Value:    value,
				Masked:   maskValue(piiType, value),
				Position: loc[0],
				Length:   loc[1] - loc[0],
			})
		}
	}

	return matches
}

// Mask 对内容中的 PII 进行脱敏处理
func (d *PIIDetector) Mask(content string) string {
	result := content

	for piiType, pattern := range d.patterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			return maskValue(piiType, match)
		})
	}

	return result
}

// Filter 实现 Filter 接口，对内容进行脱敏过滤
func (d *PIIDetector) Filter(ctx context.Context, content string) (string, error) {
	return d.Mask(content), nil
}

// GetAction 返回当前配置的处理动作
func (d *PIIDetector) GetAction() PIIAction {
	return d.action
}

// SetAction 设置处理动作
func (d *PIIDetector) SetAction(action PIIAction) {
	d.action = action
}

// maskValue 根据 PII 类型对值进行脱敏
func maskValue(piiType PIIType, value string) string {
	switch piiType {
	case PIITypePhone:
		// 手机号: 保留前3位和后4位，中间用****替换
		if len(value) >= 7 {
			return value[:3] + "****" + value[len(value)-4:]
		}
		return strings.Repeat("*", len(value))
	case PIITypeEmail:
		// 邮箱: 保留@前的首字符和@后的域名，中间用***替换
		atIndex := strings.Index(value, "@")
		if atIndex > 0 {
			return value[:1] + "***" + value[atIndex:]
		}
		return strings.Repeat("*", len(value))
	case PIITypeIDCard:
		// 身份证: 保留前6位和后4位，中间用********替换
		if len(value) >= 10 {
			return value[:6] + "********" + value[len(value)-4:]
		}
		return strings.Repeat("*", len(value))
	case PIITypeBankCard:
		// 银行卡: 保留前4位和后4位，中间用****替换
		if len(value) >= 8 {
			return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
		}
		return strings.Repeat("*", len(value))
	case PIITypeAddress:
		// 地址: 全部替换为 [地址已脱敏]
		return "[地址已脱敏]"
	default:
		return strings.Repeat("*", len(value))
	}
}

// formatPIIErrorMessage 格式化 PII 错误消息
func formatPIIErrorMessage(piiType PIIType, count int) string {
	typeNames := map[PIIType]string{
		PIITypePhone:    "手机号",
		PIITypeEmail:    "邮箱地址",
		PIITypeIDCard:   "身份证号",
		PIITypeBankCard: "银行卡号",
		PIITypeAddress:  "地址信息",
	}
	typeName := typeNames[piiType]
	if typeName == "" {
		typeName = string(piiType)
	}
	return "检测到 " + typeName + " 信息，已拒绝处理"
}

// formatPIIWarningMessage 格式化 PII 警告消息
func formatPIIWarningMessage(piiType PIIType, count int) string {
	typeNames := map[PIIType]string{
		PIITypePhone:    "手机号",
		PIITypeEmail:    "邮箱地址",
		PIITypeIDCard:   "身份证号",
		PIITypeBankCard: "银行卡号",
		PIITypeAddress:  "地址信息",
	}
	typeName := typeNames[piiType]
	if typeName == "" {
		typeName = string(piiType)
	}
	return "检测到 " + typeName + " 信息"
}
