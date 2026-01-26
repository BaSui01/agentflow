package guardrails

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockValidator 用于测试的模拟验证器
type mockValidator struct {
	name       string
	priority   int
	valid      bool
	err        error
	execOrder  *[]string // 用于记录执行顺序
	shouldFail bool
}

func newMockValidator(name string, priority int, valid bool) *mockValidator {
	return &mockValidator{
		name:     name,
		priority: priority,
		valid:    valid,
	}
}

func (m *mockValidator) Name() string {
	return m.name
}

func (m *mockValidator) Priority() int {
	return m.priority
}

func (m *mockValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	// 记录执行顺序
	if m.execOrder != nil {
		*m.execOrder = append(*m.execOrder, m.name)
	}

	if m.err != nil {
		return nil, m.err
	}

	result := NewValidationResult()
	if !m.valid {
		result.AddError(ValidationError{
			Code:     "MOCK_ERROR",
			Message:  "Mock validation failed: " + m.name,
			Severity: SeverityMedium,
		})
	}
	return result, nil
}

func TestNewValidatorChain(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		chain := NewValidatorChain(nil)
		assert.NotNil(t, chain)
		assert.Equal(t, ChainModeCollectAll, chain.GetMode())
		assert.Equal(t, 0, chain.Len())
	})

	t.Run("custom config", func(t *testing.T) {
		config := &ValidatorChainConfig{
			Mode: ChainModeFailFast,
		}
		chain := NewValidatorChain(config)
		assert.Equal(t, ChainModeFailFast, chain.GetMode())
	})
}

func TestValidatorChain_Add(t *testing.T) {
	chain := NewValidatorChain(nil)

	v1 := newMockValidator("v1", 10, true)
	v2 := newMockValidator("v2", 20, true)

	chain.Add(v1)
	assert.Equal(t, 1, chain.Len())

	chain.Add(v2)
	assert.Equal(t, 2, chain.Len())

	// 批量添加
	v3 := newMockValidator("v3", 30, true)
	v4 := newMockValidator("v4", 40, true)
	chain.Add(v3, v4)
	assert.Equal(t, 4, chain.Len())
}

func TestValidatorChain_Remove(t *testing.T) {
	chain := NewValidatorChain(nil)

	v1 := newMockValidator("v1", 10, true)
	v2 := newMockValidator("v2", 20, true)
	chain.Add(v1, v2)

	// 移除存在的验证器
	removed := chain.Remove("v1")
	assert.True(t, removed)
	assert.Equal(t, 1, chain.Len())

	// 移除不存在的验证器
	removed = chain.Remove("v3")
	assert.False(t, removed)
	assert.Equal(t, 1, chain.Len())
}

func TestValidatorChain_Clear(t *testing.T) {
	chain := NewValidatorChain(nil)

	chain.Add(
		newMockValidator("v1", 10, true),
		newMockValidator("v2", 20, true),
	)
	assert.Equal(t, 2, chain.Len())

	chain.Clear()
	assert.Equal(t, 0, chain.Len())
}

func TestValidatorChain_Validators(t *testing.T) {
	chain := NewValidatorChain(nil)

	// 添加乱序的验证器
	chain.Add(
		newMockValidator("v3", 30, true),
		newMockValidator("v1", 10, true),
		newMockValidator("v2", 20, true),
	)

	validators := chain.Validators()
	require.Len(t, validators, 3)

	// 验证按优先级排序
	assert.Equal(t, "v1", validators[0].Name())
	assert.Equal(t, "v2", validators[1].Name())
	assert.Equal(t, "v3", validators[2].Name())
}

func TestValidatorChain_PriorityOrder(t *testing.T) {
	// 测试验证器按优先级顺序执行
	// Requirements 1.5: 按优先级顺序执行所有规则
	chain := NewValidatorChain(nil)

	var execOrder []string

	v1 := newMockValidator("v1", 30, true)
	v1.execOrder = &execOrder

	v2 := newMockValidator("v2", 10, true)
	v2.execOrder = &execOrder

	v3 := newMockValidator("v3", 20, true)
	v3.execOrder = &execOrder

	// 乱序添加
	chain.Add(v1, v2, v3)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	require.NoError(t, err)
	assert.True(t, result.Valid)

	// 验证执行顺序：应该按优先级 10 -> 20 -> 30
	require.Len(t, execOrder, 3)
	assert.Equal(t, "v2", execOrder[0]) // priority 10
	assert.Equal(t, "v3", execOrder[1]) // priority 20
	assert.Equal(t, "v1", execOrder[2]) // priority 30

	// 验证 metadata 中的执行顺序
	executionOrder, ok := result.Metadata["execution_order"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"v2", "v3", "v1"}, executionOrder)
}

