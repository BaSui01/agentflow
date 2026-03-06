package main

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (s *Server) WaitForShutdown() {
	// 使用 httpManager 的 WaitForShutdown（它会监听信号）
	if s.httpManager != nil {
		s.httpManager.WaitForShutdown()
	}

	// 执行清理
	s.Shutdown()
}

// Shutdown 优雅关闭所有服务
func (s *Server) Shutdown() {
	s.logger.Info("Starting graceful shutdown...")

	timeout := s.cfg.Server.ShutdownTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 0. 停止 rate limiter 清理 goroutine
	if s.rateLimiterCancel != nil {
		s.rateLimiterCancel()
	}
	if s.tenantRateLimiterCancel != nil {
		s.tenantRateLimiterCancel()
	}

	// 1-3. 通过统一生命周期注册表关闭 hot reload / HTTP / metrics 服务。
	s.stopLifecycleServices(ctx)

	// 4. Flush and shutdown telemetry exporters
	// 必须在 HTTP/Metrics server 关闭之后执行，确保 in-flight 请求的 span/metric 不丢失
	if s.telemetry != nil {
		if err := s.telemetry.Shutdown(ctx); err != nil {
			s.logger.Error("Telemetry shutdown error", zap.Error(err))
		}
	}

	// 5. Teardown cached agent instances
	if s.resolver != nil {
		s.resolver.TeardownAll(ctx)
	}

	// 6. 关闭数据库连接
	if s.db != nil {
		if sqlDB, err := s.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				s.logger.Error("Database close error", zap.Error(err))
			} else {
				s.logger.Info("Database connection closed")
			}
		}
	}

	// 7. 关闭 MongoDB 连接
	if s.mongoClient != nil {
		if err := s.mongoClient.Close(ctx); err != nil {
			s.logger.Error("MongoDB close error", zap.Error(err))
		} else {
			s.logger.Info("MongoDB connection closed")
		}
	}

	// 7.1 关闭多模态 Redis 连接（如果启用）
	if s.multimodalRedis != nil {
		if err := s.multimodalRedis.Close(); err != nil {
			s.logger.Error("Multimodal Redis close error", zap.Error(err))
		}
	}

	// 7.5 关闭 AuditLogger
	if s.auditLogger != nil {
		if err := s.auditLogger.Close(); err != nil {
			s.logger.Error("AuditLogger close error", zap.Error(err))
		}
	}

	// 8. 等待所有 goroutine 完成
	s.wg.Wait()

	s.logger.Info("Graceful shutdown completed")
}
