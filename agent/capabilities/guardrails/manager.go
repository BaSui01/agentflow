package guardcore

import (
	"context"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

type Manager struct {
	enabled             bool
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	logger              *zap.Logger
}

func NewManager(logger *zap.Logger) *Manager {
	return &Manager{logger: logger}
}

func (g *Manager) Init(cfg *guardrails.GuardrailsConfig) {
	if cfg == nil {
		return
	}
	g.enabled = true

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

func (g *Manager) SetConfig(cfg *guardrails.GuardrailsConfig) {
	if cfg == nil {
		g.enabled = false
		g.inputValidatorChain = nil
		g.outputValidator = nil
		return
	}
	g.Init(cfg)
}

func (g *Manager) Enabled() bool { return g.enabled }

func (g *Manager) ValidateInput(ctx context.Context, content string) (*guardrails.ValidationResult, error) {
	if g.inputValidatorChain == nil {
		return &guardrails.ValidationResult{Valid: true}, nil
	}
	return g.inputValidatorChain.Validate(ctx, content)
}

func (g *Manager) ValidateAndFilterOutput(ctx context.Context, content string) (string, *guardrails.ValidationResult, error) {
	if g.outputValidator == nil {
		return content, &guardrails.ValidationResult{Valid: true}, nil
	}
	return g.outputValidator.ValidateAndFilter(ctx, content)
}

func (g *Manager) AddInputValidator(v guardrails.Validator) {
	if g.inputValidatorChain == nil {
		g.inputValidatorChain = guardrails.NewValidatorChain(nil)
		g.enabled = true
	}
	g.inputValidatorChain.Add(v)
}

func (g *Manager) AddOutputValidator(v guardrails.Validator) {
	if g.outputValidator == nil {
		g.outputValidator = guardrails.NewOutputValidator(nil)
		g.enabled = true
	}
	g.outputValidator.AddValidator(v)
}

func (g *Manager) AddOutputFilter(f guardrails.Filter) {
	if g.outputValidator == nil {
		g.outputValidator = guardrails.NewOutputValidator(nil)
		g.enabled = true
	}
	g.outputValidator.AddFilter(f)
}
