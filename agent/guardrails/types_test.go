package guardrails

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ValidationResult
// ---------------------------------------------------------------------------

func TestNewValidationResult(t *testing.T) {
	t.Parallel()
	r := NewValidationResult()
	assert.True(t, r.Valid)
	assert.Empty(t, r.Errors)
	assert.Empty(t, r.Warnings)
	assert.NotNil(t, r.Metadata)
}

func TestValidationResult_AddError(t *testing.T) {
	t.Parallel()
	r := NewValidationResult()
	r.AddError(ValidationError{
		Code:     ErrCodeBlockedKeyword,
		Message:  "blocked",
		Severity: SeverityHigh,
	})
	assert.False(t, r.Valid)
	require.Len(t, r.Errors, 1)
	assert.Equal(t, ErrCodeBlockedKeyword, r.Errors[0].Code)
}

func TestValidationResult_AddWarning(t *testing.T) {
	t.Parallel()
	r := NewValidationResult()
	r.AddWarning("something minor")
	assert.True(t, r.Valid) // warnings don't invalidate
	require.Len(t, r.Warnings, 1)
	assert.Equal(t, "something minor", r.Warnings[0])
}

func TestValidationResult_Merge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		base       func() *ValidationResult
		other      func() *ValidationResult
		wantValid  bool
		wantErrors int
		wantWarns  int
	}{
		{
			name:       "merge nil is no-op",
			base:       NewValidationResult,
			other:      func() *ValidationResult { return nil },
			wantValid:  true,
			wantErrors: 0,
			wantWarns:  0,
		},
		{
			name: "merge invalid into valid",
			base: NewValidationResult,
			other: func() *ValidationResult {
				r := NewValidationResult()
				r.AddError(ValidationError{Code: "E1", Message: "err1", Severity: SeverityHigh})
				return r
			},
			wantValid:  false,
			wantErrors: 1,
			wantWarns:  0,
		},
		{
			name: "merge warnings",
			base: func() *ValidationResult {
				r := NewValidationResult()
				r.AddWarning("w1")
				return r
			},
			other: func() *ValidationResult {
				r := NewValidationResult()
				r.AddWarning("w2")
				return r
			},
			wantValid:  true,
			wantErrors: 0,
			wantWarns:  2,
		},
		{
			name: "merge metadata",
			base: func() *ValidationResult {
				r := NewValidationResult()
				r.Metadata["key1"] = "val1"
				return r
			},
			other: func() *ValidationResult {
				r := NewValidationResult()
				r.Metadata["key2"] = "val2"
				return r
			},
			wantValid:  true,
			wantErrors: 0,
			wantWarns:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := tt.base()
			base.Merge(tt.other())
			assert.Equal(t, tt.wantValid, base.Valid)
			assert.Len(t, base.Errors, tt.wantErrors)
			assert.Len(t, base.Warnings, tt.wantWarns)
		})
	}
}

func TestValidationResult_Merge_Tripwire(t *testing.T) {
	t.Parallel()
	base := NewValidationResult()
	other := NewValidationResult()
	other.Tripwire = true

	base.Merge(other)
	assert.True(t, base.Tripwire)
}

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 10000, cfg.MaxInputLength)
	assert.Equal(t, FailureActionReject, cfg.OnInputFailure)
	assert.Equal(t, FailureActionReject, cfg.OnOutputFailure)
	assert.False(t, cfg.PIIDetectionEnabled)
	assert.False(t, cfg.InjectionDetection)
	assert.Equal(t, 0, cfg.MaxRetries)
	assert.Empty(t, cfg.BlockedKeywords)
	assert.Empty(t, cfg.InputValidators)
	assert.Empty(t, cfg.OutputValidators)
	assert.Empty(t, cfg.OutputFilters)
}

// ---------------------------------------------------------------------------
// ValidatorRegistry
// ---------------------------------------------------------------------------

// registryMockValidator is a minimal Validator for registry tests.
type registryMockValidator struct {
	name     string
	priority int
}

func (v *registryMockValidator) Name() string     { return v.name }
func (v *registryMockValidator) Priority() int     { return v.priority }
func (v *registryMockValidator) Validate(_ context.Context, _ string) (*ValidationResult, error) {
	return NewValidationResult(), nil
}

func TestValidatorRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	reg := NewValidatorRegistry()
	v := &registryMockValidator{name: "test_v", priority: 1}
	reg.Register(v)

	got, ok := reg.Get("test_v")
	assert.True(t, ok)
	assert.Equal(t, "test_v", got.Name())
}

func TestValidatorRegistry_GetNotFound(t *testing.T) {
	t.Parallel()
	reg := NewValidatorRegistry()
	_, ok := reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestValidatorRegistry_Unregister(t *testing.T) {
	t.Parallel()
	reg := NewValidatorRegistry()
	v := &registryMockValidator{name: "to_remove"}
	reg.Register(v)
	reg.Unregister("to_remove")
	_, ok := reg.Get("to_remove")
	assert.False(t, ok)
}

func TestValidatorRegistry_List(t *testing.T) {
	t.Parallel()
	reg := NewValidatorRegistry()
	assert.Empty(t, reg.List())

	reg.Register(&registryMockValidator{name: "v1"})
	reg.Register(&registryMockValidator{name: "v2"})
	assert.Len(t, reg.List(), 2)
}

// ---------------------------------------------------------------------------
// FilterRegistry
// ---------------------------------------------------------------------------

// registryMockFilter is a minimal Filter for registry tests.
type registryMockFilter struct {
	name string
}

func (f *registryMockFilter) Name() string { return f.name }
func (f *registryMockFilter) Filter(_ context.Context, content string) (string, error) {
	return content, nil
}

func TestFilterRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	reg := NewFilterRegistry()
	f := &registryMockFilter{name: "test_f"}
	reg.Register(f)

	got, ok := reg.Get("test_f")
	assert.True(t, ok)
	assert.Equal(t, "test_f", got.Name())
}

func TestFilterRegistry_GetNotFound(t *testing.T) {
	t.Parallel()
	reg := NewFilterRegistry()
	_, ok := reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestFilterRegistry_Unregister(t *testing.T) {
	t.Parallel()
	reg := NewFilterRegistry()
	f := &registryMockFilter{name: "to_remove"}
	reg.Register(f)
	reg.Unregister("to_remove")
	_, ok := reg.Get("to_remove")
	assert.False(t, ok)
}

func TestFilterRegistry_List(t *testing.T) {
	t.Parallel()
	reg := NewFilterRegistry()
	assert.Empty(t, reg.List())

	reg.Register(&registryMockFilter{name: "f1"})
	reg.Register(&registryMockFilter{name: "f2"})
	reg.Register(&registryMockFilter{name: "f3"})
	assert.Len(t, reg.List(), 3)
}

func TestFilterRegistry_OverwriteExisting(t *testing.T) {
	t.Parallel()
	reg := NewFilterRegistry()
	reg.Register(&registryMockFilter{name: "f1"})
	reg.Register(&registryMockFilter{name: "f1"}) // overwrite
	assert.Len(t, reg.List(), 1)
}

func TestValidatorRegistry_OverwriteExisting(t *testing.T) {
	t.Parallel()
	reg := NewValidatorRegistry()
	reg.Register(&registryMockValidator{name: "v1", priority: 1})
	reg.Register(&registryMockValidator{name: "v1", priority: 2}) // overwrite
	assert.Len(t, reg.List(), 1)
	got, _ := reg.Get("v1")
	assert.Equal(t, 2, got.Priority())
}

// ---------------------------------------------------------------------------
// Severity and error code constants
// ---------------------------------------------------------------------------

func TestSeverityConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "critical", SeverityCritical)
	assert.Equal(t, "high", SeverityHigh)
	assert.Equal(t, "medium", SeverityMedium)
	assert.Equal(t, "low", SeverityLow)
}

func TestErrorCodeConstants(t *testing.T) {
	t.Parallel()
	codes := []string{
		ErrCodeInjectionDetected,
		ErrCodePIIDetected,
		ErrCodeMaxLengthExceeded,
		ErrCodeBlockedKeyword,
		ErrCodeContentBlocked,
		ErrCodeValidationFailed,
	}
	for _, code := range codes {
		assert.NotEmpty(t, code)
	}
}

func TestFailureActionConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, FailureAction("reject"), FailureActionReject)
	assert.Equal(t, FailureAction("warn"), FailureActionWarn)
	assert.Equal(t, FailureAction("retry"), FailureActionRetry)
}
