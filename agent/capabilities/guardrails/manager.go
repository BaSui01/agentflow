package guardrails

import (
	"context"

	"go.uber.org/zap"
)

type Manager struct {
	enabled             bool
	inputValidatorChain *ValidatorChain
	outputValidator     *OutputValidator
	logger              *zap.Logger
}

func NewManager(logger *zap.Logger) *Manager {
	return &Manager{logger: logger}
}

func (g *Manager) Init(cfg *GuardrailsConfig) {
	if cfg == nil {
		return
	}
	g.enabled = true

	g.inputValidatorChain = NewValidatorChain(&ValidatorChainConfig{
		Mode: ChainModeCollectAll,
	})
	for _, v := range cfg.InputValidators {
		g.inputValidatorChain.Add(v)
	}
	if cfg.MaxInputLength > 0 {
		g.inputValidatorChain.Add(NewLengthValidator(&LengthValidatorConfig{
			MaxLength: cfg.MaxInputLength,
			Action:    LengthActionReject,
		}))
	}
	if len(cfg.BlockedKeywords) > 0 {
		g.inputValidatorChain.Add(NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: cfg.BlockedKeywords,
			CaseSensitive:   false,
		}))
	}
	if cfg.InjectionDetection {
		g.inputValidatorChain.Add(NewInjectionDetector(nil))
	}
	if cfg.PIIDetectionEnabled {
		g.inputValidatorChain.Add(NewPIIDetector(nil))
	}

	outputConfig := &OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	}
	g.outputValidator = NewOutputValidator(outputConfig)

	g.logger.Info("guardrails initialized",
		zap.Int("input_validators", g.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

func (g *Manager) SetConfig(cfg *GuardrailsConfig) {
	if cfg == nil {
		g.enabled = false
		g.inputValidatorChain = nil
		g.outputValidator = nil
		return
	}
	g.Init(cfg)
}

func (g *Manager) Enabled() bool { return g.enabled }

func (g *Manager) ValidateInput(ctx context.Context, content string) (*ValidationResult, error) {
	if g.inputValidatorChain == nil {
		return &ValidationResult{Valid: true}, nil
	}
	return g.inputValidatorChain.Validate(ctx, content)
}

func (g *Manager) ValidateAndFilterOutput(ctx context.Context, content string) (string, *ValidationResult, error) {
	if g.outputValidator == nil {
		return content, &ValidationResult{Valid: true}, nil
	}
	return g.outputValidator.ValidateAndFilter(ctx, content)
}

func (g *Manager) AddInputValidator(v Validator) {
	if g.inputValidatorChain == nil {
		g.inputValidatorChain = NewValidatorChain(nil)
		g.enabled = true
	}
	g.inputValidatorChain.Add(v)
}

func (g *Manager) AddOutputValidator(v Validator) {
	if g.outputValidator == nil {
		g.outputValidator = NewOutputValidator(nil)
		g.enabled = true
	}
	g.outputValidator.AddValidator(v)
}

func (g *Manager) AddOutputFilter(f Filter) {
	if g.outputValidator == nil {
		g.outputValidator = NewOutputValidator(nil)
		g.enabled = true
	}
	g.outputValidator.AddFilter(f)
}
