package telemetry

import (
	"time"
)

// InitConfig holds the configuration needed to initialize OpenTelemetry.
// This is a self-contained copy of the relevant fields from config.TelemetryConfig,
// decoupling pkg/telemetry from the config package.
type InitConfig struct {
	// Enabled controls whether telemetry is active.
	Enabled bool
	// OTLPEndpoint is the OTLP gRPC endpoint for traces and metrics.
	OTLPEndpoint string
	// OTLPInsecure uses plaintext connection (dev/test only).
	OTLPInsecure bool
	// ServiceName identifies this service in traces and metrics.
	ServiceName string
	// SampleRate controls the trace sampling ratio (0.0–1.0).
	SampleRate float64
	// ShutdownTimeout is the maximum time to wait for pending spans/metrics on shutdown.
	ShutdownTimeout time.Duration
}
