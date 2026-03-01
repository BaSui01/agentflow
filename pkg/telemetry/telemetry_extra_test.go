package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggerWithTrace_NoSpan(t *testing.T) {
	t.Helper()
	core, _ := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	ctx := context.Background()
	result := LoggerWithTrace(ctx, logger)
	assert.Equal(t, logger, result)
}

func TestLoggerWithTrace_InvalidSpan(t *testing.T) {
	t.Helper()
	core, _ := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	ctx := context.Background()
	span := trace.SpanFromContext(ctx)
	assert.False(t, span.SpanContext().IsValid())

	result := LoggerWithTrace(ctx, logger)
	assert.Equal(t, logger, result)
}

func TestLoggerWithTrace_ValidSpan(t *testing.T) {
	core, obs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	// Create a valid span context
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	result := LoggerWithTrace(ctx, logger)
	assert.NotEqual(t, logger, result)

	// Log something and check the fields
	result.Info("test")
	entries := obs.All()
	assert.Len(t, entries, 1)
	// Should have trace_id and span_id fields
	fields := entries[0].ContextMap()
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", fields["trace_id"])
	assert.Equal(t, "00f067aa0ba902b7", fields["span_id"])
}

func TestBuildVersion_ReturnsDev(t *testing.T) {
	// buildVersion reads debug.ReadBuildInfo() which returns "(devel)" in tests
	v := buildVersion()
	assert.Equal(t, "dev", v)
}

