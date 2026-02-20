package guardrails

import (
	"context"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ChainMode 验证器链执行模式
type ChainMode string

const (
	// ChainModeFailFast 快速失败模式：遇到第一个错误立即停止
	ChainModeFailFast ChainMode = "fail_fast"
	// ChainModeCollectAll 收集全部模式：执行所有验证器并收集所有结果
	ChainModeCollectAll ChainMode = "collect_all"
	// ChainModeParallel 并行模式：并行执行所有验证器并收集结果
	ChainModeParallel ChainMode = "parallel"
)

// ValidatorChainConfig 验证器链配置
type ValidatorChainConfig struct {
	// Mode 执行模式
	Mode ChainMode
}

// DefaultValidatorChainConfig 返回默认配置
func DefaultValidatorChainConfig() *ValidatorChainConfig {
	return &ValidatorChainConfig{
		Mode: ChainModeCollectAll,
	}
}

// ValidatorChain 验证器链
// 按优先级顺序执行多个验证器并聚合结果
// Requirements 1.5: 按优先级顺序执行所有规则
// Requirements 1.6: 返回包含失败原因的详细错误信息
type ValidatorChain struct {
	validators []Validator
	mode       ChainMode
	mu         sync.RWMutex
}

// NewValidatorChain 创建验证器链
func NewValidatorChain(config *ValidatorChainConfig) *ValidatorChain {
	if config == nil {
		config = DefaultValidatorChainConfig()
	}

	return &ValidatorChain{
		validators: make([]Validator, 0),
		mode:       config.Mode,
	}
}

// Name 返回验证器链名称
func (c *ValidatorChain) Name() string {
	return "validator_chain"
}

// Priority 返回验证器链优先级（作为整体的优先级）
func (c *ValidatorChain) Priority() int {
	return 0 // 链本身优先级最高
}

// Add 添加验证器到链中
func (c *ValidatorChain) Add(validators ...Validator) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.validators = append(c.validators, validators...)
}

// Remove 从链中移除指定名称的验证器
func (c *ValidatorChain) Remove(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, v := range c.validators {
		if v.Name() == name {
			c.validators = append(c.validators[:i], c.validators[i+1:]...)
			return true
		}
	}
	return false
}

// Clear 清空所有验证器
func (c *ValidatorChain) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.validators = make([]Validator, 0)
}

// Validators 返回按优先级排序的验证器列表
func (c *ValidatorChain) Validators() []Validator {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 复制并排序
	sorted := make([]Validator, len(c.validators))
	copy(sorted, c.validators)
	sortValidatorsByPriority(sorted)

	return sorted
}

// Len 返回验证器数量
func (c *ValidatorChain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.validators)
}

// SetMode 设置执行模式
func (c *ValidatorChain) SetMode(mode ChainMode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mode = mode
}

// GetMode 获取当前执行模式
func (c *ValidatorChain) GetMode() ChainMode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.mode
}

// Validate 执行验证器链
// 按优先级顺序执行所有验证器，聚合验证结果
// 实现 Validator 接口
func (c *ValidatorChain) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	c.mu.RLock()
	validators := make([]Validator, len(c.validators))
	copy(validators, c.validators)
	mode := c.mode
	c.mu.RUnlock()

	// 并行模式走独立路径
	if mode == ChainModeParallel {
		return c.validateParallel(ctx, validators, content)
	}

	// 按优先级排序（数字越小优先级越高）
	sortValidatorsByPriority(validators)

	// 创建聚合结果
	result := NewValidationResult()
	result.Metadata["validators_executed"] = make([]string, 0)
	result.Metadata["execution_order"] = make([]string, 0)

	// 按顺序执行验证器
	for _, v := range validators {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			result.AddError(ValidationError{
				Code:     ErrCodeValidationFailed,
				Message:  "验证被取消: " + ctx.Err().Error(),
				Severity: SeverityMedium,
			})
			return result, ctx.Err()
		default:
		}

		// 记录执行顺序
		executionOrder := result.Metadata["execution_order"].([]string)
		result.Metadata["execution_order"] = append(executionOrder, v.Name())

		// 执行验证
		vResult, err := v.Validate(ctx, content)
		if err != nil {
			result.AddError(ValidationError{
				Code:     ErrCodeValidationFailed,
				Message:  "验证器 " + v.Name() + " 执行失败: " + err.Error(),
				Severity: SeverityCritical,
			})

			// 快速失败模式下立即返回
			if mode == ChainModeFailFast {
				return result, err
			}
			continue
		}

		// 记录已执行的验证器
		executed := result.Metadata["validators_executed"].([]string)
		result.Metadata["validators_executed"] = append(executed, v.Name())

		// Tripwire 优先级高于模式：立即中断
		if vResult.Tripwire {
			result.Merge(vResult)
			return result, &TripwireError{
				ValidatorName: v.Name(),
				Result:        result,
			}
		}

		// 合并结果
		result.Merge(vResult)

		// 快速失败模式：遇到验证失败立即停止
		if mode == ChainModeFailFast && !vResult.Valid {
			return result, nil
		}
	}

	return result, nil
}

