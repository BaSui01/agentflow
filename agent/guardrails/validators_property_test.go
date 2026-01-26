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

// Feature: agent-framework-2026-enhancements, Property 2: Input Length Limit
// Validates: Requirements 1.3 - Truncate or reject input exceeding max length
// This property test verifies that length validator correctly handles length limits.
func TestProperty_LengthValidator_InputLengthLimit(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(10, 100).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionReject,
		})

		ctx := context.Background()

		// Test content within limit
		withinLimit := rapid.IntRange(1, maxLength).Draw(rt, "withinLimit")
		shortContent := strings.Repeat("a", withinLimit)
		result, err := validator.Validate(ctx, shortContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Content within limit should pass: len=%d, max=%d", withinLimit, maxLength)

		// Test content exceeding limit
		exceedBy := rapid.IntRange(1, 50).Draw(rt, "exceedBy")
		longContent := strings.Repeat("a", maxLength+exceedBy)
		result, err = validator.Validate(ctx, longContent)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Content exceeding limit should fail: len=%d, max=%d", maxLength+exceedBy, maxLength)
		assert.NotEmpty(t, result.Errors)
		assert.Equal(t, ErrCodeMaxLengthExceeded, result.Errors[0].Code)
	})
}

// TestProperty_LengthValidator_TruncateMode tests truncation behavior
func TestProperty_LengthValidator_TruncateMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(10, 100).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionTruncate,
		})

		ctx := context.Background()

		// Test content exceeding limit
		exceedBy := rapid.IntRange(1, 50).Draw(rt, "exceedBy")
		longContent := strings.Repeat("x", maxLength+exceedBy)
		result, err := validator.Validate(ctx, longContent)
		require.NoError(t, err)

		// Truncate mode should not invalidate, but add warning
		assert.True(t, result.Valid, "Truncate mode should not invalidate")
		assert.NotEmpty(t, result.Warnings, "Should have truncation warning")

		// Check truncated content in metadata
		truncated, ok := result.Metadata["truncated_content"].(string)
		require.True(t, ok, "Should have truncated_content in metadata")
		assert.Len(t, truncated, maxLength, "Truncated content should be exactly maxLength")
	})
}

// TestProperty_LengthValidator_ChineseCharacters tests Chinese character counting
func TestProperty_LengthValidator_ChineseCharacters(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(5, 20).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionReject,
		})

		ctx := context.Background()

		// Generate Chinese content within limit
		charCount := rapid.IntRange(1, maxLength).Draw(rt, "charCount")
		chineseContent := strings.Repeat("中", charCount)
		result, err := validator.Validate(ctx, chineseContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Chinese content within limit should pass: chars=%d, max=%d", charCount, maxLength)

		// Generate Chinese content exceeding limit
		exceedCount := maxLength + rapid.IntRange(1, 10).Draw(rt, "exceedCount")
		longChinese := strings.Repeat("文", exceedCount)
		result, err = validator.Validate(ctx, longChinese)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Chinese content exceeding limit should fail")
	})
}

// TestProperty_LengthValidator_TruncatePreservesPrefix tests truncation preserves content prefix
func TestProperty_LengthValidator_TruncatePreservesPrefix(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxLength := rapid.IntRange(10, 50).Draw(rt, "maxLength")
		validator := NewLengthValidator(&LengthValidatorConfig{
			MaxLength: maxLength,
			Action:    LengthActionTruncate,
		})

		// Generate unique content
		content := rapid.StringMatching(`[a-z]{60,100}`).Draw(rt, "content")
		truncated := validator.Truncate(content)

		// Truncated content should be prefix of original
		assert.True(t, strings.HasPrefix(content, truncated), "Truncated should be prefix of original")
		assert.LessOrEqual(t, len(truncated), maxLength, "Truncated length should not exceed max")
	})
}

// TestProperty_KeywordValidator_BlockedKeywordDetection tests keyword detection
func TestProperty_KeywordValidator_BlockedKeywordDetection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random blocked keywords
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

		// Test content with blocked keyword
		keyword := rapid.SampledFrom(keywords).Draw(rt, "selectedKeyword")
		content := "some text " + keyword + " more text"
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect blocked keyword: %s", keyword)
		assert.Equal(t, ErrCodeBlockedKeyword, result.Errors[0].Code)

		// Test content without blocked keywords
		cleanContent := rapid.StringMatching(`[0-9]{20,30}`).Draw(rt, "cleanContent")
		result, err = validator.Validate(ctx, cleanContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Content without keywords should pass")
	})
}

// TestProperty_KeywordValidator_CaseInsensitive tests case insensitive matching
func TestProperty_KeywordValidator_CaseInsensitive(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		keyword := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "keyword")

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: []string{keyword},
			Action:          KeywordActionReject,
			CaseSensitive:   false,
		})

		ctx := context.Background()

		// Test uppercase version
		upperContent := "text " + strings.ToUpper(keyword) + " text"
		result, err := validator.Validate(ctx, upperContent)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect uppercase keyword")

		// Test mixed case
		mixedCase := strings.ToUpper(keyword[:len(keyword)/2]) + keyword[len(keyword)/2:]
		mixedContent := "text " + mixedCase + " text"
		result, err = validator.Validate(ctx, mixedContent)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect mixed case keyword")
	})
}

// TestProperty_KeywordValidator_FilterMode tests keyword filtering
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

		// Filter mode should not invalidate
		assert.True(t, result.Valid, "Filter mode should not invalidate")

		// Check filtered content
		filtered, ok := result.Metadata["filtered_content"].(string)
		require.True(t, ok, "Should have filtered_content")
		assert.NotContains(t, filtered, keyword, "Filtered content should not contain keyword")
		assert.Contains(t, filtered, replacement, "Filtered content should contain replacement")
	})
}

// TestProperty_KeywordValidator_SeverityMapping tests keyword severity mapping
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

// TestProperty_KeywordValidator_MultipleKeywords tests multiple keyword detection
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

		// Content with multiple keywords
		content := keywords[0] + " and " + keywords[1] + " and " + keywords[2]
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid)

		// Check metadata for keyword count
		count, ok := result.Metadata["keyword_count"].(int)
		require.True(t, ok, "Should have keyword_count")
		assert.GreaterOrEqual(t, count, 3, "Should detect all keywords")
	})
}

// TestProperty_KeywordValidator_AddRemoveKeyword tests dynamic keyword management
func TestProperty_KeywordValidator_AddRemoveKeyword(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		initialKeyword := rapid.StringMatching(`[a-z]{5,8}`).Draw(rt, "initialKeyword")
		newKeyword := rapid.StringMatching(`[a-z]{5,8}`).Draw(rt, "newKeyword")

		validator := NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: []string{initialKeyword},
			Action:          KeywordActionReject,
		})

		ctx := context.Background()

		// Initial keyword should be detected
		result, _ := validator.Validate(ctx, initialKeyword)
		assert.False(t, result.Valid, "Initial keyword should be detected")

		// Add new keyword
		validator.AddKeyword(newKeyword, SeverityHigh)
		result, _ = validator.Validate(ctx, newKeyword)
		assert.False(t, result.Valid, "New keyword should be detected")

		// Remove initial keyword
		validator.RemoveKeyword(initialKeyword)
		result, _ = validator.Validate(ctx, initialKeyword)
		assert.True(t, result.Valid, "Removed keyword should not be detected")
	})
}
