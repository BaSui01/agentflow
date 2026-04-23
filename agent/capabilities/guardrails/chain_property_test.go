package guardrails

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// 特征:代理-框架-2026-增强,财产3:验证人优先执行令
// 审定:要求1.5 - 按优先顺序执行所有规则
// 此属性测试可验证验证器按优先级执行 。
func TestProperty_ValidatorChain_PriorityExecutionOrder(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机优先级
		numValidators := rapid.IntRange(2, 5).Draw(rt, "numValidators")
		priorities := make([]int, numValidators)
		for i := range priorities {
			priorities[i] = rapid.IntRange(1, 100).Draw(rt, fmt.Sprintf("priority_%d", i))
		}

		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// 添加具有不同优先级的验证符
		for i, priority := range priorities {
			chain.Add(&propMockValidator{
				name:     fmt.Sprintf("validator_%d", i),
				priority: priority,
			})
		}

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)

		// 从元数据检查执行命令
		executionOrder, ok := result.Metadata["execution_order"].([]string)
		require.True(t, ok, "Should have execution_order in metadata")
		require.Len(t, executionOrder, numValidators, "All validators should be executed")

		// 校验验证符按优先级排序
		validators := chain.Validators()
		for i := 0; i < len(validators)-1; i++ {
			assert.LessOrEqual(t, validators[i].Priority(), validators[i+1].Priority(),
				"Validators should be sorted by priority")
		}
	})
}

// 特性:代理框架-2026-增强,属性4:验证错误信息完整性
// 校验: 要求1.6 - 以失败原因返回详细错误信息
// 此属性测试验证验证错误包含完整信息 。
func TestProperty_ValidatorChain_ErrorInformationCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// 添加将失败的验证符
		errorCode := rapid.SampledFrom([]string{
			ErrCodeInjectionDetected,
			ErrCodePIIDetected,
			ErrCodeMaxLengthExceeded,
			ErrCodeBlockedKeyword,
		}).Draw(rt, "errorCode")

		errorMessage := rapid.StringMatching(`[a-zA-Z ]{10,50}`).Draw(rt, "errorMessage")
		severity := rapid.SampledFrom([]string{
			SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow,
		}).Draw(rt, "severity")

		chain.Add(&propMockFailingValidator{
			name:         "failing_validator",
			priority:     10,
			errorCode:    errorCode,
			errorMessage: errorMessage,
			severity:     severity,
		})

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should be invalid when validator fails")

		// 校验出错的完整性
		require.NotEmpty(t, result.Errors, "Should have errors")
		validationErr := result.Errors[0]
		assert.Equal(t, errorCode, validationErr.Code, "Error code should match")
		assert.Equal(t, errorMessage, validationErr.Message, "Error message should match")
		assert.Equal(t, severity, validationErr.Severity, "Severity should match")
	})
}

// 测试Property ValidatorChan FailFastMode 测试失败-快速执行模式
func TestProperty_ValidatorChain_FailFastMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeFailFast,
		})

		// 添加多个验证符, 第一个失败
		chain.Add(&propMockFailingValidator{
			name:         "first_failing",
			priority:     10,
			errorCode:    ErrCodeValidationFailed,
			errorMessage: "First failure",
			severity:     SeverityHigh,
		})
		chain.Add(&propMockValidator{
			name:     "second_validator",
			priority: 20,
		})
		chain.Add(&propMockValidator{
			name:     "third_validator",
			priority: 30,
		})

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)
		assert.False(t, result.Valid)

		// 在故障快模式下,在第一次故障后应停止
		executionOrder, ok := result.Metadata["execution_order"].([]string)
		require.True(t, ok)
		assert.Len(t, executionOrder, 1, "Should stop after first failure in fail-fast mode")
	})
}