// validateParallel 并行执行所有验证器并收集结果。
// 如果任何验证器返回 Tripwire，通过 context cancel 取消其他验证器。
func (c *ValidatorChain) validateParallel(ctx context.Context, validators []Validator, content string) (*ValidationResult, error) {
	if len(validators) == 0 {
		result := NewValidationResult()
		result.Metadata["validators_executed"] = make([]string, 0)
		result.Metadata["execution_order"] = make([]string, 0)
		return result, nil
	}

	type validatorResult struct {
		name   string
		result *ValidationResult
		err    error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]validatorResult, len(validators))
	g, gctx := errgroup.WithContext(ctx)

	// tripwire 信号：一旦触发就取消所有其他验证器
	var tripwireOnce sync.Once
	var tripwireName string

	for i, v := range validators {
		i, v := i, v
		g.Go(func() error {
			vResult, err := v.Validate(gctx, content)
			results[i] = validatorResult{
				name:   v.Name(),
				result: vResult,
				err:    err,
			}
			// 检测 Tripwire 并取消其他验证器
			if err == nil && vResult != nil && vResult.Tripwire {
				tripwireOnce.Do(func() {
					tripwireName = v.Name()
					cancel()
				})
			}
			return nil // 不让 errgroup 提前终止，我们自己收集所有结果
		})
	}

	// 等待所有 goroutine 完成
	_ = g.Wait()

	// 聚合结果
	result := NewValidationResult()
	executed := make([]string, 0, len(validators))

	for _, vr := range results {
		if vr.err != nil {
			result.AddError(ValidationError{
				Code:     ErrCodeValidationFailed,
				Message:  "验证器 " + vr.name + " 执行失败: " + vr.err.Error(),
				Severity: SeverityCritical,
			})
			continue
		}
		if vr.result == nil {
			// 验证器被取消且未产生结果
			continue
		}
		executed = append(executed, vr.name)
		result.Merge(vr.result)
	}

	result.Metadata["validators_executed"] = executed
	result.Metadata["execution_order"] = executed // 并行模式下顺序不确定

	if tripwireName != "" {
		return result, &TripwireError{
			ValidatorName: tripwireName,
			Result:        result,
		}
	}

	return result, nil
}

// ValidateWithCallback 执行验证器链，支持回调
// callback 在每个验证器执行后被调用，返回 false 可中断执行
func (c *ValidatorChain) ValidateWithCallback(
	ctx context.Context,
	content string,
	callback func(validator Validator, result *ValidationResult) bool,
) (*ValidationResult, error) {
	c.mu.RLock()
	validators := make([]Validator, len(c.validators))
	copy(validators, c.validators)
	c.mu.RUnlock()

	// 按优先级排序
	sortValidatorsByPriority(validators)

	// 创建聚合结果
	result := NewValidationResult()
	result.Metadata["validators_executed"] = make([]string, 0)
	result.Metadata["execution_order"] = make([]string, 0)

	for _, v := range validators {
		// 检查上下文
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// 记录执行顺序
		executionOrder := result.Metadata["execution_order"].([]string)
		result.Metadata["execution_order"] = append(executionOrder, v.Name())

		// 执行验证
		vResult, err := v.Validate(ctx, content)
		if err != nil {
			result.AddError(ValidationError{
				Code:     ErrCodeValidationFailed,
				Message:  "验证器 " + v.Name() + " 执行失败: " + err.Error(),
				Severity: SeverityCritical,
			})
			continue
		}

		// 记录已执行的验证器
		executed := result.Metadata["validators_executed"].([]string)
		result.Metadata["validators_executed"] = append(executed, v.Name())

		// 合并结果
		result.Merge(vResult)

		// 调用回调
		if callback != nil && !callback(v, vResult) {
			break
		}
	}

	return result, nil
}

// sortValidatorsByPriority 按优先级排序验证器（数字越小优先级越高）
func sortValidatorsByPriority(validators []Validator) {
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].Priority() < validators[j].Priority()
	})
}
