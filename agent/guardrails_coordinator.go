// 包代理为AgentFlow提供了核心代理框架.
// 此文件执行 Guardrails 协调员管理输入/输出验证 。
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

// 护栏协调员利用护栏协调输入/输出验证。
// 它囊括了先前在BaseAgent中存在的护栏逻辑.
type GuardrailsCoordinator struct {
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	config              *guardrails.GuardrailsConfig
	enabled             bool
	logger              *zap.Logger
}

// 新护卫组织协调员设立了一个新的护卫组织协调员。
func NewGuardrailsCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *GuardrailsCoordinator {
	gc := &GuardrailsCoordinator{
		config: config,
		logger: logger.With(zap.String("component", "guardrails_coordinator")),
	}

	if config != nil {
		gc.initialize(config)
	}

	return gc
}

// 初始化根据配置设置护栏.
func (gc *GuardrailsCoordinator) initialize(cfg *guardrails.GuardrailsConfig) {
	gc.enabled = true

	// 初始化输入验证链
	gc.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})

	// 添加已配置的输入验证符
	for _, v := range cfg.InputValidators {
		gc.inputValidatorChain.Add(v)
	}

	// 根据配置添加内置验证符
	if cfg.MaxInputLength > 0 {
		gc.inputValidatorChain.Add(guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
			MaxLength: cfg.MaxInputLength,
			Action:    guardrails.LengthActionReject,
		}))
	}

	if len(cfg.BlockedKeywords) > 0 {
		gc.inputValidatorChain.Add(guardrails.NewKeywordValidator(&guardrails.KeywordValidatorConfig{
			BlockedKeywords: cfg.BlockedKeywords,
			CaseSensitive:   false,
		}))
	}

	if cfg.InjectionDetection {
		gc.inputValidatorChain.Add(guardrails.NewInjectionDetector(nil))
	}

	if cfg.PIIDetectionEnabled {
		gc.inputValidatorChain.Add(guardrails.NewPIIDetector(nil))
	}

	// 初始化输出验证符
	outputConfig := &guardrails.OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	}
	gc.outputValidator = guardrails.NewOutputValidator(outputConfig)

	gc.logger.Info("guardrails initialized",
		zap.Int("input_validators", gc.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

// 验证输入验证输入内容.
// 返回验证结果时发现任何错误 。
func (gc *GuardrailsCoordinator) ValidateInput(ctx context.Context, input string) (*guardrails.ValidationResult, error) {
	if !gc.enabled || gc.inputValidatorChain == nil {
		return &guardrails.ValidationResult{Valid: true}, nil
	}
	return gc.inputValidatorChain.Validate(ctx, input)
}

// 校验输出验证和过滤输出内容.
// 返回被过滤的输出和验证结果。
func (gc *GuardrailsCoordinator) ValidateOutput(ctx context.Context, output string) (string, *guardrails.ValidationResult, error) {
	if !gc.enabled || gc.outputValidator == nil {
		return output, &guardrails.ValidationResult{Valid: true}, nil
	}
	return gc.outputValidator.ValidateAndFilter(ctx, output)
}

// 启用是否启用了守护栏 。
func (gc *GuardrailsCoordinator) Enabled() bool {
	return gc.enabled
}

// 设置可启用或禁用护栏 。
func (gc *GuardrailsCoordinator) SetEnabled(enabled bool) {
	gc.enabled = enabled
}

// 添加InputValidator为输入链添加了验证符.
func (gc *GuardrailsCoordinator) AddInputValidator(v guardrails.Validator) {
	if gc.inputValidatorChain == nil {
		gc.inputValidatorChain = guardrails.NewValidatorChain(nil)
		gc.enabled = true
	}
	gc.inputValidatorChain.Add(v)
}

// 添加输出变量在输出验证符中添加一个验证符.
func (gc *GuardrailsCoordinator) AddOutputValidator(v guardrails.Validator) {
	if gc.outputValidator == nil {
		gc.outputValidator = guardrails.NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddValidator(v)
}

// 添加 OutputFilter 为输出验证器添加了过滤器 。
func (gc *GuardrailsCoordinator) AddOutputFilter(f guardrails.Filter) {
	if gc.outputValidator == nil {
		gc.outputValidator = guardrails.NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddFilter(f)
}

// BuildValidationFeed BackMessage为验证失败创建了反馈消息.
// 此消息可以发回LLM,请求更正回复.
func (gc *GuardrailsCoordinator) BuildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

// GetInputValidator Chain 返回输入验证器链.
func (gc *GuardrailsCoordinator) GetInputValidatorChain() *guardrails.ValidatorChain {
	return gc.inputValidatorChain
}

// GetOutputValidator 返回输出验证器 。
func (gc *GuardrailsCoordinator) GetOutputValidator() *guardrails.OutputValidator {
	return gc.outputValidator
}

// GetConfig 返回守护链配置 。
func (gc *GuardrailsCoordinator) GetConfig() *guardrails.GuardrailsConfig {
	return gc.config
}

// 输入ValidatorCount 返回输入验证器的数量 。
func (gc *GuardrailsCoordinator) InputValidatorCount() int {
	if gc.inputValidatorChain == nil {
		return 0
	}
	return gc.inputValidatorChain.Len()
}
