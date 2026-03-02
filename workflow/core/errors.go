package core

import (
	"errors"
	"fmt"
)

// 统一步骤错误。
var (
	ErrStepNotConfigured = errors.New("step dependency not configured")
	ErrStepValidation    = errors.New("step validation failed")
	ErrStepExecution     = errors.New("step execution failed")
	ErrStepTimeout       = errors.New("step execution timed out")
)

// StepError 包装步骤执行错误，携带步骤 ID 和类型。
type StepError struct {
	StepID   string
	StepType StepType
	Cause    error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("step %s (%s): %v", e.StepID, e.StepType, e.Cause)
}

func (e *StepError) Unwrap() error {
	return e.Cause
}

// NewStepError 创建步骤错误。
func NewStepError(stepID string, stepType StepType, cause error) *StepError {
	return &StepError{StepID: stepID, StepType: stepType, Cause: cause}
}
