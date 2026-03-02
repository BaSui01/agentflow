package main

import (
	"context"
	"fmt"

	pkgservice "github.com/BaSui01/agentflow/pkg/service"
	"go.uber.org/zap"
)

type lifecycleService struct {
	name  string
	start func(context.Context) error
	stop  func(context.Context) error
}

func (s lifecycleService) Name() string { return s.name }

func (s lifecycleService) Start(ctx context.Context) error {
	if s.start == nil {
		return nil
	}
	return s.start(ctx)
}

func (s lifecycleService) Stop(ctx context.Context) error {
	if s.stop == nil {
		return nil
	}
	return s.stop(ctx)
}

func (svr *Server) startLifecycleServices() error {
	svr.serviceRegistry = pkgservice.NewRegistry(svr.logger)

	if svr.hotReloadManager != nil {
		svr.serviceRegistry.Register(lifecycleService{
			name: "hot_reload",
			start: func(ctx context.Context) error {
				if err := svr.hotReloadManager.Start(ctx); err != nil {
					return fmt.Errorf("start hot reload manager: %w", err)
				}
				svr.logger.Info("Hot reload manager started")
				return nil
			},
			stop: func(context.Context) error {
				if err := svr.hotReloadManager.Stop(); err != nil {
					return fmt.Errorf("stop hot reload manager: %w", err)
				}
				return nil
			},
		}, pkgservice.ServiceInfo{Name: "hot_reload", Priority: 10})
	}

	if svr.httpManager != nil {
		svr.serviceRegistry.Register(lifecycleService{
			name: "http_server",
			start: func(context.Context) error {
				if err := svr.httpManager.Start(); err != nil {
					return fmt.Errorf("start http server: %w", err)
				}
				svr.logger.Info("HTTP server started", zap.Int("port", svr.cfg.Server.HTTPPort))
				return nil
			},
			stop: func(ctx context.Context) error {
				if err := svr.httpManager.Shutdown(ctx); err != nil {
					return fmt.Errorf("stop http server: %w", err)
				}
				return nil
			},
		}, pkgservice.ServiceInfo{Name: "http_server", Priority: 20, DependsOn: []string{"hot_reload"}})
	}

	if svr.metricsManager != nil {
		svr.serviceRegistry.Register(lifecycleService{
			name: "metrics_server",
			start: func(context.Context) error {
				if err := svr.metricsManager.Start(); err != nil {
					return fmt.Errorf("start metrics server: %w", err)
				}
				svr.logger.Info("Metrics server started", zap.Int("port", svr.cfg.Server.MetricsPort))
				return nil
			},
			stop: func(ctx context.Context) error {
				if err := svr.metricsManager.Shutdown(ctx); err != nil {
					return fmt.Errorf("stop metrics server: %w", err)
				}
				return nil
			},
		}, pkgservice.ServiceInfo{Name: "metrics_server", Priority: 30, DependsOn: []string{"http_server"}})
	}

	return svr.serviceRegistry.StartAll(context.Background())
}

func (svr *Server) stopLifecycleServices(ctx context.Context) {
	if svr.serviceRegistry == nil {
		return
	}
	if err := svr.serviceRegistry.StopAll(ctx); err != nil {
		svr.logger.Error("Service registry shutdown error", zap.Error(err))
	}
}
