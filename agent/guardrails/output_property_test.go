package guardrails

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// 特性:代理框架-2026-增强,财产6:产出验证失败记录
// 审定:要求2.5 - 记录所有审定失败事件以供审计
// 此属性测试验证验证失败被正确记录 。
func TestProperty_OutputValidator_FailureLogging(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		auditLogger := NewMemoryAuditLogger(100)

		outputValidator := NewOutputValidator(&OutputValidatorConfig{
			EnableAuditLog: true,
			AuditLogger:    auditLogger,
		})

		// 添加失败的验证符
		errorCode := rapid.SampledFrom([]string{
			ErrCodePIIDetected,
			ErrCodeContentBlocked,
			ErrCodeValidationFailed,
		}).Draw(rt, "errorCode")

		errorMessage := rapid.StringMatching(`[a-zA-Z ]{10,30}`).Draw(rt, "errorMessage")

		outputValidator.AddValidator(&propMockFailingValidator{
			name:         "test_validator",
			priority:     10,
			errorCode:    errorCode,
			errorMessage: errorMessage,
			severity:     SeverityHigh,
		})

		ctx := context.Background()
		content := rapid.StringMatching(`[a-zA-Z ]{20,50}`).Draw(rt, "content")

		_, err := outputValidator.Validate(ctx, content)
		require.NoError(t, err)

		// 已创建审计日志条目
		entries := auditLogger.GetEntries()
		require.NotEmpty(t, entries, "Should have audit log entries")

		entry := entries[len(entries)-1]
		assert.Equal(t, AuditEventValidationFailed, entry.EventType, "Should be validation failed event")
		assert.Equal(t, "test_validator", entry.ValidatorName, "Should log validator name")
		assert.NotEmpty(t, entry.ContentHash, "Should have content hash")
		assert.NotEmpty(t, entry.Errors, "Should have errors in log")
		assert.False(t, entry.Timestamp.IsZero(), "Should have timestamp")
	})
}

// 测试 Property outputValidator AuditLogQuery 测试审计日志查询
func TestProperty_OutputValidator_AuditLogQuery(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		auditLogger := NewMemoryAuditLogger(100)

		// 生成多个日志条目
		numEntries := rapid.IntRange(5, 20).Draw(rt, "numEntries")
		eventTypes := []AuditEventType{
			AuditEventValidationFailed,
			AuditEventContentFiltered,
			AuditEventPIIDetected,
			AuditEventInjectionDetected,
		}

		ctx := context.Background()
		for i := 0; i < numEntries; i++ {
			entry := &AuditLogEntry{
				Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
				EventType:     rapid.SampledFrom(eventTypes).Draw(rt, fmt.Sprintf("eventType_%d", i)),
				ValidatorName: fmt.Sprintf("validator_%d", i%3),
				ContentHash:   fmt.Sprintf("hash_%d", i),
			}
			err := auditLogger.Log(ctx, entry)
			require.NoError(t, err)
		}

		// 查询所有条目
		allEntries, err := auditLogger.Query(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, allEntries, numEntries, "Should return all entries")

		// 使用事件类型过滤器查询
		filterType := rapid.SampledFrom(eventTypes).Draw(rt, "filterType")
		filtered, err := auditLogger.Query(ctx, &AuditLogFilter{
			EventTypes: []AuditEventType{filterType},
		})
		require.NoError(t, err)
		for _, entry := range filtered {
			assert.Equal(t, filterType, entry.EventType, "Filtered entries should match event type")
		}

		// 查询限制
		limit := rapid.IntRange(1, numEntries).Draw(rt, "limit")
		limited, err := auditLogger.Query(ctx, &AuditLogFilter{
			Limit: limit,
		})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(limited), limit, "Should respect limit")
	})
}

// 测试Property outputValidator 连接测试内容过滤
func TestProperty_OutputValidator_ContentFiltering(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 以被屏蔽的模式创建内容过滤器
		blockedWord := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "blockedWord")
		filter, err := NewContentFilter(&ContentFilterConfig{
			BlockedPatterns: []string{blockedWord},
			Replacement:     "[BLOCKED]",
		})
		require.NoError(t, err)

		outputValidator := NewOutputValidator(&OutputValidatorConfig{
			Filters: []Filter{filter},
		})

		ctx := context.Background()

		// 含有被封锁单词的内容
		content := "prefix " + blockedWord + " suffix"
		filtered, result, err := outputValidator.ValidateAndFilter(ctx, content)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotContains(t, filtered, blockedWord, "Filtered content should not contain blocked word")
		assert.Contains(t, filtered, "[BLOCKED]", "Should contain replacement")
	})
}

// 检测 Property outputValidator 安全更换测试 关键错误的安全替换
func TestProperty_OutputValidator_SafeReplacement(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		safeReplacement := rapid.StringMatching(`\[SAFE: [a-z]{5,10}\]`).Draw(rt, "safeReplacement")

		outputValidator := NewOutputValidator(&OutputValidatorConfig{
			SafeReplacement: safeReplacement,
		})

		// 添加返回关键错误的验证符
		outputValidator.AddValidator(&propMockFailingValidator{
			name:         "critical_validator",
			priority:     10,
			errorCode:    ErrCodeContentBlocked,
			errorMessage: "Critical content blocked",
			severity:     SeverityCritical,
		})

		ctx := context.Background()
		content := rapid.StringMatching(`[a-zA-Z ]{20,50}`).Draw(rt, "content")

		filtered, result, err := outputValidator.ValidateAndFilter(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, safeReplacement, filtered, "Should return safe replacement for critical errors")

		replaced, ok := result.Metadata["replaced_with_safe_response"].(bool)
		assert.True(t, ok && replaced, "Should mark as replaced with safe response")
	})
}

