// Package service defines a unified lifecycle interface for application services.
package service

import "context"

// Service defines the unified service lifecycle interface.
type Service interface {
	// Name returns the service name (used for logging and dependency resolution).
	Name() string
	// Start initializes and starts the service.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the service.
	Stop(ctx context.Context) error
}

// HealthChecker is an optional interface that services can implement
// to participate in health checks.
type HealthChecker interface {
	// Health returns nil when the service is healthy.
	Health(ctx context.Context) error
}

// ServiceInfo holds metadata for service registration.
type ServiceInfo struct {
	// Name identifies the service (must match Service.Name()).
	Name string
	// Priority controls startup order (lower numbers start first).
	Priority int
	// DependsOn lists service names that must be started before this one.
	DependsOn []string
}