func TestValidatorChain_CollectAllMode(t *testing.T) {
	// 测试收集全部模式：执行所有验证器
	chain := NewValidatorChain(&ValidatorChainConfig{
		Mode: ChainModeCollectAll,
	})

	var execOrder []string

	v1 := newMockValidator("v1", 10, false) // 验证失败
	v1.execOrder = &execOrder

	v2 := newMockValidator("v2", 20, true) // 验证成功
	v2.execOrder = &execOrder

	v3 := newMockValidator("v3", 30, false) // 验证失败
	v3.execOrder = &execOrder

	chain.Add(v1, v2, v3)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	require.NoError(t, err)
	assert.False(t, result.Valid)

	// 所有验证器都应该被执行
	assert.Len(t, execOrder, 3)

	// 应该收集所有错误
	assert.Len(t, result.Errors, 2)
}

func TestValidatorChain_FailFastMode(t *testing.T) {
	// 测试快速失败模式：遇到第一个错误立即停止
	chain := NewValidatorChain(&ValidatorChainConfig{
		Mode: ChainModeFailFast,
	})

	var execOrder []string

	v1 := newMockValidator("v1", 10, true) // 验证成功
	v1.execOrder = &execOrder

	v2 := newMockValidator("v2", 20, false) // 验证失败
	v2.execOrder = &execOrder

	v3 := newMockValidator("v3", 30, true) // 不应该被执行
	v3.execOrder = &execOrder

	chain.Add(v1, v2, v3)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	require.NoError(t, err)
	assert.False(t, result.Valid)

	// 只有前两个验证器被执行
	assert.Len(t, execOrder, 2)
	assert.Equal(t, []string{"v1", "v2"}, execOrder)

	// 只有一个错误
	assert.Len(t, result.Errors, 1)
}

func TestValidatorChain_ErrorAggregation(t *testing.T) {
	// 测试错误聚合
	// Requirements 1.6: 返回包含失败原因的详细错误信息
	chain := NewValidatorChain(nil)

	chain.Add(
		newMockValidator("v1", 10, false),
		newMockValidator("v2", 20, false),
		newMockValidator("v3", 30, false),
	)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	require.NoError(t, err)
	assert.False(t, result.Valid)

	// 应该有3个错误
	require.Len(t, result.Errors, 3)

	// 每个错误都应该有完整信息
	for _, e := range result.Errors {
		assert.NotEmpty(t, e.Code)
		assert.NotEmpty(t, e.Message)
		assert.NotEmpty(t, e.Severity)
	}
}

func TestValidatorChain_ValidatorError(t *testing.T) {
	// 测试验证器执行错误
	chain := NewValidatorChain(nil)

	v1 := newMockValidator("v1", 10, true)
	v2 := newMockValidator("v2", 20, true)
	v2.err = errors.New("validator error")
	v3 := newMockValidator("v3", 30, true)

	chain.Add(v1, v2, v3)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	// 收集全部模式下不返回错误
	require.NoError(t, err)
	assert.False(t, result.Valid)

	// 应该有一个错误记录
	require.Len(t, result.Errors, 1)
	assert.Equal(t, ErrCodeValidationFailed, result.Errors[0].Code)
	assert.Contains(t, result.Errors[0].Message, "v2")
}

func TestValidatorChain_ValidatorError_FailFast(t *testing.T) {
	// 测试快速失败模式下的验证器执行错误
	chain := NewValidatorChain(&ValidatorChainConfig{
		Mode: ChainModeFailFast,
	})

	var execOrder []string

	v1 := newMockValidator("v1", 10, true)
	v1.execOrder = &execOrder

	v2 := newMockValidator("v2", 20, true)
	v2.err = errors.New("validator error")
	v2.execOrder = &execOrder

	v3 := newMockValidator("v3", 30, true)
	v3.execOrder = &execOrder

	chain.Add(v1, v2, v3)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	// 快速失败模式下返回错误
	require.Error(t, err)
	assert.False(t, result.Valid)

	// v3 不应该被执行
	assert.Len(t, execOrder, 2)
}

