// Package agent provides the core agent framework for AgentFlow.
// This file implements GuardrailsCoordinator for managing input/output validation.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

// GuardrailsCoordinator coordinates input/output validation using guardrails.
// It encapsulates guardrails logic that was previously in BaseAgent.
type GuardrailsCoordinator struct {
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	config              *guardrails.GuardrailsConfig
	enabled             bool
	logger              *zap.Logger
}

// NewGuardrailsCoordinator creates a new guardrails coordinator.
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

// initialize sets up the guardrails based on configuration.
func (gc *GuardrailsCoordinator) initialize(cfg *guardrails.GuardrailsConfig) {
	gc.enabled = true

	// Initialize input validator chain
	gc.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})

	// Add configured input validators
	for _, v := range cfg.InputValidators {
		gc.inputValidatorChain.Add(v)
	}

	// Add built-in validators based on config
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

	// Initialize output validator
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

// ValidateInput validates input content.
// Returns validation result with any errors found.
func (gc *GuardrailsCoordinator) ValidateInput(ctx context.Context, input string) (*guardrails.ValidationResult, error) {
	if !gc.enabled || gc.inputValidatorChain == nil {
		return &guardrails.ValidationResult{Valid: true}, nil
	}
	return gc.inputValidatorChain.Validate(ctx, input)
}

// ValidateOutput validates and filters output content.
// Returns the filtered output and validation result.
func (gc *GuardrailsCoordinator) ValidateOutput(ctx context.Context, output string) (string, *guardrails.ValidationResult, error) {
	if !gc.enabled || gc.outputValidator == nil {
		return output, &guardrails.ValidationResult{Valid: true}, nil
	}
	return gc.outputValidator.ValidateAndFilter(ctx, output)
}

// Enabled returns whether guardrails are enabled.
func (gc *GuardrailsCoordinator) Enabled() bool {
	return gc.enabled
}

// SetEnabled enables or disables guardrails.
func (gc *GuardrailsCoordinator) SetEnabled(enabled bool) {
	gc.enabled = enabled
}

// AddInputValidator adds a validator to the input chain.
func (gc *GuardrailsCoordinator) AddInputValidator(v guardrails.Validator) {
	if gc.inputValidatorChain == nil {
		gc.inputValidatorChain = guardrails.NewValidatorChain(nil)
		gc.enabled = true
	}
	gc.inputValidatorChain.Add(v)
}

// AddOutputValidator adds a validator to the output validator.
func (gc *GuardrailsCoordinator) AddOutputValidator(v guardrails.Validator) {
	if gc.outputValidator == nil {
		gc.outputValidator = guardrails.NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddValidator(v)
}

// AddOutputFilter adds a filter to the output validator.
func (gc *GuardrailsCoordinator) AddOutputFilter(f guardrails.Filter) {
	if gc.outputValidator == nil {
		gc.outputValidator = guardrails.NewOutputValidator(nil)
		gc.enabled = true
	}
	gc.outputValidator.AddFilter(f)
}

// BuildValidationFeedbackMessage creates a feedback message for validation failures.
// This message can be sent back to the LLM to request a corrected response.
func (gc *GuardrailsCoordinator) BuildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

// GetInputValidatorChain returns the input validator chain.
func (gc *GuardrailsCoordinator) GetInputValidatorChain() *guardrails.ValidatorChain {
	return gc.inputValidatorChain
}

// GetOutputValidator returns the output validator.
func (gc *GuardrailsCoordinator) GetOutputValidator() *guardrails.OutputValidator {
	return gc.outputValidator
}

// GetConfig returns the guardrails configuration.
func (gc *GuardrailsCoordinator) GetConfig() *guardrails.GuardrailsConfig {
	return gc.config
}

// InputValidatorCount returns the number of input validators.
func (gc *GuardrailsCoordinator) InputValidatorCount() int {
	if gc.inputValidatorChain == nil {
		return 0
	}
	return gc.inputValidatorChain.Len()
}
