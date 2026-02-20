// 包护栏为代理商提供输入/输出验证和内容过滤.
package guardrails

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOutputValidator(t *testing.T) {
	tests := []struct {
		name   string
		config *OutputValidatorConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &OutputValidatorConfig{
				SafeReplacement: "custom replacement",
				EnableAuditLog:  true,
				Priority:        100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewOutputValidator(tt.config)
			assert.NotNil(t, v)
			assert.Equal(t, "output_validator", v.Name())
		})
	}
}

func TestOutputValidator_Validate(t *testing.T) {
	ctx := context.Background()

	// 创建 PII 检测器作为验证器
	piiDetector := NewPIIDetector(&PIIDetectorConfig{
		Action:   PIIActionReject,
		Priority: 100,
	})

	config := &OutputValidatorConfig{
		Validators:     []Validator{piiDetector},
		EnableAuditLog: true,
	}
	v := NewOutputValidator(config)

	tests := []struct {
		name        string
		content     string
		expectValid bool
	}{
		{
			name:        "clean content passes",
			content:     "这是一段正常的输出内容",
			expectValid: true,
		},
		{
			name:        "content with phone number fails",
			content:     "请联系 13812345678 获取更多信息",
			expectValid: false,
		},
		{
			name:        "content with email fails",
			content:     "请发送邮件到 test@example.com",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.Validate(ctx, tt.content)
			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, result.Valid)
		})
	}
}

func TestOutputValidator_ValidateAndFilter(t *testing.T) {
	ctx := context.Background()

	// 创建内容过滤器
	contentFilter, err := NewContentFilter(&ContentFilterConfig{
		BlockedPatterns: []string{`badword`, `harmful`},
		Replacement:     "[已过滤]",
	})
	require.NoError(t, err)

	// 创建 PII 检测器（脱敏模式）
	piiDetector := NewPIIDetector(&PIIDetectorConfig{
		Action:   PIIActionMask,
		Priority: 100,
	})

	config := &OutputValidatorConfig{
		Validators:      []Validator{piiDetector},
		Filters:         []Filter{contentFilter},
		SafeReplacement: "[内容已被安全系统过滤]",
		EnableAuditLog:  true,
	}
	v := NewOutputValidator(config)

	tests := []struct {
		name           string
		content        string
		expectFiltered bool
		expectContains string
	}{
		{
			name:           "clean content unchanged",
			content:        "这是正常内容",
			expectFiltered: false,
			expectContains: "这是正常内容",
		},
		{
			name:           "blocked word filtered",
			content:        "这里有 badword 内容",
			expectFiltered: true,
			expectContains: "[已过滤]",
		},
		{
			name:           "multiple blocked words filtered",
			content:        "badword 和 harmful 都被过滤",
			expectFiltered: true,
			expectContains: "[已过滤]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered, result, err := v.ValidateAndFilter(ctx, tt.content)
			require.NoError(t, err)
			assert.NotNil(t, result)

			if tt.expectFiltered {
				assert.Contains(t, filtered, tt.expectContains)
				assert.NotEqual(t, tt.content, filtered)
			} else {
				assert.Equal(t, tt.content, filtered)
			}
		})
	}
}

func TestOutputValidator_SafeReplacement(t *testing.T) {
	ctx := context.Background()

	// 创建一个总是返回严重错误的验证器
	criticalValidator := &mockCriticalValidator{}

	config := &OutputValidatorConfig{
		Validators:      []Validator{criticalValidator},
		SafeReplacement: "[安全替代响应]",
		EnableAuditLog:  true,
	}
	v := NewOutputValidator(config)

	filtered, result, err := v.ValidateAndFilter(ctx, "任何内容")
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.Equal(t, "[安全替代响应]", filtered)
	assert.True(t, result.Metadata["replaced_with_safe_response"].(bool))
}

// mockCriticalValidator 模拟返回严重错误的验证器
type mockCriticalValidator struct{}

func (v *mockCriticalValidator) Name() string  { return "mock_critical" }
func (v *mockCriticalValidator) Priority() int { return 1 }
func (v *mockCriticalValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()
	result.AddError(ValidationError{
		Code:     "CRITICAL_ERROR",
		Message:  "严重错误",
		Severity: SeverityCritical,
	})
	return result, nil
}