func TestValidatorChain_ContextCancellation(t *testing.T) {
	// 测试上下文取消
	chain := NewValidatorChain(nil)

	var execOrder []string

	v1 := newMockValidator("v1", 10, true)
	v1.execOrder = &execOrder

	v2 := newMockValidator("v2", 20, true)
	v2.execOrder = &execOrder

	chain.Add(v1, v2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	result, err := chain.Validate(ctx, "test content")

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.False(t, result.Valid)
}

func TestValidatorChain_EmptyChain(t *testing.T) {
	// 测试空链
	chain := NewValidatorChain(nil)

	ctx := context.Background()
	result, err := chain.Validate(ctx, "test content")

	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidatorChain_ValidateWithCallback(t *testing.T) {
	chain := NewValidatorChain(nil)

	chain.Add(
		newMockValidator("v1", 10, true),
		newMockValidator("v2", 20, false),
		newMockValidator("v3", 30, true),
	)

	var callbackOrder []string
	ctx := context.Background()

	result, err := chain.ValidateWithCallback(ctx, "test", func(v Validator, r *ValidationResult) bool {
		callbackOrder = append(callbackOrder, v.Name())
		// 在 v2 后停止
		return v.Name() != "v2"
	})

	require.NoError(t, err)
	assert.False(t, result.Valid)

	// 只执行到 v2
	assert.Equal(t, []string{"v1", "v2"}, callbackOrder)
}

func TestValidatorChain_IntegrationWithRealValidators(t *testing.T) {
	// 集成测试：使用真实的验证器
	chain := NewValidatorChain(nil)

	// 添加长度验证器（优先级 10）
	lengthValidator := NewLengthValidator(&LengthValidatorConfig{
		MaxLength: 100,
		Action:    LengthActionReject,
		Priority:  10,
	})

	// 添加关键词验证器（优先级 20）
	keywordValidator := NewKeywordValidator(&KeywordValidatorConfig{
		BlockedKeywords: []string{"forbidden"},
		Action:          KeywordActionReject,
		Priority:        20,
	})

	chain.Add(lengthValidator, keywordValidator)

	ctx := context.Background()

	t.Run("valid content", func(t *testing.T) {
		result, err := chain.Validate(ctx, "This is valid content")
		require.NoError(t, err)
		assert.True(t, result.Valid)
	})

	t.Run("content with forbidden keyword", func(t *testing.T) {
		result, err := chain.Validate(ctx, "This contains forbidden word")
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, ErrCodeBlockedKeyword, result.Errors[0].Code)
	})

	t.Run("content too long", func(t *testing.T) {
		longContent := make([]byte, 150)
		for i := range longContent {
			longContent[i] = 'a'
		}
		result, err := chain.Validate(ctx, string(longContent))
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, ErrCodeMaxLengthExceeded, result.Errors[0].Code)
	})

	t.Run("multiple violations", func(t *testing.T) {
		// 内容太长且包含禁止关键词
		longContent := make([]byte, 150)
		for i := range longContent {
			longContent[i] = 'a'
		}
		content := string(longContent) + " forbidden"

		result, err := chain.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		// 收集全部模式下应该有两个错误
		assert.Len(t, result.Errors, 2)
	})
}

func TestValidatorChain_Name(t *testing.T) {
	chain := NewValidatorChain(nil)
	assert.Equal(t, "validator_chain", chain.Name())
}

func TestValidatorChain_Priority(t *testing.T) {
	chain := NewValidatorChain(nil)
	assert.Equal(t, 0, chain.Priority())
}

func TestValidatorChain_SetMode(t *testing.T) {
	chain := NewValidatorChain(nil)
	assert.Equal(t, ChainModeCollectAll, chain.GetMode())

	chain.SetMode(ChainModeFailFast)
	assert.Equal(t, ChainModeFailFast, chain.GetMode())
}

func TestSortValidatorsByPriority(t *testing.T) {
	validators := []Validator{
		newMockValidator("v3", 30, true),
		newMockValidator("v1", 10, true),
		newMockValidator("v4", 40, true),
		newMockValidator("v2", 20, true),
	}

	sortValidatorsByPriority(validators)

	assert.Equal(t, "v1", validators[0].Name())
	assert.Equal(t, "v2", validators[1].Name())
	assert.Equal(t, "v3", validators[2].Name())
	assert.Equal(t, "v4", validators[3].Name())
}
