package guardrails

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 长度变量测试
// ============================================================================

func TestNewLengthValidator(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		v := NewLengthValidator(nil)
		assert.NotNil(t, v)
		assert.Equal(t, 10000, v.GetMaxLength())
		assert.Equal(t, LengthActionReject, v.GetAction())
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &LengthValidatorConfig{
			MaxLength: 100,
			Action:    LengthActionTruncate,
			Priority:  5,
		}
		v := NewLengthValidator(config)
		assert.Equal(t, 100, v.GetMaxLength())
		assert.Equal(t, LengthActionTruncate, v.GetAction())
		assert.Equal(t, 5, v.Priority())
	})
}

func TestLengthValidator_Name(t *testing.T) {
	v := NewLengthValidator(nil)
	assert.Equal(t, "length_validator", v.Name())
}

func TestLengthValidator_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		maxLength      int
		action         LengthAction
		content        string
		expectValid    bool
		expectError    bool
		expectWarning  bool
		expectTruncate bool
	}{
		{
			name:        "content within limit",
			maxLength:   100,
			action:      LengthActionReject,
			content:     "Hello, World!",
			expectValid: true,
		},
		{
			name:        "content exactly at limit",
			maxLength:   5,
			action:      LengthActionReject,
			content:     "Hello",
			expectValid: true,
		},
		{
			name:        "content exceeds limit - reject",
			maxLength:   5,
			action:      LengthActionReject,
			content:     "Hello, World!",
			expectValid: false,
			expectError: true,
		},
		{
			name:           "content exceeds limit - truncate",
			maxLength:      5,
			action:         LengthActionTruncate,
			content:        "Hello, World!",
			expectValid:    true,
			expectWarning:  true,
			expectTruncate: true,
		},
		{
			name:        "chinese content within limit",
			maxLength:   10,
			action:      LengthActionReject,
			content:     "你好世界",
			expectValid: true,
		},
		{
			name:        "chinese content exceeds limit - reject",
			maxLength:   3,
			action:      LengthActionReject,
			content:     "你好世界",
			expectValid: false,
			expectError: true,
		},
		{
			name:           "chinese content exceeds limit - truncate",
			maxLength:      3,
			action:         LengthActionTruncate,
			content:        "你好世界",
			expectValid:    true,
			expectWarning:  true,
			expectTruncate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &LengthValidatorConfig{
				MaxLength: tt.maxLength,
				Action:    tt.action,
			}
			v := NewLengthValidator(config)

			result, err := v.Validate(ctx, tt.content)
			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, result.Valid)

			if tt.expectError {
				assert.NotEmpty(t, result.Errors)
				assert.Equal(t, ErrCodeMaxLengthExceeded, result.Errors[0].Code)
			}

			if tt.expectWarning {
				assert.NotEmpty(t, result.Warnings)
			}

			if tt.expectTruncate {
				truncated, ok := result.Metadata["truncated_content"].(string)
				assert.True(t, ok)
				assert.Equal(t, tt.maxLength, len([]rune(truncated)))
			}
		})
	}
}

func TestLengthValidator_Truncate(t *testing.T) {
	v := NewLengthValidator(&LengthValidatorConfig{MaxLength: 5})

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "content within limit",
			content:  "Hello",
			expected: "Hello",
		},
		{
			name:     "content exceeds limit",
			content:  "Hello, World!",
			expected: "Hello",
		},
		{
			name:     "chinese content truncate",
			content:  "你好世界测试",
			expected: "你好世界测",
		},
		{
			name:     "empty content",
			content:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Truncate(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLengthValidator_Metadata(t *testing.T) {
	ctx := context.Background()
	v := NewLengthValidator(&LengthValidatorConfig{
		MaxLength: 5,
		Action:    LengthActionReject,
	})

	result, err := v.Validate(ctx, "Hello, World!")
	require.NoError(t, err)

	assert.Equal(t, 13, result.Metadata["original_length"])
	assert.Equal(t, 5, result.Metadata["max_length"])
	assert.Equal(t, 8, result.Metadata["exceeded_by"])
}

// ============================================================================
// 关键词校正测试
// ============================================================================

func TestNewKeywordValidator(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		v := NewKeywordValidator(nil)
		assert.NotNil(t, v)
		assert.Empty(t, v.GetBlockedKeywords())
		assert.Equal(t, KeywordActionReject, v.GetAction())
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &KeywordValidatorConfig{
			BlockedKeywords: []string{"bad", "evil"},
			Action:          KeywordActionWarn,
			CaseSensitive:   true,
			Priority:        15,
		}
		v := NewKeywordValidator(config)
		assert.Equal(t, []string{"bad", "evil"}, v.GetBlockedKeywords())
		assert.Equal(t, KeywordActionWarn, v.GetAction())
		assert.Equal(t, 15, v.Priority())
	})
}

