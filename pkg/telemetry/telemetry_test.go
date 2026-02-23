package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap/zaptest"
)

// saveAndRestoreGlobalProviders snapshots the current global OTel providers
// and restores them via t.Cleanup so tests don't leak state.
func saveAndRestoreGlobalProviders(t *testing.T) {
	t.Helper()
	origTP := otel.GetTracerProvider()
	origMP := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetMeterProvider(origMP)
	})
}

func TestInit_Disabled(t *testing.T) {
	saveAndRestoreGlobalProviders(t)
	logger := zaptest.NewLogger(t)

	cfg := config.TelemetryConfig{
		Enabled: false,
	}

	p, err := Init(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, p)

	// Noop providers — both internal fields are nil
	assert.Nil(t, p.tp, "TracerProvider should be nil when disabled")
	assert.Nil(t, p.mp, "MeterProvider should be nil when disabled")
}

func TestInit_Enabled(t *testing.T) {
	saveAndRestoreGlobalProviders(t)
	logger := zaptest.NewLogger(t)

	cfg := config.TelemetryConfig{
		Enabled:      true,
		OTLPEndpoint: "localhost:4317",
		ServiceName:  "agentflow-test",
		SampleRate:   0.5,
	}

	p, err := Init(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, p)

	// Real providers — both internal fields are non-nil
	assert.NotNil(t, p.tp, "TracerProvider should be set when enabled")
	assert.NotNil(t, p.mp, "MeterProvider should be set when enabled")

	// Global providers should be the SDK types (not noop)
	globalTP := otel.GetTracerProvider()
	globalMP := otel.GetMeterProvider()
	_, tpIsSDK := globalTP.(*sdktrace.TracerProvider)
	_, mpIsSDK := globalMP.(*sdkmetric.MeterProvider)
	assert.True(t, tpIsSDK, "global TracerProvider should be *sdktrace.TracerProvider")
	assert.True(t, mpIsSDK, "global MeterProvider should be *sdkmetric.MeterProvider")

	// Cleanup: shutdown to release resources (short timeout — no collector running)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = p.Shutdown(ctx)
	})
}

func TestProviders_Shutdown_Nil(t *testing.T) {
	// A nil *Providers must not panic on Shutdown.
	var p *Providers
	err := p.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestProviders_Shutdown_Noop(t *testing.T) {
	saveAndRestoreGlobalProviders(t)
	logger := zaptest.NewLogger(t)

	cfg := config.TelemetryConfig{Enabled: false}
	p, err := Init(cfg, logger)
	require.NoError(t, err)

	// Shutdown on noop providers should return nil
	err = p.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestProviders_Shutdown_Real(t *testing.T) {
	saveAndRestoreGlobalProviders(t)
	logger := zaptest.NewLogger(t)

	cfg := config.TelemetryConfig{
		Enabled:      true,
		OTLPEndpoint: "localhost:4317",
		ServiceName:  "agentflow-shutdown-test",
		SampleRate:   1.0,
	}

	p, err := Init(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, p.tp)
	require.NotNil(t, p.mp)

	// Shutdown completes without panic. The exporter may return a
	// connection-refused error because no OTLP collector is running,
	// which is expected in a test environment — we only verify it
	// doesn't panic and finishes within the deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	assert.NotPanics(t, func() {
		_ = p.Shutdown(ctx)
	})
}

func TestBuildVersion(t *testing.T) {
	v := buildVersion()
	assert.NotEmpty(t, v, "buildVersion should return a non-empty string")
	// In test binaries, debug.ReadBuildInfo typically returns "(devel)",
	// so buildVersion falls back to "dev".
	assert.Equal(t, "dev", v)
}