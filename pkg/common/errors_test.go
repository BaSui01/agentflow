package common

import (
	"errors"
	"testing"
)

func TestWrap(t *testing.T) {
	// Test with nil error
	if err := Wrap(nil, "message"); err != nil {
		t.Errorf("Wrap(nil, ...) = %v, want nil", err)
	}

	// Test with error
	baseErr := errors.New("base error")
	wrapped := Wrap(baseErr, "wrapped")
	if wrapped.Error() != "wrapped: base error" {
		t.Errorf("Wrap error = %q, want %q", wrapped.Error(), "wrapped: base error")
	}

	// Test unwrapping
	unwrapped := errors.Unwrap(wrapped)
	if unwrapped.Error() != "base error" {
		t.Errorf("Unwrap = %q, want %q", unwrapped.Error(), "base error")
	}
}

func TestWrapf(t *testing.T) {
	// Test with nil error
	if err := Wrapf(nil, "format %s", "test"); err != nil {
		t.Errorf("Wrapf(nil, ...) = %v, want nil", err)
	}

	// Test with error
	baseErr := errors.New("base error")
	wrapped := Wrapf(baseErr, "failed at %s", "step1")
	if wrapped.Error() != "failed at step1: base error" {
		t.Errorf("Wrapf error = %q, want %q", wrapped.Error(), "failed at step1: base error")
	}
}

func TestCause(t *testing.T) {
	// Test with single error
	baseErr := errors.New("base")
	if Cause(baseErr).Error() != "base" {
		t.Errorf("Cause = %v, want base", Cause(baseErr))
	}

	// Test with wrapped error
	wrapped1 := Wrap(baseErr, "level1")
	wrapped2 := Wrap(wrapped1, "level2")
	if Cause(wrapped2).Error() != "base" {
		t.Errorf("Cause = %v, want base", Cause(wrapped2))
	}
}

func TestIs(t *testing.T) {
	baseErr := errors.New("base")
	wrapped := Wrap(baseErr, "wrapped")

	if !Is(wrapped, baseErr) {
		t.Error("Is should return true for wrapped error")
	}

	otherErr := errors.New("other")
	if Is(wrapped, otherErr) {
		t.Error("Is should return false for different error")
	}
}

type customError struct{ msg string }

func (e *customError) Error() string { return e.msg }

func TestAs(t *testing.T) {
	customErr := &customError{msg: "custom"}
	wrapped := Wrap(customErr, "wrapped")

	var target *customError
	if !As(wrapped, &target) {
		t.Error("As should return true for matching type")
	}
	if target.msg != "custom" {
		t.Errorf("As target = %q, want %q", target.msg, "custom")
	}
}

func TestJoin(t *testing.T) {
	err1 := errors.New("error1")
	err2 := errors.New("error2")

	joined := Join(err1, err2)
	if joined == nil {
		t.Error("Join should not return nil for non-nil errors")
	}

	// Join with nil should ignore nil
	joined = Join(err1, nil, err2)
	if joined == nil {
		t.Error("Join should ignore nil errors")
	}

	// Join with all nil should return nil
	if Join(nil, nil) != nil {
		t.Error("Join(nil, nil) should return nil")
	}
}

func TestNew(t *testing.T) {
	err := New("test error")
	if err.Error() != "test error" {
		t.Errorf("New error = %q, want %q", err.Error(), "test error")
	}
}
