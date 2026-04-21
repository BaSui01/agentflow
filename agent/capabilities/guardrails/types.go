package guardrails

import (
	"context"
	"fmt"
)

// Validator 验证器接口
// 用于验证输入或输出内容的安全性和合规性
type Validator interface {
	// Validate 执行验证，返回验证结果
	Validate(ctx context.Context, content string) (*ValidationResult, error)
	// Name 返回验证器名称
	Name() string
	// Priority 返回优先级（数字越小优先级越高）
	Priority() int
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Tripwire bool              `json:"tripwire,omitempty"` // 触发即中断整个 Agent 执行链
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// NewValidationResult 创建一个有效的验证结果
func NewValidationResult() *ValidationResult {
	return &ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []string{},
		Metadata: make(map[string]any),
	}
}

// AddError 添加验证错误并将结果标记为无效
func (r *ValidationResult) AddError(err ValidationError) {
	r.Valid = false
	r.Errors = append(r.Errors, err)
}

// AddWarning 添加警告信息
func (r *ValidationResult) AddWarning(warning string) {
	r.Warnings = append(r.Warnings, warning)
}

// Merge 合并另一个验证结果
func (r *ValidationResult) Merge(other *ValidationResult) {
	if other == nil {
		return
	}
	if !other.Valid {
		r.Valid = false
	}
	if other.Tripwire {
		r.Tripwire = true
	}
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
	for k, v := range other.Metadata {
		r.Metadata[k] = v
	}
}

// ValidationError 验证错误
type ValidationError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // critical, high, medium, low
	Field    string `json:"field,omitempty"`
}

// Severity 常量定义
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// Error 错误代码常量
const (
	ErrCodeInjectionDetected = "INJECTION_DETECTED"
	ErrCodePIIDetected       = "PII_DETECTED"
	ErrCodeMaxLengthExceeded = "MAX_LENGTH_EXCEEDED"
	ErrCodeBlockedKeyword    = "BLOCKED_KEYWORD"
	ErrCodeContentBlocked    = "CONTENT_BLOCKED"
	ErrCodeValidationFailed  = "VALIDATION_FAILED"
)

// TripwireError 表示 Tripwire 被触发的错误。
// 当验证器返回 Tripwire=true 时，整个 Agent 执行链应立即中断。
type TripwireError struct {
	ValidatorName string
	Result        *ValidationResult
}

// Error 实现 error 接口
func (e *TripwireError) Error() string {
	return fmt.Sprintf("tripwire triggered by validator %q", e.ValidatorName)
}

// Filter 过滤器接口
// 用于过滤和转换内容
type Filter interface {
	// Filter 执行过滤，返回过滤后的内容
	Filter(ctx context.Context, content string) (string, error)
	// Name 返回过滤器名称
	Name() string
}

// GuardrailsConfig 护栏配置
type GuardrailsConfig struct {
	InputValidators  []Validator `json:"-"`
	OutputValidators []Validator `json:"-"`
	OutputFilters    []Filter    `json:"-"`

	// 内置验证器配置
	MaxInputLength      int      `json:"max_input_length"`
	BlockedKeywords     []string `json:"blocked_keywords"`
	PIIDetectionEnabled bool     `json:"pii_detection_enabled"`
	InjectionDetection  bool     `json:"injection_detection"`

	// 失败处理
	OnInputFailure  FailureAction `json:"on_input_failure"`
	OnOutputFailure FailureAction `json:"on_output_failure"`
	MaxRetries      int           `json:"max_retries"`
}

// FailureAction 失败处理动作
type FailureAction string

const (
	// FailureActionReject 拒绝请求
	FailureActionReject FailureAction = "reject"
	// FailureActionWarn 发出警告但继续处理
	FailureActionWarn FailureAction = "warn"
	// FailureActionRetry 重试请求
	FailureActionRetry FailureAction = "retry"
)

// DefaultConfig 返回默认配置
func DefaultConfig() *GuardrailsConfig {
	return &GuardrailsConfig{
		InputValidators:     []Validator{},
		OutputValidators:    []Validator{},
		OutputFilters:       []Filter{},
		MaxInputLength:      10000,
		BlockedKeywords:     []string{},
		PIIDetectionEnabled: false,
		InjectionDetection:  false,
		OnInputFailure:      FailureActionReject,
		OnOutputFailure:     FailureActionReject,
		MaxRetries:          0,
	}
}

// ValidatorRegistry 验证器注册表
// 支持自定义验证规则的注册和扩展 (Requirements 1.7)
type ValidatorRegistry struct {
	validators map[string]Validator
}

// NewValidatorRegistry 创建验证器注册表
func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make(map[string]Validator),
	}
}

// Register 注册验证器
func (r *ValidatorRegistry) Register(v Validator) {
	r.validators[v.Name()] = v
}

// Unregister 注销验证器
func (r *ValidatorRegistry) Unregister(name string) {
	delete(r.validators, name)
}

// Get 获取验证器
func (r *ValidatorRegistry) Get(name string) (Validator, bool) {
	v, ok := r.validators[name]
	return v, ok
}

// List 列出所有验证器
func (r *ValidatorRegistry) List() []Validator {
	result := make([]Validator, 0, len(r.validators))
	for _, v := range r.validators {
		result = append(result, v)
	}
	return result
}

// FilterRegistry 过滤器注册表
type FilterRegistry struct {
	filters map[string]Filter
}

// NewFilterRegistry 创建过滤器注册表
func NewFilterRegistry() *FilterRegistry {
	return &FilterRegistry{
		filters: make(map[string]Filter),
	}
}

// Register 注册过滤器
func (r *FilterRegistry) Register(f Filter) {
	r.filters[f.Name()] = f
}

// Unregister 注销过滤器
func (r *FilterRegistry) Unregister(name string) {
	delete(r.filters, name)
}

// Get 获取过滤器
func (r *FilterRegistry) Get(name string) (Filter, bool) {
	f, ok := r.filters[name]
	return f, ok
}

// List 列出所有过滤器
func (r *FilterRegistry) List() []Filter {
	result := make([]Filter, 0, len(r.filters))
	for _, f := range r.filters {
		result = append(result, f)
	}
	return result
}
