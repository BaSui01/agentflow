package guardrails

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

type Coordinator struct {
	inputValidatorChain *ValidatorChain
	outputValidator     *OutputValidator
	config              *GuardrailsConfig
	enabled             bool
	logger              *zap.Logger
}

func NewCoordinator(config *GuardrailsConfig, logger *zap.Logger) *Coordinator {
	gc := &Coordinator{
		config: config,
		logger: logger.With(zap.String("component", "guardrails_coordinator")),
	}
	if config != nil {
		gc.initialize(config)
	}
	return gc
}

func (gc *Coordinator) initialize(cfg *GuardrailsConfig) {
	gc.enabled = true
	gc.inputValidatorChain = NewValidatorChain(&ValidatorChainConfig{
		Mode: ChainModeCollectAll,
	})
	for _, v := range cfg.InputValidators {
		gc.inputValidatorChain.Add(v)
	}
	if cfg.MaxInputLength > 0 {
		gc.inputValidatorChain.Add(NewLengthValidator(&LengthValidatorConfig{
			MaxLength: cfg.MaxInputLength,
			Action:    LengthActionReject,
		}))
	}
	if len(cfg.BlockedKeywords) > 0 {
		gc.inputValidatorChain.Add(NewKeywordValidator(&KeywordValidatorConfig{
			BlockedKeywords: cfg.BlockedKeywords,
			CaseSensitive:   false,
		}))
	}
	if cfg.InjectionDetection {
		gc.inputValidatorChain.Add(NewInjectionDetector(nil))
	}
	if cfg.PIIDetectionEnabled {
		gc.inputValidatorChain.Add(NewPIIDetector(nil))
	}

	outputConfig := &OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	}
	gc.outputValidator = NewOutputValidator(outputConfig)

	gc.logger.Info("guardrails initialized",
		zap.Int("input_validators", gc.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

func (gc *Coordinator) ValidateInput(ctx context.Context, input string) (*ValidationResult, error) {
	if !gc.enabled || gc.inputValidatorChain == nil {
		return &ValidationResult{Valid: true}, nil
	}
	return gc.inputValidatorChain.Validate(ctx, input)
}

func (gc *Coordinator) ValidateOutput(ctx context.Context, output string) (string, *ValidationResult, error) {
	if !gc.enabled || gc.outputValidator == nil {
		return output, &ValidationResult{Valid: true}, nil
	}
	return gc.outputValidator.ValidateAndFilter(ctx, output)
}

func (gc *Coordinator) Enabled() bool                { return gc.enabled }
func (gc *Coordinator) SetEnabled(enabled bool)      { gc.enabled = enabled }
func (gc *Coordinator) GetConfig() *GuardrailsConfig { return gc.config }

func (gc *Coordinator) AddInputValidator(v Validator) {
	if gc.inputValidatorChain == nil {
		gc.inputValidatorChain = NewValidatorChain(nil)
		gc.enabled = true
	}
	gc.inputValidatorChain.Add(v)
}

func (gc *Coordinator) AddOutputValidator(v Validator) {
	if gc.outputValidator == nil {
		gc.outputValidator = NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddValidator(v)
}

func (gc *Coordinator) AddOutputFilter(f Filter) {
	if gc.outputValidator == nil {
		gc.outputValidator = NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddFilter(f)
}

func (gc *Coordinator) BuildValidationFeedbackMessage(result *ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

func (gc *Coordinator) GetInputValidatorChain() *ValidatorChain {
	return gc.inputValidatorChain
}
func (gc *Coordinator) GetOutputValidator() *OutputValidator { return gc.outputValidator }

func (gc *Coordinator) InputValidatorCount() int {
	if gc.inputValidatorChain == nil {
		return 0
	}
	return gc.inputValidatorChain.Len()
}
