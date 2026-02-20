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

// 特性:代理框架-2026-增强,属性1:输入验证检测
// 验证:要求1.2 - PII检测
// 这个属性测试验证PII探测器在任何输入中正确识别出PII.
func TestProperty_PIIDetector_InputValidationDetection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机电话号码 (中文格式)
		phonePrefix := rapid.SampledFrom([]string{"13", "14", "15", "16", "17", "18", "19"}).Draw(rt, "phonePrefix")
		phoneSuffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "phoneSuffix")
		phone := phonePrefix + phoneSuffix

		// 生成随机电子邮件
		emailUser := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "emailUser")
		emailDomain := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "emailDomain")
		email := emailUser + "@" + emailDomain + ".com"

		// 生成周围文本
		prefix := rapid.StringMatching(`[a-zA-Z\x{4e00}-\x{9fa5}]{0,20}`).Draw(rt, "prefix")
		suffix := rapid.StringMatching(`[a-zA-Z\x{4e00}-\x{9fa5}]{0,20}`).Draw(rt, "suffix")

		detector := NewPIIDetector(&PIIDetectorConfig{
			Action:       PIIActionReject,
			EnabledTypes: []PIIType{PIITypePhone, PIITypeEmail},
		})

		ctx := context.Background()

		// 测试电话检测
		contentWithPhone := prefix + phone + suffix
		result, err := detector.Validate(ctx, contentWithPhone)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect phone number: %s", phone)
		assert.NotEmpty(t, result.Errors, "Should have validation errors for phone")

		// 测试电子邮件检测
		contentWithEmail := prefix + email + suffix
		result, err = detector.Validate(ctx, contentWithEmail)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect email: %s", email)
		assert.NotEmpty(t, result.Errors, "Should have validation errors for email")

		// 测试内容无 PII
		cleanContent := rapid.StringMatching(`[a-zA-Z\x{4e00}-\x{9fa5}]{10,50}`).Draw(rt, "cleanContent")
		result, err = detector.Validate(ctx, cleanContent)
		require.NoError(t, err)
		assert.True(t, result.Valid, "Should pass for clean content: %s", cleanContent)
	})
}

// 特性:代理框架2026-增强,财产5:输出敏感信息遮盖
// 验证:要求2.1 - 检测和掩盖敏感信息
// 这个属性测试验证PII遮掩了内容结构同时隐藏了PII.
func TestProperty_PIIDetector_OutputSensitiveInfoMasking(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机电话号码
		phonePrefix := rapid.SampledFrom([]string{"13", "14", "15", "16", "17", "18", "19"}).Draw(rt, "phonePrefix")
		phoneSuffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "phoneSuffix")
		phone := phonePrefix + phoneSuffix

		// 生成随机电子邮件
		emailUser := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "emailUser")
		emailDomain := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "emailDomain")
		email := emailUser + "@" + emailDomain + ".com"

		detector := NewPIIDetector(&PIIDetectorConfig{
			Action:       PIIActionMask,
			EnabledTypes: []PIIType{PIITypePhone, PIITypeEmail},
		})

		ctx := context.Background()

		// 测试电话口罩
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

		// 测试电子邮件遮盖
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

// Property PII 检测器  Masking PreservesLength 验证掩码保持合理长度
func TestProperty_PIIDetector_MaskingPreservesLength(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成包含多个 PII 的内容
		phonePrefix := rapid.SampledFrom([]string{"13", "15", "18"}).Draw(rt, "phonePrefix")
		phoneSuffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "phoneSuffix")
		phone := phonePrefix + phoneSuffix

		content := fmt.Sprintf("用户手机号是%s，请联系", phone)

		detector := NewPIIDetector(&PIIDetectorConfig{
			EnabledTypes: []PIIType{PIITypePhone},
		})

		masked := detector.Mask(content)

		// 被遮盖的内容应具有相近长度(在合理范围内)
		originalLen := len([]rune(content))
		maskedLen := len([]rune(masked))

		// 允许由于口罩字符而出现一些差异
		assert.InDelta(t, originalLen, maskedLen, 5, "Masked length should be similar to original")
		assert.NotEqual(t, content, masked, "Content should be modified")
	})
}

// 检测所有 PII 类型
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

// 测试 Property PII 检测器  NoFasePosients 验证干净的内容通过验证
func TestProperty_PIIDetector_NoFalsePositives(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成不应检测为 PII 的内容
		// 避免看起来像电话号码或电子邮件的模式
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
