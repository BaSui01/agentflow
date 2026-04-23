package guardrails

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPIIDetector(t *testing.T) {
	tests := []struct {
		name           string
		config         *PIIDetectorConfig
		expectedAction PIIAction
		expectedTypes  int
	}{
		{
			name:           "nil config uses defaults",
			config:         nil,
			expectedAction: PIIActionMask,
			expectedTypes:  4, // phone, email, id_card, bank_card
		},
		{
			name: "custom action",
			config: &PIIDetectorConfig{
				Action: PIIActionReject,
			},
			expectedAction: PIIActionReject,
			expectedTypes:  4,
		},
		{
			name: "specific enabled types",
			config: &PIIDetectorConfig{
				Action:       PIIActionWarn,
				EnabledTypes: []PIIType{PIITypePhone, PIITypeEmail},
			},
			expectedAction: PIIActionWarn,
			expectedTypes:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewPIIDetector(tt.config)
			assert.NotNil(t, detector)
			assert.Equal(t, tt.expectedAction, detector.GetAction())
			assert.Len(t, detector.patterns, tt.expectedTypes)
		})
	}
}

func TestPIIDetector_Name(t *testing.T) {
	detector := NewPIIDetector(nil)
	assert.Equal(t, "pii_detector", detector.Name())
}

func TestPIIDetector_Priority(t *testing.T) {
	tests := []struct {
		name     string
		priority int
	}{
		{"default priority", 100},
		{"custom priority", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &PIIDetectorConfig{Priority: tt.priority}
			detector := NewPIIDetector(config)
			assert.Equal(t, tt.priority, detector.Priority())
		})
	}
}

func TestPIIDetector_Detect_Phone(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		EnabledTypes: []PIIType{PIITypePhone},
	})

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"valid phone", "我的手机号是13812345678", 1},
		{"multiple phones", "联系方式：13812345678 或 15987654321", 2},
		{"no phone", "这是一段普通文本", 0},
		{"invalid phone - too short", "1381234567", 0},
		{"invalid phone - wrong prefix", "12812345678", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := detector.Detect(tt.content)
			assert.Len(t, matches, tt.expected)
			for _, m := range matches {
				assert.Equal(t, PIITypePhone, m.Type)
			}
		})
	}
}

func TestPIIDetector_Detect_Email(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		EnabledTypes: []PIIType{PIITypeEmail},
	})

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"valid email", "邮箱：test@example.com", 1},
		{"multiple emails", "联系：a@b.com 和 c@d.org", 2},
		{"no email", "这是一段普通文本", 0},
		{"complex email", "user.name+tag@sub.domain.co.uk", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := detector.Detect(tt.content)
			assert.Len(t, matches, tt.expected)
			for _, m := range matches {
				assert.Equal(t, PIITypeEmail, m.Type)
			}
		})
	}
}

func TestPIIDetector_Detect_IDCard(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		EnabledTypes: []PIIType{PIITypeIDCard},
	})

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"valid id card", "身份证号：110101199001011234", 1},
		{"id card with X", "身份证：11010119900101123X", 1},
		{"no id card", "这是一段普通文本", 0},
		{"invalid - wrong date", "身份证：110101199013011234", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := detector.Detect(tt.content)
			assert.Len(t, matches, tt.expected)
			for _, m := range matches {
				assert.Equal(t, PIITypeIDCard, m.Type)
			}
		})
	}
}

func TestPIIDetector_Detect_BankCard(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		EnabledTypes: []PIIType{PIITypeBankCard},
	})

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"16 digit card", "卡号：6222021234567890", 1},
		{"19 digit card", "卡号：6222021234567890123", 1},
		{"no bank card", "这是一段普通文本", 0},
		{"too short", "123456789012345", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := detector.Detect(tt.content)
			assert.Len(t, matches, tt.expected)
			for _, m := range matches {
				assert.Equal(t, PIITypeBankCard, m.Type)
			}
		})
	}
}

func TestPIIDetector_Detect_Multiple(t *testing.T) {
	detector := NewPIIDetector(nil)

	content := "联系方式：手机13812345678，邮箱test@example.com，身份证110101199001011234"
	matches := detector.Detect(content)

	// 应该检测到手机号、邮箱、身份证
	assert.GreaterOrEqual(t, len(matches), 3)

	types := make(map[PIIType]bool)
	for _, m := range matches {
		types[m.Type] = true
	}
	assert.True(t, types[PIITypePhone])
	assert.True(t, types[PIITypeEmail])
	assert.True(t, types[PIITypeIDCard])
}

