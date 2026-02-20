package guardrails

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// 特性:代理框架-2026-增强,属性2:输入长度限制
// 校验: 1.3要求 - 超过最大长度的输入截断或拒绝
// 此属性测试可以验证长度验证器正确处理长度限制 。
func TestProperty_LengthValidator_InputLengthLimit(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(10, 100).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionReject,
		})

		ctx := context.Background()

		// 在限度内测试内容
		withinLimit := rapid.IntRange(1, maxLength).Draw(rt, "withinLimit")
		shortContent := strings.Repeat("a", withinLimit)
		result, err := validator.Validate(ctx, shortContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Content within limit should pass: len=%d, max=%d", withinLimit, maxLength)

		// 测试内容超过限度
		exceedBy := rapid.IntRange(1, 50).Draw(rt, "exceedBy")
		longContent := strings.Repeat("a", maxLength+exceedBy)
		result, err = validator.Validate(ctx, longContent)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Content exceeding limit should fail: len=%d, max=%d", maxLength+exceedBy, maxLength)
		assert.NotEmpty(t, result.Errors)
		assert.Equal(t, ErrCodeMaxLengthExceeded, result.Errors[0].Code)
	})
}

// 测试Property LengthValidator Truncate Mode 测试分解行为
func TestProperty_LengthValidator_TruncateMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(10, 100).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionTruncate,
		})

		ctx := context.Background()

		// 测试内容超过限度
		exceedBy := rapid.IntRange(1, 50).Draw(rt, "exceedBy")
		longContent := strings.Repeat("x", maxLength+exceedBy)
		result, err := validator.Validate(ctx, longContent)
		require.NoError(t, err)

		// 截断模式不应无效, 但添加警告
		assert.True(t, result.Valid, "Truncate mode should not invalidate")
		assert.NotEmpty(t, result.Warnings, "Should have truncation warning")

		// 检查元数据中截断的内容
		truncated, ok := result.Metadata["truncated_content"].(string)
		require.True(t, ok, "Should have truncated_content in metadata")
		assert.Len(t, truncated, maxLength, "Truncated content should be exactly maxLength")
	})
}

// 测试 Property LengthValidator ChineseCharacters 测试中国字符计数
func TestProperty_LengthValidator_ChineseCharacters(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(5, 20).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionReject,
		})

		ctx := context.Background()

		// 在限制范围内生成中文内容
		charCount := rapid.IntRange(1, maxLength).Draw(rt, "charCount")
		chineseContent := strings.Repeat("中", charCount)
		result, err := validator.Validate(ctx, chineseContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Chinese content within limit should pass: chars=%d, max=%d", charCount, maxLength)

		// 生成超过限制的中文内容
		exceedCount := maxLength + rapid.IntRange(1, 10).Draw(rt, "exceedCount")
		longChinese := strings.Repeat("文", exceedCount)
		result, err = validator.Validate(ctx, longChinese)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Chinese content exceeding limit should fail")
	})
}

// 测试Property LengthValidator TruncatePreserves Prefix 测试前缀保存内容前缀
func TestProperty_LengthValidator_TruncatePreservesPrefix(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(10, 50).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionTruncate,
		})

		// 生成独有的内容
		content := rapid.StringMatching(`[a-z]{60,100}`).Draw(rt, "content")
		truncated := validator.Truncate(content)

		// 截断内容应为原件的前缀
		assert.True(t, strings.HasPrefix(content, truncated), "Truncated should be prefix of original")
		assert.LessOrEqual(t, len(truncated), maxLength, "Truncated length should not exceed max")
	})
}

// 测试 Property 关键词服务器 BlockKeyword检测测试关键词检测
func TestProperty_KeywordValidator_BlockedKeywordDetection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机被屏蔽的关键字
		keywordCount := rapid.IntRange(1, 5).Draw(rt, "keywordCount")
		keywords := make([]string, keywordCount)
		for i := range keywords {
			keywords[i] = rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, fmt.Sprintf("keyword_%d", i))
		}

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: keywords,
			Action:          KeywordActionReject,
			CaseSensitive:   false,
		})

		ctx := context.Background()

		// 用被屏蔽的关键字测试内容
		keyword := rapid.SampledFrom(keywords).Draw(rt, "selectedKeyword")
		content := "some text " + keyword + " more text"
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect blocked keyword: %s", keyword)
		assert.Equal(t, ErrCodeBlockedKeyword, result.Errors[0].Code)

		// 测试没有被屏蔽的关键字的内容
		cleanContent := rapid.StringMatching(`[0-9]{20,30}`).Draw(rt, "cleanContent")
		result, err = validator.Validate(ctx, cleanContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Content without keywords should pass")
	})
}

