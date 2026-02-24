package agent

import (
	"context"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

// GuardrailsManager encapsulates guardrails-related fields and methods extracted from BaseAgent.
type GuardrailsManager struct {
	enabled             bool
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	logger              *zap.Logger
}

// NewGuardrailsManager creates a new GuardrailsManager.
func NewGuardrailsManager(logger *zap.Logger) *GuardrailsManager {
	return &GuardrailsManager{logger: logger}
}

// Init initializes guardrails from the given config.
func (g *GuardrailsManager) Init(cfg *guardrails.GuardrailsConfig) {
	if cfg == nil {
		return
	}
	g.enabled = true

	// Initialize input validator chain
	g.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})

	for _, v := range cfg.InputValidators {
		g.inputValidatorChain.Add(v)
	}

	if cfg.MaxInputLength > 0 {
		g.inputValidatorChain.Add(guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
			MaxLength: cfg.MaxInputLength,
			Action:    guardrails.LengthActionReject,
		}))
	}

	if len(cfg.BlockedKeywords) > 0 {
		g.inputValidatorChain.Add(guardrails.NewKeywordValidator(&guardrails.KeywordValidatorConfig{
			BlockedKeywords: cfg.BlockedKeywords,
			CaseSensitive:   false,
		}))
	}
	if cfg.InjectionDetection {
		g.inputValidatorChain.Add(guardrails.NewInjectionDetector(nil))
	}

	if cfg.PIIDetectionEnabled {
		g.inputValidatorChain.Add(guardrails.NewPIIDetector(nil))
	}

	// Initialize output validator
	outputConfig := &guardrails.OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	}
	g.outputValidator = guardrails.NewOutputValidator(outputConfig)

	g.logger.Info("guardrails initialized",
		zap.Int("input_validators", g.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

// SetConfig replaces the guardrails configuration.
func (g *GuardrailsManager) SetConfig(cfg *guardrails.GuardrailsConfig) {
	if cfg == nil {
		g.enabled = false
		g.inputValidatorChain = nil
		g.outputValidator = nil
		return
	}
	g.Init(cfg)
}

// Enabled returns whether guardrails are enabled.
func (g *GuardrailsManager) Enabled() bool { return g.enabled }

// ValidateInput validates input content using the input validator chain.
func (g *GuardrailsManager) ValidateInput(ctx context.Context, content string) (*guardrails.ValidationResult, error) {
	if g.inputValidatorChain == nil {
		return &guardrails.ValidationResult{Valid: true}, nil
	}
	return g.inputValidatorChain.Validate(ctx, content)
}

// ValidateAndFilterOutput validates and filters output content.
func (g *GuardrailsManager) ValidateAndFilterOutput(ctx context.Context, content string) (string, *guardrails.ValidationResult, error) {
	if g.outputValidator == nil {
		return content, &guardrails.ValidationResult{Valid: true}, nil
	}
	return g.outputValidator.ValidateAndFilter(ctx, content)
}

// AddInputValidator adds a custom input validator.
func (g *GuardrailsManager) AddInputValidator(v guardrails.Validator) {
	if g.inputValidatorChain == nil {
		g.inputValidatorChain = guardrails.NewValidatorChain(nil)
		g.enabled = true
	}
	g.inputValidatorChain.Add(v)
}

// AddOutputValidator adds a custom output validator.
func (g *GuardrailsManager) AddOutputValidator(v guardrails.Validator) {
	if g.outputValidator == nil {
		g.outputValidator = guardrails.NewOutputValidator(nil)
		g.enabled = true
	}
	g.outputValidator.AddValidator(v)
}

// AddOutputFilter adds a custom output filter.
func (g *GuardrailsManager) AddOutputFilter(f guardrails.Filter) {
	if g.outputValidator == nil {
		g.outputValidator = guardrails.NewOutputValidator(nil)
		g.enabled = true
	}
	g.outputValidator.AddFilter(f)
}
