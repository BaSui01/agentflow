// Copyright 2024 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by a MIT license that can be
// found in the LICENSE file.

/*
Package guardrails provides input/output validation and content filtering for agents.

# Overview

The guardrails package implements security guardrails for AI agents, including
PII detection, prompt injection prevention, content filtering, and customizable
validation rules. It helps ensure that agent inputs and outputs are safe,
compliant, and appropriate.

# Architecture

	┌─────────────────────────────────────────────────────────────┐
	│                    ValidatorChain                           │
	│  (Orchestrates multiple validators in priority order)       │
	├─────────────────────────────────────────────────────────────┤
	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
	│  │   Length    │  │    PII      │  │     Injection       │ │
	│  │  Validator  │  │  Detector   │  │     Detector        │ │
	│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
	│  │  Content    │  │   Output    │  │      Custom         │ │
	│  │   Type      │  │  Validator  │  │    Validators       │ │
	│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
	└─────────────────────────────────────────────────────────────┘

# Validator Interface

All validators implement the Validator interface:

	type Validator interface {
	    Validate(ctx context.Context, content string) (*ValidationResult, error)
	    Name() string
	    Priority() int
	}

# Built-in Validators

LengthValidator: Validates input length constraints.

	validator := guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
	    MaxLength: 10000,
	    Action:    guardrails.LengthActionReject,
	})

PIIDetector: Detects personally identifiable information.

	detector := guardrails.NewPIIDetector(&guardrails.PIIDetectorConfig{
	    DetectEmail:      true,
	    DetectPhone:      true,
	    DetectCreditCard: true,
	    DetectSSN:        true,
	    Action:           guardrails.PIIActionMask,
	})

InjectionDetector: Detects prompt injection attempts.

	detector := guardrails.NewInjectionDetector(&guardrails.InjectionDetectorConfig{
	    Sensitivity: guardrails.SensitivityMedium,
	    Action:      guardrails.InjectionActionBlock,
	})

OutputValidator: Validates agent outputs for safety and compliance.

	validator := guardrails.NewOutputValidator(&guardrails.OutputValidatorConfig{
	    BlockHarmfulContent: true,
	    BlockPII:            true,
	})

# Validator Chain

The ValidatorChain orchestrates multiple validators:

	chain := guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
	    Mode: guardrails.ChainModeCollectAll, // or ChainModeFailFast
	})

	chain.Add(
	    guardrails.NewLengthValidator(nil),
	    guardrails.NewPIIDetector(nil),
	    guardrails.NewInjectionDetector(nil),
	)

	result, err := chain.Validate(ctx, userInput)
	if err != nil {
	    log.Fatal(err)
	}

	if !result.Valid {
	    for _, e := range result.Errors {
	        log.Printf("Validation error: %s - %s", e.Code, e.Message)
	    }
	}

# Chain Modes

  - ChainModeFailFast: Stop at first validation error (faster)
  - ChainModeCollectAll: Run all validators and collect all errors (comprehensive)

# Validation Result

The ValidationResult contains detailed validation information:

	type ValidationResult struct {
	    Valid    bool              // Overall validation status
	    Errors   []ValidationError // List of validation errors
	    Warnings []string          // Non-blocking warnings
	    Metadata map[string]any    // Additional metadata
	}

# Error Codes

Predefined error codes for common validation failures:

	const (
	    ErrCodeMaxLengthExceeded = "max_length_exceeded"
	    ErrCodePIIDetected       = "pii_detected"
	    ErrCodeInjectionDetected = "injection_detected"
	    ErrCodeContentBlocked    = "content_blocked"
	)

# Custom Validators

Implement the Validator interface to create custom validators:

	type MyValidator struct {
	    priority int
	}

	func (v *MyValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	    result := guardrails.NewValidationResult()
	    // Custom validation logic
	    if containsBadWord(content) {
	        result.AddError(guardrails.ValidationError{
	            Code:     "bad_word_detected",
	            Message:  "Content contains inappropriate language",
	            Severity: guardrails.SeverityHigh,
	        })
	    }
	    return result, nil
	}

	func (v *MyValidator) Name() string     { return "my_validator" }
	func (v *MyValidator) Priority() int    { return v.priority }

# Integration with Agents

Guardrails integrate seamlessly with the agent framework:

	agent, err := agent.NewAgentBuilder(config).
	    WithProvider(provider).
	    WithInputGuardrails(inputChain).
	    WithOutputGuardrails(outputChain).
	    Build()

# Thread Safety

All validators and the ValidatorChain are thread-safe and can be used
concurrently from multiple goroutines.

# Performance

The package is optimized for performance:
  - Compiled regex patterns are cached
  - Validators run in priority order (fail-fast mode)
  - Minimal allocations in hot paths

See benchmark results:

	BenchmarkValidatorChain_Validate-12       23475    4613 ns/op    680 B/op    13 allocs/op
	BenchmarkPIIDetector_Detect-12            15234    7823 ns/op    512 B/op     8 allocs/op
*/
package guardrails
