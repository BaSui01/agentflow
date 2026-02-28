package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm/tools"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	pkgserver "github.com/BaSui01/agentflow/pkg/server"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// =============================================================================
// Service adapters — wrap existing components into the service.Service interface
// =============================================================================

// --- MongoDBService ---

// MongoDBService wraps pkg/mongodb.Client as a Service.
type MongoDBService struct {
	cfg    config.MongoDBConfig
	logger *zap.Logger
	client *mongoclient.Client
}

func NewMongoDBService(cfg config.MongoDBConfig, logger *zap.Logger) *MongoDBService {
	return &MongoDBService{cfg: cfg, logger: logger}
}

func (s *MongoDBService) Name() string { return "mongodb" }

func (s *MongoDBService) Start(ctx context.Context) error {
	client, err := mongoclient.NewClient(s.cfg, s.logger)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	s.client = client
	s.logger.Info("MongoDB client initialized", zap.String("database", s.cfg.Database))
	return nil
}

func (s *MongoDBService) Stop(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	return s.client.Close(ctx)
}

func (s *MongoDBService) Health(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("mongodb client not initialized")
	}
	return s.client.Ping(ctx)
}

// Client returns the underlying MongoDB client. Must be called after Start.
func (s *MongoDBService) Client() *mongoclient.Client { return s.client }

// --- TelemetryService ---

// TelemetryService wraps pkg/telemetry.Providers as a Service.
type TelemetryService struct {
	providers *telemetry.Providers
}

func NewTelemetryService(providers *telemetry.Providers) *TelemetryService {
	return &TelemetryService{providers: providers}
}

func (s *TelemetryService) Name() string                    { return "telemetry" }
func (s *TelemetryService) Start(_ context.Context) error   { return nil } // Already initialized in main.
func (s *TelemetryService) Stop(ctx context.Context) error  { return s.providers.Shutdown(ctx) }

// --- MetricsService ---

// MetricsService wraps the Prometheus metrics HTTP server as a Service.
type MetricsService struct {
	cfg     config.ServerConfig
	logger  *zap.Logger
	manager *pkgserver.Manager
}

func NewMetricsService(cfg config.ServerConfig, logger *zap.Logger) *MetricsService {
	return &MetricsService{cfg: cfg, logger: logger}
}

func (s *MetricsService) Name() string { return "metrics" }

func (s *MetricsService) Start(_ context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	serverConfig := pkgserver.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.MetricsPort),
		ReadTimeout:     s.cfg.ReadTimeout,
		WriteTimeout:    s.cfg.WriteTimeout,
		ShutdownTimeout: s.cfg.ShutdownTimeout,
	}

	s.manager = pkgserver.NewManager(mux, serverConfig, s.logger)
	if err := s.manager.Start(); err != nil {
		return err
	}
	s.logger.Info("Metrics server started", zap.Int("port", s.cfg.MetricsPort))
	return nil
}

func (s *MetricsService) Stop(ctx context.Context) error {
	if s.manager == nil {
		return nil
	}
	return s.manager.Shutdown(ctx)
}

// --- HTTPService ---

// HTTPService wraps the main HTTP server as a Service.
type HTTPService struct {
	handler http.Handler
	cfg     config.ServerConfig
	logger  *zap.Logger
	manager *pkgserver.Manager
}

func NewHTTPService(handler http.Handler, cfg config.ServerConfig, logger *zap.Logger) *HTTPService {
	return &HTTPService{handler: handler, cfg: cfg, logger: logger}
}

func (s *HTTPService) Name() string { return "http" }

func (s *HTTPService) Start(_ context.Context) error {
	serverConfig := pkgserver.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.HTTPPort),
		ReadTimeout:     s.cfg.ReadTimeout,
		WriteTimeout:    s.cfg.WriteTimeout,
		IdleTimeout:     2 * s.cfg.ReadTimeout,
		MaxHeaderBytes:  1 << 20,
		ShutdownTimeout: s.cfg.ShutdownTimeout,
	}

	s.manager = pkgserver.NewManager(s.handler, serverConfig, s.logger)
	if err := s.manager.Start(); err != nil {
		return err
	}
	s.logger.Info("HTTP server started", zap.Int("port", s.cfg.HTTPPort))
	return nil
}

func (s *HTTPService) Stop(ctx context.Context) error {
	if s.manager == nil {
		return nil
	}
	return s.manager.Shutdown(ctx)
}

// Manager returns the underlying server manager (used for WaitForShutdown).
func (s *HTTPService) Manager() *pkgserver.Manager { return s.manager }

// --- HotReloadService ---

// HotReloadService wraps config.HotReloadManager as a Service.
type HotReloadService struct {
	manager *config.HotReloadManager
}

func NewHotReloadService(manager *config.HotReloadManager) *HotReloadService {
	return &HotReloadService{manager: manager}
}

func (s *HotReloadService) Name() string { return "hotreload" }

func (s *HotReloadService) Start(ctx context.Context) error {
	return s.manager.Start(ctx)
}

func (s *HotReloadService) Stop(_ context.Context) error {
	return s.manager.Stop()
}

// --- AuditLoggerService ---

// AuditLoggerService wraps tools.DefaultAuditLogger as a Service so it can be
// shut down before MongoDB (ensuring pending writes are flushed first).
type AuditLoggerService struct {
	logger *tools.DefaultAuditLogger
}

func NewAuditLoggerService(auditLogger *tools.DefaultAuditLogger) *AuditLoggerService {
	return &AuditLoggerService{logger: auditLogger}
}

func (s *AuditLoggerService) Name() string                  { return "audit_logger" }
func (s *AuditLoggerService) Start(_ context.Context) error { return nil } // Already initialized.

func (s *AuditLoggerService) Stop(_ context.Context) error {
	if s.logger == nil {
		return nil
	}
	return s.logger.Close()
}

// --- DatabaseService ---

// DatabaseService wraps a *gorm.DB connection for health checking and shutdown.
type DatabaseService struct {
	db     dbCloser
	logger *zap.Logger
}

// dbCloser abstracts the sql.DB methods we need, avoiding a direct gorm import.
type dbCloser interface {
	PingContext(ctx context.Context) error
	Close() error
}

func NewDatabaseService(db dbCloser, logger *zap.Logger) *DatabaseService {
	return &DatabaseService{db: db, logger: logger}
}

func (s *DatabaseService) Name() string                  { return "database" }
func (s *DatabaseService) Start(_ context.Context) error { return nil } // Already connected.

func (s *DatabaseService) Stop(_ context.Context) error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *DatabaseService) Health(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.db.PingContext(ctx)
}
