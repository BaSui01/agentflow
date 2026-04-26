package common

import (
	"errors"
	"fmt"
)

// Wrap wraps an error with a message.
// Example: Wrap(err, "failed to connect") returns an error with message "failed to connect: <err>".
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Wrapf wraps an error with a formatted message.
// Example: Wrapf(err, "failed to connect to %s", host).
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

// Cause returns the underlying cause of an error by unwrapping it.
// This is useful for finding the root error in a chain of wrapped errors.
func Cause(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}

// Is reports whether any error in err's tree matches target.
// This is a convenience wrapper around errors.Is.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's tree that matches target,
// and if one is found, sets target to that error value and returns true.
// This is a convenience wrapper around errors.As.
func As(err error, target any) bool {
	return errors.As(err, target)
}

// Join returns an error that wraps the given errors.
// Any nil error values are discarded.
// This is a convenience wrapper around errors.Join (Go 1.20+).
func Join(errs ...error) error {
	return errors.Join(errs...)
}

// New returns an error that formats as the given text.
// This is a convenience wrapper around errors.New.
func New(text string) error {
	return errors.New(text)
}
