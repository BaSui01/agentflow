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

// Feature: agent-framework-2026-enhancements, Property 1: Input Validation Detection
// Validates: Requirements 1.2 - PII Detection
// This property test verifies that PII detector correctly identifies PII in any input.
func TestProperty_PIIDetector_InputValidationDetection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random phone number (Chinese format)
		phonePrefix := rapid.SampledFrom([]string{"13", "14", "15", "16", "17", "18", "19"}).Draw(rt, "phonePrefix")
		phoneSuffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "phoneSuffix")
		phone := phonePrefix + phoneSuffix

		// Generate random email
		emailUser := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "emailUser")
		emailDomain := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "emailDomain")
		email := emailUser + "@" + emailDomain + ".com"

		// Generate surrounding text
		prefix := rapid.StringMatching(`[a-zA-Z\x{4e00}-\x{9fa5}]{0,20}`).Draw(rt, "prefix")
		suffix := rapid.StringMatching(`[a-zA-Z\x{4e00}-\x{9fa5}]{0,20}`).Draw(rt, "suffix")

		detector := NewPIIDetector(&PIIDetectorConfig{
			Action:       PIIActionReject,
			EnabledTypes: []PIIType{PIITypePhone, PIITypeEmail},
		})

		ctx := context.Background()

		// Test phone detection
		contentWithPhone := prefix + phone + suffix
		result, err := detector.Validate(ctx, contentWithPhone)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect phone number: %s", phone)
		assert.NotEmpty(t, result.Errors, "Should have validation errors for phone")

		// Test email detection
		contentWithEmail := prefix + email + suffix
		result, err = detector.Validate(ctx, contentWithEmail)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect email: %s", email)
		assert.NotEmpty(t, result.Errors, "Should have validation errors for email")

		// Test content without PII
		cleanContent := rapid.StringMatching(`[a-zA-Z\x{4e00}-\x{9fa5}]{10,50}`).Draw(rt, "cleanContent")
		result, err = detector.Validate(ctx, cleanContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Should pass for clean content: %s", cleanContent)
	})
}

// Feature: agent-framework-2026-enhancements, Property 5: Output Sensitive Information Masking
// Validates: Requirements 2.1 - Detect and mask sensitive information
// This property test verifies that PII masking preserves content structure while hiding PII.
func TestProperty_PIIDetector_OutputSensitiveInfoMasking(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random phone number
		phonePrefix := rapid.SampledFrom([]string{"13", "14", "15", "16", "17", "18", "19"}).Draw(rt, "phonePrefix")
		phoneSuffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "phoneSuffix")
		phone := phonePrefix + phoneSuffix

		// Generate random email
		emailUser := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "emailUser")
		emailDomain := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "emailDomain")
		email := emailUser + "@" + emailDomain + ".com"

		detector := NewPIIDetector(&PIIDetectorConfig{
			Action:       PIIActionMask,
			EnabledTypes: []PIIType{PIITypePhone, PIITypeEmail},
		})

		ctx := context.Background()

		// Test phone masking
		contentWithPhone := "联系电话：" + phone
		result, err := detector.Validate(ctx, contentWithPhone)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Mask mode should not invalidate result")

		maskedContent, ok := result.Metadata["masked_content"].(string)
		require.True(t, ok, "Should have masked_content in metadata")
		assert.NotContains(t, maskedContent, phone, "Masked content should not contain original phone")
		assert.Contains(t, maskedContent, "****", "Masked content should contain mask characters")
		assert.Contains(t, maskedContent, phone[:3], "Masked phone should preserve first 3 digits")
		assert.Contains(t, maskedContent, phone[len(phone)-4:], "Masked phone should preserve last 4 digits")

		// Test email masking
		contentWithEmail := "邮箱地址：" + email
		result, err = detector.Validate(ctx, contentWithEmail)
		require.NoError(t, err)

		maskedContent, ok = result.Metadata["masked_content"].(string)
		require.True(t, ok, "Should have masked_content in metadata")
		assert.NotContains(t, maskedContent, email, "Masked content should not contain original email")
		assert.Contains(t, maskedContent, "***@", "Masked email should contain mask pattern")
		assert.Contains(t, maskedContent, emailDomain+".com", "Masked email should preserve domain")
	})
}

// TestProperty_PIIDetector_MaskingPreservesLength verifies masking maintains reasonable length
func TestProperty_PIIDetector_MaskingPreservesLength(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate content with multiple PII
		phonePrefix := rapid.SampledFrom([]string{"13", "15", "18"}).Draw(rt, "phonePrefix")
		phoneSuffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "phoneSuffix")
		phone := phonePrefix + phoneSuffix

		content := fmt.Sprintf("用户手机号是%s，请联系", phone)

		detector := NewPIIDetector(&PIIDetectorConfig{
			EnabledTypes: []PIIType{PIITypePhone},
		})

		masked := detector.Mask(content)

		// Masked content should have similar length (within reasonable bounds)
		originalLen := len([]rune(content))
		maskedLen := len([]rune(masked))

		// Allow some variance due to mask characters
		assert.InDelta(t, originalLen, maskedLen, 5, "Masked length should be similar to original")
		assert.NotEqual(t, content, masked, "Content should be modified")
	})
}

// TestProperty_PIIDetector_DetectAllTypes verifies all PII types are detected
func TestProperty_PIIDetector_DetectAllTypes(t *testing.T) {
	testCases := []struct {
		name     string
		piiType  PIIType
		generate func(*rapid.T) string
	}{
		{
			name:    "Phone",
			piiType: PIITypePhone,
			generate: func(rt *rapid.T) string {
				prefix := rapid.SampledFrom([]string{"13", "14", "15", "16", "17", "18", "19"}).Draw(rt, "prefix")
				suffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "suffix")
				return prefix + suffix
			},
		},
		{
			name:    "Email",
			piiType: PIITypeEmail,
			generate: func(rt *rapid.T) string {
				user := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "user")
				domain := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "domain")
				return user + "@" + domain + ".com"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				pii := tc.generate(rt)
				detector := NewPIIDetector(&PIIDetectorConfig{
					EnabledTypes: []PIIType{tc.piiType},
				})

				matches := detector.Detect(pii)
				assert.NotEmpty(t, matches, "Should detect %s: %s", tc.name, pii)
				assert.Equal(t, tc.piiType, matches[0].Type, "Should be correct PII type")
			})
		})
	}
}

// TestProperty_PIIDetector_NoFalsePositives verifies clean content passes validation
func TestProperty_PIIDetector_NoFalsePositives(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate content that should NOT be detected as PII
		// Avoid patterns that look like phone numbers or emails
		words := make([]string, rapid.IntRange(3, 10).Draw(rt, "wordCount"))
		for i := range words {
			words[i] = rapid.StringMatching(`[a-zA-Z]{3,8}`).Draw(rt, fmt.Sprintf("word_%d", i))
		}
		content := strings.Join(words, " ")

		detector := NewPIIDetector(&PIIDetectorConfig{
			Action: PIIActionReject,
		})

		ctx := context.Background()
		result, err := detector.Validate(ctx, content)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Clean content should pass: %s", content)
	})
}