// 测试Property  ValidatorChan  Collect AllMode 测试收集全部执行模式
func TestProperty_ValidatorChain_CollectAllMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		numValidators := rapid.IntRange(2, 5).Draw(rt, "numValidators")
		failingIndex := rapid.IntRange(0, numValidators-1).Draw(rt, "failingIndex")

		for i := 0; i < numValidators; i++ {
			if i == failingIndex {
				chain.Add(&propMockFailingValidator{
					name:         fmt.Sprintf("validator_%d", i),
					priority:     i * 10,
					errorCode:    ErrCodeValidationFailed,
					errorMessage: "Failure",
					severity:     SeverityMedium,
				})
			} else {
				chain.Add(&propMockValidator{
					name:     fmt.Sprintf("validator_%d", i),
					priority: i * 10,
				})
			}
		}

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)

		// 在收集- 全部模式下, 所有验证符都应该执行
		executionOrder, ok := result.Metadata["execution_order"].([]string)
		require.True(t, ok)
		assert.Len(t, executionOrder, numValidators, "All validators should be executed in collect-all mode")
	})
}

// 测试结果合并
func TestProperty_ValidatorChain_MergesResults(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// 添加多个失败验证符
		numFailures := rapid.IntRange(2, 4).Draw(rt, "numFailures")
		for i := 0; i < numFailures; i++ {
			chain.Add(&propMockFailingValidator{
				name:         fmt.Sprintf("failing_%d", i),
				priority:     i * 10,
				errorCode:    fmt.Sprintf("ERROR_%d", i),
				errorMessage: fmt.Sprintf("Error message %d", i),
				severity:     SeverityMedium,
			})
		}

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)
		assert.False(t, result.Valid)

		// 应收集所有错误
		assert.Len(t, result.Errors, numFailures, "Should collect all errors")
	})
}

// 测试Property  Validator Chain Add 移动参数测试动态验证器管理
func TestProperty_ValidatorChain_AddRemoveValidators(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(nil)

		// 添加验证符
		numToAdd := rapid.IntRange(2, 5).Draw(rt, "numToAdd")
		names := make([]string, numToAdd)
		for i := 0; i < numToAdd; i++ {
			names[i] = fmt.Sprintf("validator_%d", i)
			chain.Add(&propMockValidator{
				name:     names[i],
				priority: i * 10,
			})
		}

		assert.Equal(t, numToAdd, chain.Len(), "Should have correct number of validators")

		// 删除一个校验符
		removeIndex := rapid.IntRange(0, numToAdd-1).Draw(rt, "removeIndex")
		removed := chain.Remove(names[removeIndex])
		assert.True(t, removed, "Should successfully remove validator")
		assert.Equal(t, numToAdd-1, chain.Len(), "Should have one less validator")

		// 全部清除
		chain.Clear()
		assert.Equal(t, 0, chain.Len(), "Should be empty after clear")
	})
}

// 测试Property ValidatorChan ContextConcellation 测试上下文取消处理
func TestProperty_ValidatorChain_ContextCancellation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// 添加验证符
		for i := 0; i < 3; i++ {
			chain.Add(&propMockValidator{
				name:     fmt.Sprintf("validator_%d", i),
				priority: i * 10,
			})
		}

		// 创建已取消的上下文
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result, err := chain.Validate(ctx, "test content")
		assert.Error(t, err, "Should return error for cancelled context")
		assert.NotNil(t, result, "Should still return result")
	})
}

// PropMockValidator 是用于属性测试的简单验证器
type propMockValidator struct {
	name     string
	priority int
}

func (v *propMockValidator) Name() string  { return v.name }
func (v *propMockValidator) Priority() int { return v.priority }
func (v *propMockValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	return NewValidationResult(), nil
}

// PropMock FailingValidator 是属性测试总是失败的验证器
type propMockFailingValidator struct {
	name         string
	priority     int
	errorCode    string
	errorMessage string
	severity     string
}

func (v *propMockFailingValidator) Name() string  { return v.name }
func (v *propMockFailingValidator) Priority() int { return v.priority }
func (v *propMockFailingValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()
	result.AddError(ValidationError{
		Code:     v.errorCode,
		Message:  v.errorMessage,
		Severity: v.severity,
	})
	return result, nil
}