// 测试Property ContentFilter PatternMatching 测试内容过滤模式匹配
func TestProperty_ContentFilter_PatternMatching(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成图案
		numPatterns := rapid.IntRange(1, 3).Draw(rt, "numPatterns")
		patterns := make([]string, numPatterns)
		for i := range patterns {
			patterns[i] = rapid.StringMatching(`[a-z]{4,8}`).Draw(rt, fmt.Sprintf("pattern_%d", i))
		}

		filter, err := NewContentFilter(&ContentFilterConfig{
			BlockedPatterns: patterns,
			Replacement:     "[X]",
			CaseSensitive:   false,
		})
		require.NoError(t, err)

		ctx := context.Background()

		// 检测到每个模式
		for _, pattern := range patterns {
			content := "text " + pattern + " more"
			matches := filter.Detect(content)
			assert.NotEmpty(t, matches, "Should detect pattern: %s", pattern)

			filtered, err := filter.Filter(ctx, content)
			require.NoError(t, err)
			assert.NotContains(t, filtered, pattern, "Filtered should not contain pattern")
		}
	})
}

// 测试Property ContentFilter Add RemovePattern 测试动态模式管理
func TestProperty_ContentFilter_AddRemovePattern(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		filter, err := NewContentFilter(&ContentFilterConfig{
			Replacement: "[REMOVED]",
		})
		require.NoError(t, err)

		// 添加图案
		numPatterns := rapid.IntRange(2, 5).Draw(rt, "numPatterns")
		patterns := make([]string, numPatterns)
		for i := range patterns {
			patterns[i] = rapid.StringMatching(`[a-z]{5,8}`).Draw(rt, fmt.Sprintf("pattern_%d", i))
			err := filter.AddPattern(patterns[i])
			require.NoError(t, err)
		}

		assert.Len(t, filter.GetPatterns(), numPatterns, "Should have all patterns")

		// 删除一个图案
		removeIndex := rapid.IntRange(0, numPatterns-1).Draw(rt, "removeIndex")
		removed := filter.RemovePattern(patterns[removeIndex])
		assert.True(t, removed, "Should successfully remove pattern")
		assert.Len(t, filter.GetPatterns(), numPatterns-1, "Should have one less pattern")
	})
}

// 测试Property  AuditLogger MaxSize 测试 最大大小的审计日志执行
func TestProperty_AuditLogger_MaxSize(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxSize := rapid.IntRange(5, 20).Draw(rt, "maxSize")
		logger := NewMemoryAuditLogger(maxSize)

		ctx := context.Background()

		// 添加大于最大大小的条目
		numEntries := maxSize + rapid.IntRange(5, 15).Draw(rt, "extraEntries")
		for i := 0; i < numEntries; i++ {
			entry := &AuditLogEntry{
				Timestamp:     time.Now(),
				EventType:     AuditEventValidationFailed,
				ValidatorName: fmt.Sprintf("validator_%d", i),
				ContentHash:   fmt.Sprintf("hash_%d", i),
			}
			err := logger.Log(ctx, entry)
			require.NoError(t, err)
		}

		// 不应超过最大尺寸
		entries := logger.GetEntries()
		assert.LessOrEqual(t, len(entries), maxSize, "Should not exceed max size")
	})
}

// 测试Property  AuditLogger  TimeRangFilter 测试时间范围过滤
func TestProperty_AuditLogger_TimeRangeFilter(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		logger := NewMemoryAuditLogger(100)
		ctx := context.Background()

		baseTime := time.Now()

		// 添加带有不同时间戳的条目
		for i := 0; i < 10; i++ {
			entry := &AuditLogEntry{
				Timestamp:     baseTime.Add(time.Duration(i) * time.Hour),
				EventType:     AuditEventValidationFailed,
				ValidatorName: fmt.Sprintf("validator_%d", i),
				ContentHash:   fmt.Sprintf("hash_%d", i),
			}
			err := logger.Log(ctx, entry)
			require.NoError(t, err)
		}

		// 查询时间范围
		startTime := baseTime.Add(2 * time.Hour)
		endTime := baseTime.Add(7 * time.Hour)

		filtered, err := logger.Query(ctx, &AuditLogFilter{
			StartTime: &startTime,
			EndTime:   &endTime,
		})
		require.NoError(t, err)

		for _, entry := range filtered {
			assert.True(t, !entry.Timestamp.Before(startTime), "Entry should be after start time")
			assert.True(t, !entry.Timestamp.After(endTime), "Entry should be before end time")
		}
	})
}

// 测试Property ContentFilterValidator 集成测试 内容FilterValidator集成
func TestProperty_ContentFilterValidator_Integration(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		blockedWord := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "blockedWord")

		filter, err := NewContentFilter(&ContentFilterConfig{
			BlockedPatterns: []string{blockedWord},
			Replacement:     "[FILTERED]",
		})
		require.NoError(t, err)

		validator := NewContentFilterValidator(filter, 50)

		ctx := context.Background()

		// 含有被封锁单词的内容
		content := "some " + blockedWord + " text"
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect blocked content")
		assert.Equal(t, ErrCodeContentBlocked, result.Errors[0].Code)

		// 检查元数据中过滤的内容
		filtered, ok := result.Metadata["filtered_content"].(string)
		require.True(t, ok, "Should have filtered_content")
		assert.NotContains(t, filtered, blockedWord)
	})
}
