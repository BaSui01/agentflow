// =============================================================================
// AgentFlow OpenTelemetry SDK Initialization
// =============================================================================
// Wraps OTel SDK setup for traces and metrics. When telemetry is disabled,
// no exporters are created and global providers remain noop.
// =============================================================================

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/BaSui01/agentflow/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// Providers holds the OTel SDK TracerProvider and MeterProvider.
// When telemetry is disabled, both fields are nil and Shutdown is a no-op.
type Providers struct {
	tp *sdktrace.TracerProvider
	mp *sdkmetric.MeterProvider
}

// Init initializes the OTel SDK. When cfg.Enabled is false, it returns
// a noop Providers (nil tp/mp) without connecting to any external service.
func Init(cfg config.TelemetryConfig, logger *zap.Logger) (*Providers, error) {
	if !cfg.Enabled {
		logger.Info("telemetry disabled, using noop providers")
		return &Providers{}, nil
	}

	ctx := context.Background()

	// Build resource with service metadata
	version := buildVersion()
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	// Create OTLP gRPC trace exporter
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	// Create OTLP gRPC metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)

	// Create MeterProvider
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	// Register as global providers
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("telemetry initialized",
		zap.String("endpoint", cfg.OTLPEndpoint),
		zap.String("service_name", cfg.ServiceName),
		zap.Float64("sample_rate", cfg.SampleRate),
	)

	return &Providers{tp: tp, mp: mp}, nil
}

// Shutdown flushes pending spans/metrics and closes exporters.
// Safe to call on noop Providers (nil tp/mp).
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.tp != nil {
		if err := p.tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracer provider: %w", err))
		}
	}
	if p.mp != nil {
		if err := p.mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown meter provider: %w", err))
		}
	}
	return errors.Join(errs...)
}

// buildVersion extracts the module version from Go build info.
// Falls back to "dev" if unavailable.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
