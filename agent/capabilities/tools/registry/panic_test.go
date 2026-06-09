package registry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecoveredPanicToErrorKeepsErrorValues(t *testing.T) {
	err := errors.New("boom")

	assert.Same(t, err, RecoveredPanicToError(err))
}

func TestRecoveredPanicToErrorWrapsNonErrorValues(t *testing.T) {
	err := RecoveredPanicToError("boom")

	assert.EqualError(t, err, "panic: boom")
}
