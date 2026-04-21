package agent

import (
	"github.com/BaSui01/agentflow/agent/guardcore"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

// 护卫系统启动
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) initGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.guardrailsEnabled = true

	// 初始化输入验证链
	b.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})

	// 添加已配置的输入验证符
	for _, v := range cfg.InputValidators {
		b.inputValidatorChain.Add(v)
	}

	// 根据配置添加内置验证符
	if cfg.MaxInputLength > 0 {
		b.inputValidatorChain.Add(guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
			MaxLength: cfg.MaxInputLength,
			Action:    guardrails.LengthActionReject,
		}))
	}

	if len(cfg.BlockedKeywords) > 0 {
		b.inputValidatorChain.Add(guardrails.NewKeywordValidator(&guardrails.KeywordValidatorConfig{
			BlockedKeywords: cfg.BlockedKeywords,
			CaseSensitive:   false,
		}))
	}

	if cfg.InjectionDetection {
		b.inputValidatorChain.Add(guardrails.NewInjectionDetector(nil))
	}

	if cfg.PIIDetectionEnabled {
		b.inputValidatorChain.Add(guardrails.NewPIIDetector(nil))
	}

	// 初始化输出验证符
	outputConfig := &guardrails.OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	}
	b.outputValidator = guardrails.NewOutputValidator(outputConfig)

	b.logger.Info("guardrails initialized",
		zap.Int("input_validators", b.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

// 设置守护栏为代理设置守护栏
// 1.7: 支持海关验证规则的登记和延期
// 使用 configMu，不与 Execute 的 execMu 争用
func (b *BaseAgent) SetGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	b.runtimeGuardrailsCfg = cfg
	b.config.Features.Guardrails = typesGuardrailsFromRuntime(cfg)
	if cfg == nil {
		b.guardrailsEnabled = false
		b.inputValidatorChain = nil
		b.outputValidator = nil
		return
	}
	b.initGuardrails(cfg)
}

// 是否启用了护栏
func (b *BaseAgent) GuardrailsEnabled() bool {
	b.configMu.RLock()
	defer b.configMu.RUnlock()
	return b.guardrailsEnabled
}

// 添加自定义输入验证器
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) AddInputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.inputValidatorChain == nil {
		b.inputValidatorChain = guardrails.NewValidatorChain(nil)
		b.guardrailsEnabled = true
	}
	b.inputValidatorChain.Add(v)
}

// 添加输出变量添加自定义输出验证器
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) AddOutputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddValidator(v)
}

// 添加 OutputFilter 添加自定义输出过滤器
func (b *BaseAgent) AddOutputFilter(f guardrails.Filter) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddFilter(f)
}

// GuardrailsManager is the agent facade type for guardrails management.
type GuardrailsManager = guardcore.Manager

// NewGuardrailsManager creates a new GuardrailsManager.
func NewGuardrailsManager(logger *zap.Logger) *GuardrailsManager {
	return guardcore.NewManager(logger)
}

// GuardrailsCoordinator is the agent facade type for guardrails coordination.
type GuardrailsCoordinator = guardcore.Coordinator

// NewGuardrailsCoordinator creates a new GuardrailsCoordinator.
func NewGuardrailsCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *GuardrailsCoordinator {
	return guardcore.NewCoordinator(config, logger)
}
