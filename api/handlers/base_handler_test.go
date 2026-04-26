package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestBaseHandler_ConcurrentUpdateService(t *testing.T) {
	logger := zap.NewNop()
	h := NewBaseHandler("initial", logger)

	assert.Equal(t, "initial", h.currentService())

	// Update service
	h.UpdateService("updated")
	assert.Equal(t, "updated", h.currentService())

	// Test nil safety
	var nilH *BaseHandler[string]
	nilH.UpdateService("should-not-panic")
	assert.Equal(t, "", nilH.currentService())
}

func TestBaseHandler_Logger(t *testing.T) {
	logger := zap.NewNop()
	h := NewBaseHandler(42, logger)

	assert.Equal(t, logger, h.Logger())

	newLogger := zap.NewNop()
	h.SetLogger(newLogger)
	assert.Equal(t, newLogger, h.Logger())

	// Nil safety
	var nilH *BaseHandler[int]
	assert.Nil(t, nilH.Logger())
	nilH.SetLogger(logger) // should not panic
}

func TestBaseHandler_NilService(t *testing.T) {
	logger := zap.NewNop()
	h := NewBaseHandler((*int)(nil), logger)

	assert.Nil(t, h.currentService())
}