func TestPIIDetector_Mask(t *testing.T) {
	t.Run("mask phone", func(t *testing.T) {
		detector := NewPIIDetector(&PIIDetectorConfig{
			EnabledTypes: []PIIType{PIITypePhone},
		})
		result := detector.Mask("手机：13812345678")
		assert.Equal(t, "手机：138****5678", result)
	})

	t.Run("mask email", func(t *testing.T) {
		detector := NewPIIDetector(&PIIDetectorConfig{
			EnabledTypes: []PIIType{PIITypeEmail},
		})
		result := detector.Mask("邮箱：test@example.com")
		assert.Equal(t, "邮箱：t***@example.com", result)
	})

	t.Run("mask id card", func(t *testing.T) {
		detector := NewPIIDetector(&PIIDetectorConfig{
			EnabledTypes: []PIIType{PIITypeIDCard},
		})
		result := detector.Mask("身份证：110101199001011234")
		assert.Equal(t, "身份证：110101********1234", result)
	})

	t.Run("mask bank card", func(t *testing.T) {
		detector := NewPIIDetector(&PIIDetectorConfig{
			EnabledTypes: []PIIType{PIITypeBankCard},
		})
		result := detector.Mask("卡号：6222021234567890")
		assert.Equal(t, "卡号：6222********7890", result)
	})

	t.Run("no pii", func(t *testing.T) {
		detector := NewPIIDetector(nil)
		result := detector.Mask("普通文本")
		assert.Equal(t, "普通文本", result)
	})
}

func TestPIIDetector_Validate_Reject(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		Action: PIIActionReject,
	})

	ctx := context.Background()

	t.Run("reject when PII detected", func(t *testing.T) {
		result, err := detector.Validate(ctx, "手机号：13812345678")
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
		assert.Equal(t, ErrCodePIIDetected, result.Errors[0].Code)
		assert.Equal(t, SeverityHigh, result.Errors[0].Severity)
	})

	t.Run("pass when no PII", func(t *testing.T) {
		result, err := detector.Validate(ctx, "普通文本内容")
		require.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})
}

func TestPIIDetector_Validate_Warn(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		Action: PIIActionWarn,
	})

	ctx := context.Background()

	t.Run("warn when PII detected", func(t *testing.T) {
		result, err := detector.Validate(ctx, "手机号：13812345678")
		require.NoError(t, err)
		assert.True(t, result.Valid) // 警告模式不会使结果无效
		assert.Empty(t, result.Errors)
		assert.NotEmpty(t, result.Warnings)
	})
}

func TestPIIDetector_Validate_Mask(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		Action: PIIActionMask,
	})

	ctx := context.Background()

	t.Run("mask when PII detected", func(t *testing.T) {
		result, err := detector.Validate(ctx, "手机号：13812345678")
		require.NoError(t, err)
		assert.True(t, result.Valid) // 脱敏模式不会使结果无效
		assert.NotEmpty(t, result.Warnings)
		assert.Contains(t, result.Metadata, "masked_content")
		assert.Equal(t, "手机号：138****5678", result.Metadata["masked_content"])
	})
}

func TestPIIDetector_Filter(t *testing.T) {
	detector := NewPIIDetector(nil)
	ctx := context.Background()

	content := "联系方式：13812345678"
	filtered, err := detector.Filter(ctx, content)
	require.NoError(t, err)
	assert.Equal(t, "联系方式：138****5678", filtered)
}

func TestPIIDetector_SetAction(t *testing.T) {
	detector := NewPIIDetector(&PIIDetectorConfig{
		Action: PIIActionMask,
	})

	assert.Equal(t, PIIActionMask, detector.GetAction())

	detector.SetAction(PIIActionReject)
	assert.Equal(t, PIIActionReject, detector.GetAction())
}

func TestPIIDetector_CustomPatterns(t *testing.T) {
	// 自定义手机号模式（只匹配138开头）
	customPattern := regexp.MustCompile(`138\d{8}`)

	detector := NewPIIDetector(&PIIDetectorConfig{
		EnabledTypes: []PIIType{PIITypePhone},
		CustomPatterns: map[PIIType]*regexp.Regexp{
			PIITypePhone: customPattern,
		},
	})

	t.Run("match custom pattern", func(t *testing.T) {
		matches := detector.Detect("手机：13812345678")
		assert.Len(t, matches, 1)
	})

	t.Run("not match non-custom pattern", func(t *testing.T) {
		matches := detector.Detect("手机：15912345678")
		assert.Len(t, matches, 0)
	})
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name     string
		piiType  PIIType
		value    string
		expected string
	}{
		{"phone normal", PIITypePhone, "13812345678", "138****5678"},
		{"phone short", PIITypePhone, "123456", "******"},
		{"email normal", PIITypeEmail, "test@example.com", "t***@example.com"},
		{"email no at", PIITypeEmail, "invalid", "*******"},
		{"id card normal", PIITypeIDCard, "110101199001011234", "110101********1234"},
		{"id card short", PIITypeIDCard, "123456789", "*********"},
		{"bank card 16", PIITypeBankCard, "6222021234567890", "6222********7890"},
		{"bank card 19", PIITypeBankCard, "6222021234567890123", "6222***********0123"},
		{"bank card short", PIITypeBankCard, "1234567", "*******"},
		{"address", PIITypeAddress, "北京市朝阳区xxx路", "[地址已脱敏]"},
		{"unknown type", PIIType("unknown"), "secret", "******"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskValue(tt.piiType, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPIIDetector_ImplementsValidator(t *testing.T) {
	var _ Validator = (*PIIDetector)(nil)
}

func TestPIIDetector_ImplementsFilter(t *testing.T) {
	var _ Filter = (*PIIDetector)(nil)
}