func TestKeywordValidator_Name(t *testing.T) {
	v := NewKeywordValidator(nil)
	assert.Equal(t, "keyword_validator", v.Name())
}

func TestKeywordValidator_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		keywords      []string
		severities    map[string]string
		action        KeywordAction
		caseSensitive bool
		content       string
		expectValid   bool
		expectError   bool
		expectWarning bool
		expectFilter  bool
	}{
		{
			name:        "no blocked keywords",
			keywords:    []string{},
			content:     "Hello, World!",
			expectValid: true,
		},
		{
			name:        "content without blocked keywords",
			keywords:    []string{"bad", "evil"},
			content:     "Hello, World!",
			expectValid: true,
		},
		{
			name:        "content with blocked keyword - reject",
			keywords:    []string{"bad", "evil"},
			action:      KeywordActionReject,
			content:     "This is bad content",
			expectValid: false,
			expectError: true,
		},
		{
			name:          "content with blocked keyword - warn",
			keywords:      []string{"bad", "evil"},
			action:        KeywordActionWarn,
			content:       "This is bad content",
			expectValid:   true,
			expectWarning: true,
		},
		{
			name:          "content with blocked keyword - filter",
			keywords:      []string{"bad", "evil"},
			action:        KeywordActionFilter,
			content:       "This is bad content",
			expectValid:   true,
			expectWarning: true,
			expectFilter:  true,
		},
		{
			name:          "case insensitive match",
			keywords:      []string{"bad"},
			action:        KeywordActionReject,
			caseSensitive: false,
			content:       "This is BAD content",
			expectValid:   false,
			expectError:   true,
		},
		{
			name:          "case sensitive no match",
			keywords:      []string{"bad"},
			action:        KeywordActionReject,
			caseSensitive: true,
			content:       "This is BAD content",
			expectValid:   true,
		},
		{
			name:        "multiple blocked keywords",
			keywords:    []string{"bad", "evil"},
			action:      KeywordActionReject,
			content:     "This is bad and evil content",
			expectValid: false,
			expectError: true,
		},
		{
			name:        "chinese blocked keywords",
			keywords:    []string{"禁止", "违规"},
			action:      KeywordActionReject,
			content:     "这是禁止的内容",
			expectValid: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &KeywordValidatorConfig{
				BlockedKeywords:   tt.keywords,
				KeywordSeverities: tt.severities,
				Action:            tt.action,
				CaseSensitive:     tt.caseSensitive,
			}
			v := NewKeywordValidator(config)

			result, err := v.Validate(ctx, tt.content)
			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, result.Valid)

			if tt.expectError {
				assert.NotEmpty(t, result.Errors)
				assert.Equal(t, ErrCodeBlockedKeyword, result.Errors[0].Code)
			}

			if tt.expectWarning {
				assert.NotEmpty(t, result.Warnings)
			}

			if tt.expectFilter {
				filtered, ok := result.Metadata["filtered_content"].(string)
				assert.True(t, ok)
				assert.NotContains(t, strings.ToLower(filtered), "bad")
			}
		})
	}
}

func TestKeywordValidator_Detect(t *testing.T) {
	config := &KeywordValidatorConfig{
		BlockedKeywords: []string{"bad", "evil"},
		KeywordSeverities: map[string]string{
			"bad":  SeverityMedium,
			"evil": SeverityCritical,
		},
		CaseSensitive: false,
	}
	v := NewKeywordValidator(config)

	t.Run("detect multiple occurrences", func(t *testing.T) {
		matches := v.Detect("bad bad bad")
		assert.Len(t, matches, 3)
		for _, m := range matches {
			assert.Equal(t, "bad", m.Keyword)
			assert.Equal(t, SeverityMedium, m.Severity)
		}
	})

	t.Run("detect different keywords", func(t *testing.T) {
		matches := v.Detect("bad and evil")
		assert.Len(t, matches, 2)

		keywords := make(map[string]bool)
		for _, m := range matches {
			keywords[m.Keyword] = true
		}
		assert.True(t, keywords["bad"])
		assert.True(t, keywords["evil"])
	})

	t.Run("detect with positions", func(t *testing.T) {
		matches := v.Detect("this is bad content")
		require.Len(t, matches, 1)
		assert.Equal(t, 8, matches[0].Position)
		assert.Equal(t, 3, matches[0].Length)
	})
}

