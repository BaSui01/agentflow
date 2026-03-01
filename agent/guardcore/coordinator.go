package guardcore

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

type Coordinator struct {
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	config              *guardrails.GuardrailsConfig
	enabled             bool
	logger              *zap.Logger
}

func NewCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *Coordinator {
	gc := &Coordinator{
		config: config,
		logger: logger.With(zap.String("component", "guardrails_coordinator")),
	}
	if config != nil {
		gc.initialize(config)
	}
	return gc
}

func (gc *Coordinator) initialize(cfg *guardrails.GuardrailsConfig) {
	gc.enabled = true
	gc.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})
	for _, v := range cfg.InputValidators {
		gc.inputValidatorChain.Add(v)
	}
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

func (gc *Coordinator) ValidateInput(ctx context.Context, input string) (*guardrails.ValidationResult, error) {
	if !gc.enabled || gc.inputValidatorChain == nil {
		return &guardrails.ValidationResult{Valid: true}, nil
	}
	return gc.inputValidatorChain.Validate(ctx, input)
}

func (gc *Coordinator) ValidateOutput(ctx context.Context, output string) (string, *guardrails.ValidationResult, error) {
	if !gc.enabled || gc.outputValidator == nil {
		return output, &guardrails.ValidationResult{Valid: true}, nil
	}
	return gc.outputValidator.ValidateAndFilter(ctx, output)
}

func (gc *Coordinator) Enabled() bool            { return gc.enabled }
func (gc *Coordinator) SetEnabled(enabled bool)  { gc.enabled = enabled }
func (gc *Coordinator) GetConfig() *guardrails.GuardrailsConfig { return gc.config }

func (gc *Coordinator) AddInputValidator(v guardrails.Validator) {
	if gc.inputValidatorChain == nil {
		gc.inputValidatorChain = guardrails.NewValidatorChain(nil)
		gc.enabled = true
	}
	gc.inputValidatorChain.Add(v)
}

func (gc *Coordinator) AddOutputValidator(v guardrails.Validator) {
	if gc.outputValidator == nil {
		gc.outputValidator = guardrails.NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddValidator(v)
}

func (gc *Coordinator) AddOutputFilter(f guardrails.Filter) {
	if gc.outputValidator == nil {
		gc.outputValidator = guardrails.NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddFilter(f)
}

func (gc *Coordinator) BuildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

func (gc *Coordinator) GetInputValidatorChain() *guardrails.ValidatorChain { return gc.inputValidatorChain }
func (gc *Coordinator) GetOutputValidator() *guardrails.OutputValidator     { return gc.outputValidator }

func (gc *Coordinator) InputValidatorCount() int {
	if gc.inputValidatorChain == nil {
		return 0
	}
	return gc.inputValidatorChain.Len()
}
