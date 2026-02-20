package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// 🌐 HTTP 服务器管理器
// =============================================================================

// Manager HTTP 服务器管理器
type Manager struct {
	server   *http.Server
	listener net.Listener
	errCh    chan error
	config   Config
	logger   *zap.Logger
	mu       sync.RWMutex
	closed   bool
}

// Config 服务器配置
type Config struct {
	// 监听地址
	Addr string `yaml:"addr" json:"addr"`

	// 读取超时
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`

	// 写入超时
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`

	// 空闲超时
	IdleTimeout time.Duration `yaml:"idle_timeout" json:"idle_timeout"`

	// 最大请求头大小
	MaxHeaderBytes int `yaml:"max_header_bytes" json:"max_header_bytes"`

	// 优雅关闭超时
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
}

// DefaultConfig 返回默认服务器配置
func DefaultConfig() Config {
	return Config{
		Addr:            ":8080",
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     120 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1 MB
		ShutdownTimeout: 30 * time.Second,
	}
}

// NewManager 创建服务器管理器
func NewManager(handler http.Handler, config Config, logger *zap.Logger) *Manager {
	server := &http.Server{
		Addr:           config.Addr,
		Handler:        handler,
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    config.IdleTimeout,
		MaxHeaderBytes: config.MaxHeaderBytes,
	}

	return &Manager{
		server: server,
		errCh:  make(chan error, 1),
		config: config,
		logger: logger.With(zap.String("component", "http_server")),
	}
}

// =============================================================================
// 🎯 核心方法
// =============================================================================

// Start 启动服务器（非阻塞）
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("server is closed")
	}

	if m.listener != nil {
		return fmt.Errorf("server already started")
	}

	listener, err := net.Listen("tcp", m.config.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", m.config.Addr, err)
	}

	m.listener = listener
	m.logger.Info("starting HTTP server", zap.String("addr", m.config.Addr))

	go m.serve(listener)

	return nil
}

// StartTLS 启动 HTTPS 服务器（非阻塞）
func (m *Manager) StartTLS(certFile, keyFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("server is closed")
	}

	if m.listener != nil {
		return fmt.Errorf("server already started")
	}

	listener, err := net.Listen("tcp", m.config.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", m.config.Addr, err)
	}

	m.listener = listener
	m.logger.Info("starting HTTPS server",
		zap.String("addr", m.config.Addr),
		zap.String("cert", certFile),
	)

	go m.serveTLS(listener, certFile, keyFile)

	return nil
}

func (m *Manager) serve(listener net.Listener) {
	if err := m.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		m.logger.Error("HTTP server failed", zap.Error(err))
		select {
		case m.errCh <- err:
		default:
		}
	}
}

func (m *Manager) serveTLS(listener net.Listener, certFile, keyFile string) {
	if err := m.server.ServeTLS(listener, certFile, keyFile); err != nil && err != http.ErrServerClosed {
		m.logger.Error("HTTPS server failed", zap.Error(err))
		select {
		case m.errCh <- err:
		default:
		}
	}
}

// Shutdown 优雅关闭服务器
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	m.logger.Info("shutting down HTTP server")

	// 创建带超时的上下文
	shutdownCtx, cancel := context.WithTimeout(ctx, m.config.ShutdownTimeout)
	defer cancel()

	// 优雅关闭
	if err := m.server.Shutdown(shutdownCtx); err != nil {
		m.logger.Error("HTTP server shutdown failed", zap.Error(err))
		return err
	}

	m.listener = nil

	m.logger.Info("HTTP server stopped")
	return nil
}

// WaitForShutdown 等待关闭信号
func (m *Manager) WaitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		m.logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	case err := <-m.errCh:
		if err != nil {
			m.logger.Error("server exited unexpectedly", zap.Error(err))
		}
	}

	ctx := context.Background()
	if err := m.Shutdown(ctx); err != nil {
		m.logger.Error("shutdown error", zap.Error(err))
	}
}

// Errors returns asynchronous server errors.
func (m *Manager) Errors() <-chan error {
	return m.errCh
}

// =============================================================================
// 🔧 辅助方法
// =============================================================================

// Addr 返回服务器监听地址
func (m *Manager) Addr() string {
	return m.config.Addr
}

// IsRunning 检查服务器是否运行中
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.closed
}