func TestContentFilter_New(t *testing.T) {
	tests := []struct {
		name        string
		config      *ContentFilterConfig
		expectError bool
	}{
		{
			name:        "nil config uses defaults",
			config:      nil,
			expectError: false,
		},
		{
			name: "valid patterns",
			config: &ContentFilterConfig{
				BlockedPatterns: []string{`test`, `\d+`},
				Replacement:     "[X]",
			},
			expectError: false,
		},
		{
			name: "invalid regex pattern",
			config: &ContentFilterConfig{
				BlockedPatterns: []string{`[invalid`},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewContentFilter(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, f)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

func TestContentFilter_Filter(t *testing.T) {
	ctx := context.Background()

	f, err := NewContentFilter(&ContentFilterConfig{
		BlockedPatterns: []string{`badword`, `harmful`, `\d{3}-\d{4}`},
		Replacement:     "[FILTERED]",
		CaseSensitive:   false,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "no match",
			content:  "clean content",
			expected: "clean content",
		},
		{
			name:     "single match",
			content:  "this is badword here",
			expected: "this is [FILTERED] here",
		},
		{
			name:     "multiple matches",
			content:  "badword and harmful content",
			expected: "[FILTERED] and [FILTERED] content",
		},
		{
			name:     "case insensitive",
			content:  "BADWORD and BadWord",
			expected: "[FILTERED] and [FILTERED]",
		},
		{
			name:     "regex pattern match",
			content:  "call 123-4567 now",
			expected: "call [FILTERED] now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := f.Filter(ctx, tt.content)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContentFilter_Detect(t *testing.T) {
	f, err := NewContentFilter(&ContentFilterConfig{
		BlockedPatterns: []string{`badword`},
		Replacement:     "[X]",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		content     string
		expectCount int
	}{
		{
			name:        "no match",
			content:     "clean content",
			expectCount: 0,
		},
		{
			name:        "single match",
			content:     "has badword here",
			expectCount: 1,
		},
		{
			name:        "multiple matches",
			content:     "badword and badword again",
			expectCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := f.Detect(tt.content)
			assert.Len(t, matches, tt.expectCount)
		})
	}
}

func TestContentFilter_AddRemovePattern(t *testing.T) {
	f, err := NewContentFilter(nil)
	require.NoError(t, err)

	// 添加模式
	err = f.AddPattern(`test`)
	assert.NoError(t, err)
	assert.Len(t, f.GetPatterns(), 1)

	// 添加另一个模式
	err = f.AddPattern(`another`)
	assert.NoError(t, err)
	assert.Len(t, f.GetPatterns(), 2)

	// 移除模式
	removed := f.RemovePattern(`test`)
	assert.True(t, removed)
	assert.Len(t, f.GetPatterns(), 1)

	// 移除不存在的模式
	removed = f.RemovePattern(`nonexistent`)
	assert.False(t, removed)
}

func TestContentFilterValidator(t *testing.T) {
	ctx := context.Background()

	f, err := NewContentFilter(&ContentFilterConfig{
		BlockedPatterns: []string{`blocked`},
		Replacement:     "[X]",
	})
	require.NoError(t, err)

	v := NewContentFilterValidator(f, 50)
	assert.Equal(t, "content_filter_validator", v.Name())
	assert.Equal(t, 50, v.Priority())

	// 测试验证
	result, err := v.Validate(ctx, "this has blocked content")
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.True(t, result.Metadata["blocked_content_detected"].(bool))

	// 测试无匹配
	result, err = v.Validate(ctx, "clean content")
	require.NoError(t, err)
	assert.True(t, result.Valid)
}

func TestMemoryAuditLogger_Log(t *testing.T) {
	ctx := context.Background()
	logger := NewMemoryAuditLogger(10)

	entry := &AuditLogEntry{
		Timestamp:     time.Now(),
		EventType:     AuditEventValidationFailed,
		ValidatorName: "test_validator",
		ContentHash:   "abc123",
		Errors: []ValidationError{
			{Code: "TEST", Message: "test error", Severity: SeverityHigh},
		},
	}

	err := logger.Log(ctx, entry)
	assert.NoError(t, err)

	entries := logger.GetEntries()
	assert.Len(t, entries, 1)
	assert.Equal(t, "test_validator", entries[0].ValidatorName)
}

func TestMemoryAuditLogger_MaxSize(t *testing.T) {
	ctx := context.Background()
	logger := NewMemoryAuditLogger(3)

	// 添加 5 个条目
	for i := 0; i < 5; i++ {
		entry := &AuditLogEntry{
			Timestamp:     time.Now(),
			EventType:     AuditEventValidationFailed,
			ValidatorName: "validator_" + string(rune('A'+i)),
		}
		err := logger.Log(ctx, entry)
		assert.NoError(t, err)
	}

	// 应该只保留最后 3 个
	entries := logger.GetEntries()
	assert.Len(t, entries, 3)
	assert.Equal(t, "validator_C", entries[0].ValidatorName)
	assert.Equal(t, "validator_D", entries[1].ValidatorName)
	assert.Equal(t, "validator_E", entries[2].ValidatorName)
}

func TestMemoryAuditLogger_Query(t *testing.T) {
	ctx := context.Background()
	logger := NewMemoryAuditLogger(100)

	now := time.Now()

	// 添加不同类型的条目
	entries := []*AuditLogEntry{
		{Timestamp: now.Add(-2 * time.Hour), EventType: AuditEventValidationFailed, ValidatorName: "v1"},
		{Timestamp: now.Add(-1 * time.Hour), EventType: AuditEventPIIDetected, ValidatorName: "v2"},
		{Timestamp: now, EventType: AuditEventValidationFailed, ValidatorName: "v1"},
	}

	for _, e := range entries {
		err := logger.Log(ctx, e)
		require.NoError(t, err)
	}

	tests := []struct {
		name        string
		filter      *AuditLogFilter
		expectCount int
	}{
		{
			name:        "no filter returns all",
			filter:      nil,
			expectCount: 3,
		},
		{
			name: "filter by event type",
			filter: &AuditLogFilter{
				EventTypes: []AuditEventType{AuditEventValidationFailed},
			},
			expectCount: 2,
		},
		{
			name: "filter by validator name",
			filter: &AuditLogFilter{
				ValidatorNames: []string{"v1"},
			},
			expectCount: 2,
		},
		{
			name: "filter with limit",
			filter: &AuditLogFilter{
				Limit: 1,
			},
			expectCount: 1,
		},
		{
			name: "filter with offset",
			filter: &AuditLogFilter{
				Offset: 2,
			},
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := logger.Query(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectCount)
		})
	}
}

func TestMemoryAuditLogger_Count(t *testing.T) {
	ctx := context.Background()
	logger := NewMemoryAuditLogger(100)

	// 添加条目
	for i := 0; i < 5; i++ {
		eventType := AuditEventValidationFailed
		if i%2 == 0 {
			eventType = AuditEventPIIDetected
		}
		entry := &AuditLogEntry{
			Timestamp: time.Now(),
			EventType: eventType,
		}
		err := logger.Log(ctx, entry)
		require.NoError(t, err)
	}

	// 总数
	count, err := logger.Count(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 5, count)

	// 按类型过滤
	count, err = logger.Count(ctx, &AuditLogFilter{
		EventTypes: []AuditEventType{AuditEventPIIDetected},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestMemoryAuditLogger_Clear(t *testing.T) {
	ctx := context.Background()
	logger := NewMemoryAuditLogger(100)

	// 添加条目
	entry := &AuditLogEntry{
		Timestamp: time.Now(),
		EventType: AuditEventValidationFailed,
	}
	err := logger.Log(ctx, entry)
	require.NoError(t, err)

	assert.Len(t, logger.GetEntries(), 1)

	// 清空
	logger.Clear()
	assert.Len(t, logger.GetEntries(), 0)
}

func TestAuditLogEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := &AuditLogEntry{
		Timestamp:     now,
		EventType:     AuditEventValidationFailed,
		ValidatorName: "test_validator",
		ContentHash:   "hash123",
		Errors: []ValidationError{
			{Code: "ERR1", Message: "error 1", Severity: SeverityHigh},
		},
		Metadata: map[string]any{
			"key": "value",
		},
	}

	assert.Equal(t, now, entry.Timestamp)
	assert.Equal(t, AuditEventValidationFailed, entry.EventType)
	assert.Equal(t, "test_validator", entry.ValidatorName)
	assert.Equal(t, "hash123", entry.ContentHash)
	assert.Len(t, entry.Errors, 1)
	assert.Equal(t, "value", entry.Metadata["key"])
}

func TestHashContent(t *testing.T) {
	// 相同内容应该产生相同哈希
	hash1 := hashContent("test content")
	hash2 := hashContent("test content")
	assert.Equal(t, hash1, hash2)

	// 不同内容应该产生不同哈希
	hash3 := hashContent("different content")
	assert.NotEqual(t, hash1, hash3)

	// 哈希应该是 64 字符的十六进制字符串
	assert.Len(t, hash1, 64)
}

func TestOutputValidator_AuditLogging(t *testing.T) {
	ctx := context.Background()

	// 创建审计日志记录器
	auditLogger := NewMemoryAuditLogger(100)

	// 创建 PII 检测器（拒绝模式）
	piiDetector := NewPIIDetector(&PIIDetectorConfig{
		Action:   PIIActionReject,
		Priority: 100,
	})

	config := &OutputValidatorConfig{
		Validators:     []Validator{piiDetector},
		EnableAuditLog: true,
		AuditLogger:    auditLogger,
	}
	v := NewOutputValidator(config)

	// 验证包含 PII 的内容
	_, err := v.Validate(ctx, "联系电话: 13812345678")
	require.NoError(t, err)

	// 检查审计日志
	entries := auditLogger.GetEntries()
	assert.Len(t, entries, 1)
	assert.Equal(t, AuditEventValidationFailed, entries[0].EventType)
	assert.Equal(t, "pii_detector", entries[0].ValidatorName)
	assert.NotEmpty(t, entries[0].ContentHash)
	assert.NotEmpty(t, entries[0].Errors)
}

func TestOutputValidator_AddValidatorAndFilter(t *testing.T) {
	v := NewOutputValidator(nil)

	// 添加验证器
	piiDetector := NewPIIDetector(nil)
	v.AddValidator(piiDetector)

	// 添加过滤器
	contentFilter, _ := NewContentFilter(nil)
	v.AddFilter(contentFilter)

	// 验证添加成功
	assert.NotNil(t, v)
}

func TestOutputValidator_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	v := NewOutputValidator(&OutputValidatorConfig{
		Validators: []Validator{NewPIIDetector(nil)},
	})

	_, err := v.Validate(ctx, "test content")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