func TestKeywordValidator_Filter(t *testing.T) {
	config := &KeywordValidatorConfig{
		BlockedKeywords: []string{"bad", "evil"},
		Replacement:     "[FILTERED]",
		CaseSensitive:   false,
	}
	v := NewKeywordValidator(config)

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "filter single keyword",
			content:  "This is bad content",
			expected: "This is [FILTERED] content",
		},
		{
			name:     "filter multiple keywords",
			content:  "This is bad and evil content",
			expected: "This is [FILTERED] and [FILTERED] content",
		},
		{
			name:     "filter case insensitive",
			content:  "This is BAD and Evil content",
			expected: "This is [FILTERED] and [FILTERED] content",
		},
		{
			name:     "no keywords to filter",
			content:  "This is good content",
			expected: "This is good content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Filter(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKeywordValidator_AddRemoveKeyword(t *testing.T) {
	v := NewKeywordValidator(&KeywordValidatorConfig{
		BlockedKeywords: []string{"bad"},
	})

	t.Run("add keyword", func(t *testing.T) {
		v.AddKeyword("evil", SeverityCritical)
		keywords := v.GetBlockedKeywords()
		assert.Contains(t, keywords, "bad")
		assert.Contains(t, keywords, "evil")
	})

	t.Run("remove keyword", func(t *testing.T) {
		v.RemoveKeyword("bad")
		keywords := v.GetBlockedKeywords()
		assert.NotContains(t, keywords, "bad")
		assert.Contains(t, keywords, "evil")
	})
}

func TestKeywordValidator_Severity(t *testing.T) {
	ctx := context.Background()
	config := &KeywordValidatorConfig{
		BlockedKeywords: []string{"low", "medium", "high", "critical"},
		KeywordSeverities: map[string]string{
			"low":      SeverityLow,
			"medium":   SeverityMedium,
			"high":     SeverityHigh,
			"critical": SeverityCritical,
		},
		Action: KeywordActionReject,
	}
	v := NewKeywordValidator(config)

	t.Run("highest severity is used", func(t *testing.T) {
		result, err := v.Validate(ctx, "low medium high critical")
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, SeverityCritical, result.Errors[0].Severity)
	})

	t.Run("single keyword severity", func(t *testing.T) {
		result, err := v.Validate(ctx, "only low severity")
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, SeverityLow, result.Errors[0].Severity)
	})
}

func TestKeywordValidator_Metadata(t *testing.T) {
	ctx := context.Background()
	config := &KeywordValidatorConfig{
		BlockedKeywords: []string{"bad", "evil"},
		Action:          KeywordActionReject,
	}
	v := NewKeywordValidator(config)

	result, err := v.Validate(ctx, "bad and evil content")
	require.NoError(t, err)

	assert.True(t, result.Metadata["blocked_keywords_detected"].(bool))
	assert.Equal(t, 2, result.Metadata["keyword_count"])

	matches, ok := result.Metadata["keyword_matches"].([]KeywordMatch)
	assert.True(t, ok)
	assert.Len(t, matches, 2)
}

// ============================================================================
// 整合测试
// ============================================================================

func TestValidators_ImplementInterface(t *testing.T) {
	// 确保两个验证器执行验证器接口
	var _ Validator = (*LengthValidator)(nil)
	var _ Validator = (*KeywordValidator)(nil)
}

func TestValidators_PriorityOrder(t *testing.T) {
	lengthValidator := NewLengthValidator(&LengthValidatorConfig{Priority: 10})
	keywordValidator := NewKeywordValidator(&KeywordValidatorConfig{Priority: 20})
	piiDetector := NewPIIDetector(&PIIDetectorConfig{Priority: 100})
	injectionDetector := NewInjectionDetector(&InjectionDetectorConfig{Priority: 50})

	validators := []Validator{piiDetector, keywordValidator, lengthValidator, injectionDetector}

	// 按优先级排序( 下数 = 高优先级)
	for i := 0; i < len(validators)-1; i++ {
		for j := i + 1; j < len(validators); j++ {
			if validators[i].Priority() > validators[j].Priority() {
				validators[i], validators[j] = validators[j], validators[i]
			}
		}
	}

	// 校验顺序:长度(10) - > 关键词(20) - > 注射(50) - > pii (100)
	assert.Equal(t, "length_validator", validators[0].Name())
	assert.Equal(t, "keyword_validator", validators[1].Name())
	assert.Equal(t, "injection_detector", validators[2].Name())
	assert.Equal(t, "pii_detector", validators[3].Name())
}