// 测试Property   关键词变异器  案件不敏感测试案例不敏感匹配
func TestProperty_KeywordValidator_CaseInsensitive(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		keyword := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword")

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: []string{keyword},
			Action:          KeywordActionReject,
			CaseSensitive:   false,
		})

		ctx := context.Background()

		// 测试大写版本
		upperContent := "text " + strings.ToUpper(keyword) + " text"
		result, err := validator.Validate(ctx, upperContent)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect uppercase keyword")

		// 测试混合情况
		mixedCase := strings.ToUpper(keyword[:len(keyword)/2]) + keyword[len(keyword)/2:]
		mixedContent := "text " + mixedCase + " text"
		result, err = validator.Validate(ctx, mixedContent)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect mixed case keyword")
	})
}

// 测试Property   关键词变换器  FilterMode 测试关键词过滤
func TestProperty_KeywordValidator_FilterMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		keyword := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword")
		replacement := "[FILTERED]"

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: []string{keyword},
			Action:          KeywordActionFilter,
			Replacement:     replacement,
		})

		ctx := context.Background()

		content := "prefix " + keyword + " suffix"
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)

		// 过滤模式不应无效
		assert.True(t, result.Valid, "Filter mode should not invalidate")

		// 检查过滤内容
		filtered, ok := result.Metadata["filtered_content"].(string)
		require.True(t, ok, "Should have filtered_content")
		assert.NotContains(t, filtered, keyword, "Filtered content should not contain keyword")
		assert.Contains(t, filtered, replacement, "Filtered content should contain replacement")
	})
}

// 测试 Property 关键词变换器  重度映射测试关键词重度映射
func TestProperty_KeywordValidator_SeverityMapping(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		keyword := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword")
		severity := rapid.SampledFrom([]string{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}).Draw(rt, "severity")

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: []string{keyword},
			KeywordSeverities: map[string]string{
				keyword: severity,
			},
			Action: KeywordActionReject,
		})

		ctx := context.Background()

		content := "text " + keyword + " text"
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, severity, result.Errors[0].Severity, "Should use configured severity")
	})
}

// 测试 Property 关键词变换器 多键关键词测试多键检测
func TestProperty_KeywordValidator_MultipleKeywords(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		keywords := []string{
			rapid.StringMatching(`[a-z]{4,6}`).Draw(rt, "keyword1"),
			rapid.StringMatching(`[a-z]{4,6}`).Draw(rt, "keyword2"),
			rapid.StringMatching(`[a-z]{4,6}`).Draw(rt, "keyword3"),
		}

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: keywords,
			Action:          KeywordActionReject,
		})

		ctx := context.Background()

		// 有多个关键字的内容
		content := keywords[0] + " and " + keywords[1] + " and " + keywords[2]
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid)

		// 检查关键字计数的元数据
		count, ok := result.Metadata["keyword_count"].(int)
		require.True(t, ok, "Should have keyword_count")
		assert.GreaterOrEqual(t, count, 3, "Should detect all keywords")
	})
}

// 测试Property   关键词服务器  AddRemoveKeyword 测试动态关键字管理
func TestProperty_KeywordValidator_AddRemoveKeyword(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		initialKeyword := rapid.StringMatching(`[a-z]{5,8}`).Draw(rt, "initialKeyword")
		newKeyword := rapid.StringMatching(`[a-z]{5,8}`).Draw(rt, "newKeyword")

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: []string{initialKeyword},
			Action:          KeywordActionReject,
		})

		ctx := context.Background()

		// 应发现初始关键字
		result, _ := validator.Validate(ctx, initialKeyword)
		assert.False(t, result.Valid, "Initial keyword should be detected")

		// 添加新关键字
		validator.AddKeyword(newKeyword, SeverityHigh)
		result, _ = validator.Validate(ctx, newKeyword)
		assert.False(t, result.Valid, "New keyword should be detected")

		// 删除初始关键字
		validator.RemoveKeyword(initialKeyword)
		result, _ = validator.Validate(ctx, initialKeyword)
		assert.True(t, result.Valid, "Removed keyword should not be detected")
	})
}
