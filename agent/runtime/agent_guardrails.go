package runtime

import (
	"strings"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// Guardrails Configuration Helpers
// =============================================================================

func runtimeGuardrailsFromTypes(cfg *types.GuardrailsConfig) *guardrails.GuardrailsConfig {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	out := guardrails.DefaultConfig()
	if cfg.MaxInputLength > 0 {
		out.MaxInputLength = cfg.MaxInputLength
	}
	if len(cfg.BlockedKeywords) > 0 {
		out.BlockedKeywords = append([]string(nil), cfg.BlockedKeywords...)
	}
	out.PIIDetectionEnabled = cfg.PIIDetection
	out.InjectionDetection = cfg.InjectionDetection
	out.MaxRetries = cfg.MaxRetries
	if v := strings.TrimSpace(cfg.OnInputFailure); v != "" {
		out.OnInputFailure = guardrails.FailureAction(v)
	}
	if v := strings.TrimSpace(cfg.OnOutputFailure); v != "" {
		out.OnOutputFailure = guardrails.FailureAction(v)
	}
	return out
}

func typesGuardrailsFromRuntime(cfg *guardrails.GuardrailsConfig) *types.GuardrailsConfig {
	if cfg == nil {
		return nil
	}
	return &types.GuardrailsConfig{
		Enabled:            true,
		MaxInputLength:     cfg.MaxInputLength,
		BlockedKeywords:    append([]string(nil), cfg.BlockedKeywords...),
		PIIDetection:       cfg.PIIDetectionEnabled,
		InjectionDetection: cfg.InjectionDetection,
		MaxRetries:         cfg.MaxRetries,
		OnInputFailure:     string(cfg.OnInputFailure),
		OnOutputFailure:    string(cfg.OnOutputFailure),
	}
}

// =============================================================================
// Guardrails Manager Types and Constructors
// =============================================================================

type GuardrailsManager = guardrails.Manager

func NewGuardrailsManager(logger *zap.Logger) *GuardrailsManager {
	return guardrails.NewManager(logger)
}

func NewGuardrailsCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *guardrails.Coordinator {
	return guardrails.NewCoordinator(config, logger)
}

// =============================================================================
// BaseAgent Guardrails Methods
// =============================================================================

func (b *BaseAgent) initGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.guardrailsEnabled = true
	b.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})
	for _, v := range cfg.InputValidators {
		b.inputValidatorChain.Add(v)
	}
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
	b.outputValidator = guardrails.NewOutputValidator(&guardrails.OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	})
	b.logger.Info("guardrails initialized",
		zap.Int("input_validators", b.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

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

func (b *BaseAgent) GuardrailsEnabled() bool {
	b.configMu.RLock()
	defer b.configMu.RUnlock()
	return b.guardrailsEnabled
}

func (b *BaseAgent) AddInputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.inputValidatorChain == nil {
		b.inputValidatorChain = guardrails.NewValidatorChain(nil)
		b.guardrailsEnabled = true
	}
	b.inputValidatorChain.Add(v)
}

func (b *BaseAgent) AddOutputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddValidator(v)
}

func (b *BaseAgent) AddOutputFilter(f guardrails.Filter) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddFilter(f)
}
